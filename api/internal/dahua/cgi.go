package dahua

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// cgi — legacy CGI chaqiruvi, HTTP Digest bilan.
//
// ⚠️ Dahua HAMMA JOYDA Digest ishlatadi, Basic emas. Go standart kutubxonasida
// Digest yo'q, shuning uchun challenge-response qo'lda bajariladi (digest.go).
func (c *Client) cgi(ctx context.Context, script string, params map[string]string) (string, error) {
	// ⚠️ `action` BIRINCHI bo'lishi SHART. `url.Values.Encode()` kalitlarni
	// alifbo bo'yicha saralaydi va `UserID=x&action=remove` hosil qiladi
	// (katta `U` kichik `a` dan oldin) — qurilma bunga 400 qaytaradi.
	// Xuddi shu URL `action` birinchi bo'lganda 200 beradi.
	//
	// Boshqa chaqiruvlar tasodifan ishlagan: `action`, `count`, `name`,
	// `offset` da alifbo tartibi baribir `action` ni birinchi qo'yadi.
	var parts []string
	if action, ok := params["action"]; ok {
		parts = append(parts, "action="+url.QueryEscape(action))
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "action" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys) // qolganlari barqaror tartibda bo'lsin

	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}

	// `Doors[0]` kabi kalitlardagi [ ] xom holda qolishi kerak — qurilma
	// %5B%5D ni qabul qilmaydi.
	raw := strings.NewReplacer("%5B", "[", "%5D", "]").Replace(strings.Join(parts, "&"))
	target := fmt.Sprintf("%s/cgi-bin/%s?%s", c.dev.baseURL(), script, raw)

	body, status, err := c.digestGet(ctx, target)
	if err != nil {
		return "", err
	}

	if status == http.StatusUnauthorized {
		// ⚠️ Dahua bir necha muvaffaqiyatsiz urinishdan keyin admin akkauntni
		// vaqtincha bloklaydi. QAYTA URINMAYMIZ.
		return "", fmt.Errorf("autentifikatsiya rad etildi (%s) — parolni tekshiring, qayta urinmang", c.dev.IP)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("qurilma %s: HTTP %d (%s)", c.dev.IP, status, script)
	}

	return body, nil
}

// ---------------------------------------------------------------- javob tahlili

var recordKeyRe = regexp.MustCompile(`^records\[(\d+)\]\.(.+)$`)

// ParseValue — `key=value` javobdan bitta qiymat.
func ParseValue(body, key string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
			return v
		}
	}
	return ""
}

// ParseRecords — `records[N].Field=value` javobni yozuvlar ro'yxatiga aylantiradi.
func ParseRecords(body string) []map[string]string {
	indexed := map[int]map[string]string{}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		m := recordKeyRe.FindStringSubmatch(strings.TrimSpace(k))
		if m == nil {
			continue
		}

		idx, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if indexed[idx] == nil {
			indexed[idx] = map[string]string{}
		}
		indexed[idx][m[2]] = v
	}

	keys := make([]int, 0, len(indexed))
	for k := range indexed {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	out := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, indexed[k])
	}
	return out
}

// ------------------------------------------------------------------- o'qish

// CountUsers — qurilmadagi foydalanuvchilar soni.
func (c *Client) CountUsers(ctx context.Context) (int, error) {
	body, err := c.cgi(ctx, "recordFinder.cgi", map[string]string{
		"action": "getQuerySize",
		"name":   "AccessControlCard",
	})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(ParseValue(body, "Size")))
}

// FetchUsers — foydalanuvchilarni sahifalab o'qiydi.
//
// ⚠️ `condition.StartTime` / `condition.EndTime` bu firmware'da JIMGINA
// e'tiborga olinmaydi — filtrlangan so'rov filtrsiz bilan bir xil natija
// qaytaradi, xato ham, ogohlantirish ham yo'q. Vaqt bo'yicha filtrlamang.
// `factory.create` (stateful finder) ham yo'q (400). Yagona yo'l — offset.
func (c *Client) FetchUsers(ctx context.Context, offset, count int) ([]map[string]string, error) {
	body, err := c.cgi(ctx, "recordFinder.cgi", map[string]string{
		"action": "doSeekFind",
		"name":   "AccessControlCard",
		"count":  strconv.Itoa(count),
		"offset": strconv.Itoa(offset),
	})
	if err != nil {
		return nil, err
	}
	return ParseRecords(body), nil
}

// FindUser — UserID bo'yicha qurilmadagi yozuvni topadi.
//
// ⚠️ Sahifalab qidiradi. `condition.*` filtrlari bu firmware'da JIMGINA
// e'tiborga olinmaydi (filtrlangan so'rov filtrsiz bilan bir xil natija
// beradi), shuning uchun ularga ishonib bo'lmaydi — javobni o'zimiz
// solishtiramiz.
//
// HAMMA mos yozuvni qaytaradi — bir xil UserID qurilmaga ikki marta tushib
// qolishi mumkin va bu aynan shunday nosozliklarning sababi bo'ladi.
func (c *Client) FindUser(ctx context.Context, userID string) ([]map[string]string, error) {
	const page = 200

	total, err := c.CountUsers(ctx)
	if err != nil {
		return nil, err
	}

	var found []map[string]string

	for offset := 0; offset < total; offset += page {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		records, err := c.FetchUsers(ctx, offset, page)
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			break
		}

		for _, rec := range records {
			if rec["UserID"] == userID {
				found = append(found, rec)
			}
		}
	}
	return found, nil
}

// RecentEvents — oxirgi kirish/chiqish yozuvlari.
//
// ⚠️ Vaqt bo'yicha filtrlash bu firmware'da JIMGINA ishlamaydi, shuning
// uchun oxirgi yozuvlar OXIRGI SAHIFADAN olinadi.
//
// ⚠️ Bu jadvaldagi `CreateTime` — epoch UTC (qurilma soatidan farqli, u
// lokal vaqt qaytaradi). Ikkisini aralashtirmang.
// `offset < 0` bo'lsa — oxiridan (eng yangi yozuvlar).
func (c *Client) RecentEvents(ctx context.Context, offset, count int) (int, []map[string]string, error) {
	body, err := c.cgi(ctx, "recordFinder.cgi", map[string]string{
		"action": "getQuerySize",
		"name":   "AccessControlCardRec",
	})
	if err != nil {
		return 0, nil, err
	}

	total, err := strconv.Atoi(strings.TrimSpace(ParseValue(body, "Size")))
	if err != nil {
		return 0, nil, fmt.Errorf("log hajmi o'qilmadi: %w", err)
	}

	if offset < 0 {
		offset = total - count
	}
	if offset < 0 {
		offset = 0
		count = total
	}

	body, err = c.cgi(ctx, "recordFinder.cgi", map[string]string{
		"action": "doSeekFind",
		"name":   "AccessControlCardRec",
		"count":  strconv.Itoa(count),
		"offset": strconv.Itoa(offset),
	})
	if err != nil {
		return 0, nil, err
	}
	return total, ParseRecords(body), nil
}

// Event — kirish/chiqish yozuvi.
//
// ⚠️ `Direction` bu yerda YO'Q: uni qurilmaning o'zidan (`devices.direction`)
// olish kerak. Hodisadagi `Type` maydoni ("Entry"/"Exit") ishonchsiz.
type Event struct {
	RecNo       int64
	UserID      string
	HappenedAt  time.Time
	Status      int
	ErrorCode   int
	Method      int
	SnapshotURL string
}

func parseEvent(rec map[string]string) (Event, bool) {
	recNo, err := strconv.ParseInt(strings.TrimSpace(rec["RecNo"]), 10, 64)
	if err != nil {
		return Event{}, false
	}

	sec, err := strconv.ParseInt(strings.TrimSpace(rec["CreateTime"]), 10, 64)
	if err != nil {
		return Event{}, false
	}

	atoi := func(s string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		return n
	}

	return Event{
		RecNo: recNo,
		// ⚠️ `CreateTime` — epoch UTC (qurilma soatidan farqli, u lokal
		// vaqt qaytaradi). Zonaga o'girish hisobotda bajariladi.
		HappenedAt:  time.Unix(sec, 0).UTC(),
		UserID:      strings.TrimSpace(rec["UserID"]),
		Status:      atoi(rec["Status"]),
		ErrorCode:   atoi(rec["ErrorCode"]),
		Method:      atoi(rec["Method"]),
		SnapshotURL: strings.TrimSpace(rec["URL"]),
	}, true
}

// EventsSince — `lastRecNo` dan keyingi hodisalar, eskisidan yangisiga qarab.
//
// ⚠️ Log — 100 000 yozuvli HALQA BUFER: eskisi ustiga yozib boriladi.
// Vaqt bo'yicha filtr esa bu firmware'da jimgina e'tiborga olinmaydi.
// Shuning uchun oxiridan orqaga qarab o'qiymiz va `lastRecNo` ga yetganda
// to'xtaymiz — kundalik yig'ishda odatda bitta sahifa yetadi.
//
// `maxPages` — birinchi yig'ishda butun logni tortib olmaslik uchun cheklov.
func (c *Client) EventsSince(ctx context.Context, lastRecNo int64, maxPages int) ([]Event, error) {
	const page = 200

	total, _, err := c.RecentEvents(ctx, 0, 1)
	if err != nil {
		return nil, err
	}

	var out []Event

	for i := 0; i < maxPages; i++ {
		offset := total - page*(i+1)
		count := page
		if offset < 0 {
			count += offset // oxirgi sahifa to'liq bo'lmasligi mumkin
			offset = 0
		}
		if count <= 0 {
			break
		}

		// ⚠️ Qurilmani bo'g'ib qo'ymaslik uchun oraliq. Bunsiz ~48 sahifadan
		// keyin javob bermay qoladi (o'lchangan: `i/o timeout`). Qurilma
		// qulamaydi, lekin o'ziga kelguncha bir necha daqiqa ketadi.
		if i > 0 {
			select {
			case <-time.After(250 * time.Millisecond):
			case <-ctx.Done():
				return out, ctx.Err()
			}
		}

		_, records, err := c.RecentEvents(ctx, offset, count)
		if err != nil {
			// O'qilganini QAYTARAMIZ: yarim yo'lda uzilsa ham shu paytgacha
			// yig'ilgani saqlanadi va keyingi chaqiruv o'shandan davom etadi.
			return out, err
		}

		reached := false
		for _, rec := range records {
			ev, ok := parseEvent(rec)
			if !ok {
				continue
			}
			if ev.RecNo <= lastRecNo {
				reached = true
				continue
			}
			out = append(out, ev)
		}

		if reached || offset == 0 {
			break
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].RecNo < out[j].RecNo })
	return out, nil
}

// TableSize — jadvaldagi yozuvlar soni. Jadval bo'lmasa xato qaytadi.
//
// Diagnostika uchun: qurilmada qaysi jadvallar borligini aniqlash.
func (c *Client) TableSize(ctx context.Context, name string) (int, error) {
	body, err := c.cgi(ctx, "recordFinder.cgi", map[string]string{
		"action": "getQuerySize",
		"name":   name,
	})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(ParseValue(body, "Size")))
}

// ------------------------------------------------------------------- yozish

// ClearAllUsers — HAMMA foydalanuvchini bir so'rovda o'chiradi.
//
// ⛔ QAYTARIB BO'LMAYDI. Yuz shablonlariga tegmaydi — ular alohida saqlanadi
// (`FaceInfoManager.cgi?action=clear` esa umuman yo'q, 400 qaytaradi).
func (c *Client) ClearAllUsers(ctx context.Context) error {
	body, err := c.cgi(ctx, "recordUpdater.cgi", map[string]string{
		"action": "clear",
		"name":   "AccessControlCard",
	})
	if err != nil {
		return err
	}
	if !strings.Contains(body, "OK") {
		return fmt.Errorf("clear rad etildi: %s", strings.TrimSpace(body))
	}
	return nil
}

// RemoveUser — RecNo bo'yicha o'chiradi (UserID emas).
//
// ⚠️ Bu YUZNI O'CHIRMAYDI — `RemoveFace` ham chaqirilishi kerak, aks holda
// qurilmada yetim yuz qoladi.
func (c *Client) RemoveUser(ctx context.Context, recNo int) error {
	body, err := c.cgi(ctx, "recordUpdater.cgi", map[string]string{
		"action": "remove",
		"name":   "AccessControlCard",
		"recno":  strconv.Itoa(recNo),
	})
	if err != nil {
		return err
	}
	if !strings.Contains(body, "OK") {
		return fmt.Errorf("remove rad etildi: %s", strings.TrimSpace(body))
	}
	return nil
}

func (c *Client) RemoveFace(ctx context.Context, userID string) error {
	body, err := c.cgi(ctx, "FaceInfoManager.cgi", map[string]string{
		"action": "remove",
		"UserID": userID,
	})
	if err != nil {
		return err
	}
	if !strings.Contains(body, "OK") {
		return fmt.Errorf("yuz o'chirilmadi: %s", strings.TrimSpace(body))
	}
	return nil
}

// UploadFace — yuz rasmini yuklaydi.
//
// ⚠️ base64 so'rov TANASIDA yuboriladi, query'da emas: 60 KB rasm ~80 KB
// base64 beradi va URL uzunligi chegarasidan oshadi.
//
// RPC2 da yuz qo'shish metodi YO'Q (qurilma bundle'ida faqat
// FaceInfoManager.getCaps bor) — shuning uchun bu CGI orqali ketadi va
// sekinroq (~450 ms, base64 hajmi tufayli).
func (c *Client) UploadFace(ctx context.Context, userID string, jpeg []byte) error {
	payload := map[string]any{
		"UserID": userID,
		"Info": map[string]any{
			"UserID":    userID,
			"PhotoData": []string{base64.StdEncoding.EncodeToString(jpeg)},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	target := c.dev.baseURL() + "/cgi-bin/FaceInfoManager.cgi?action=add"

	body, status, err := c.digestPost(ctx, target, "application/json", raw)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("yuz yuklanmadi: HTTP %d", status)
	}
	if !strings.Contains(body, "OK") {
		return fmt.Errorf("yuz yuklanmadi: %s", strings.TrimSpace(body))
	}
	return nil
}

// ---------------------------------------------------------------------- vaqt

// SetTime — qurilma soatini server vaqtiga o'rnatadi.
//
// ⚠️ Qurilma LOKAL vaqt kutadi (o'z TimeZone sozlamasiga ko'ra), UTC EMAS —
// shuning uchun `loc` server konfiguratsiyasidagi zona bo'lishi kerak.
// Qurilmalarda TimeZone=7 (Ashkhabad, UTC+5) va DST o'chiq, ya'ni
// Asia/Tashkent bilan mos.
func (c *Client) SetTime(ctx context.Context, loc *time.Location) error {
	body, err := c.cgi(ctx, "global.cgi", map[string]string{
		"action": "setCurrentTime",
		"time":   time.Now().In(loc).Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return err
	}
	if !strings.Contains(body, "OK") {
		return fmt.Errorf("vaqt o'rnatilmadi: %s", strings.TrimSpace(body))
	}
	return nil
}

// EnableNTP — avtomatik vaqt sinxronizatsiyasini yoqadi.
//
// ⚠️ `addr` NOM emas, IP bo'lishi SHART. Ichki DNS tashqi nomlarni o'ziga
// qaytaradi (`pool.ntp.org` -> 172.16.0.254), u esa NTP tarqatmaydi —
// natijada qurilma "NTP yoqilgan" holatda turadi, lekin hech qachon
// sinxronlanmaydi. Zavod sozlamasidagi `clock.isc.org` ham javob bermaydi
// (server o'lik), shuning uchun uni ishlatmang.
//
// periodMin — necha daqiqada bir sinxronlash (qurilma standarti 10).
func (c *Client) EnableNTP(ctx context.Context, addr string, periodMin int) error {
	body, err := c.cgi(ctx, "configManager.cgi", map[string]string{
		"action":           "setConfig",
		"NTP.Enable":       "true",
		"NTP.Address":      addr,
		"NTP.Port":         "123",
		"NTP.UpdatePeriod": strconv.Itoa(periodMin),
	})
	if err != nil {
		return err
	}
	if !strings.Contains(body, "OK") {
		return fmt.Errorf("NTP yoqilmadi: %s", strings.TrimSpace(body))
	}
	return nil
}

// NTPConfig — qurilmadagi hozirgi NTP sozlamasi.
type NTPConfig struct {
	Enabled bool   `json:"enabled"`
	Address string `json:"address"`
	Period  int    `json:"period_min"`
}

func (c *Client) NTPConfig(ctx context.Context) (NTPConfig, error) {
	body, err := c.cgi(ctx, "configManager.cgi", map[string]string{
		"action": "getConfig",
		"name":   "NTP",
	})
	if err != nil {
		return NTPConfig{}, err
	}

	period, _ := strconv.Atoi(strings.TrimSpace(ParseValue(body, "table.NTP.UpdatePeriod")))
	return NTPConfig{
		Enabled: strings.TrimSpace(ParseValue(body, "table.NTP.Enable")) == "true",
		Address: strings.TrimSpace(ParseValue(body, "table.NTP.Address")),
		Period:  period,
	}, nil
}

// ReplaceFace — mavjud yuzni yangisiga almashtiradi.
//
// ⚠️ `action=add` MAVJUD UserID uchun HTTP 400 qaytaradi — ustiga YOZMAYDI.
// Shu sababli avval eskisi o'chiriladi. Bunsiz rasm yangilanganda yangi rasm
// terminalga hech qachon bormaydi: bazada `failed` bo'lib qoladi, qurilmada
// esa eski yuz turaveradi.
//
// O'chirish xatosi e'tiborga olinmaydi — yuz bo'lmasa ham natija bir xil.
func (c *Client) ReplaceFace(ctx context.Context, userID string, jpeg []byte) error {
	_ = c.RemoveFace(ctx, userID)
	return c.UploadFace(ctx, userID, jpeg)
}

// drain — javob tanasini oxirigacha o'qib yopadi.
// Bunsiz ulanish qayta ishlatilmaydi va soket tugashi muammosi qaytadi.
func drain(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return "", err
	}
	return buf.String(), nil
}
