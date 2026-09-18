package dahua

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// RPC2 (JSON-RPC) — TEZ yozish yo'li.
//
// O'lchangan: RPC2 42 ms/yozuv, legacy CGI 565 ms/yozuv (13× farq).
// Farq Digest'ning ikki borish-kelishida va har so'rovda yangi TCP
// ulanishda: qurilma digest challenge'ida ulanishni yopadi, RPC2 esa
// bitta sessiya ulanishini qayta ishlatadi.
//
// ⚠️ To'da bo'lib yozish tezlikni deyarli oshirmaydi (yuzlar uchun
// o'lchangan: 4 talik to'da atigi ~14% tejaydi) — vaqt qurilmaning
// ichida ketadi, tarmoqda emas.

func md5Sum(s string) []byte {
	sum := md5.Sum([]byte(s))
	return sum[:]
}

type rpcResponse struct {
	ID      int             `json:"id"`
	Session any             `json:"session"`
	Result  json.RawMessage `json:"result"`
	Params  json.RawMessage `json:"params"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (r rpcResponse) failed() bool {
	var ok bool
	if err := json.Unmarshal(r.Result, &ok); err == nil {
		return !ok
	}
	return false
}

func (r rpcResponse) sessionString() string {
	var s string
	if json.Unmarshal([]byte(fmt.Sprint(r.Session)), &s) == nil && s != "" {
		return s
	}
	if str, ok := r.Session.(string); ok {
		return str
	}
	if f, ok := r.Session.(float64); ok {
		return fmt.Sprintf("%.0f", f)
	}
	return ""
}

func (c *Client) rpcPost(ctx context.Context, path string, body map[string]any) (rpcResponse, error) {
	var out rpcResponse

	raw, err := json.Marshal(body)
	if err != nil {
		return out, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.dev.baseURL()+path, bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.http.Do(req)
	if err != nil {
		return out, err
	}

	text, err := drain(resp)
	if err != nil {
		return out, err
	}
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("RPC2 HTTP %d (%s)", resp.StatusCode, path)
	}

	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return out, fmt.Errorf("RPC2 javobi o'qilmadi: %w", err)
	}
	return out, nil
}

// Login — RPC2 sessiyasini ochadi.
//
// ⚠️ BIR MARTA chaqiriladi. Dahua bir necha muvaffaqiyatsiz urinishdan keyin
// admin akkauntni vaqtincha bloklaydi — xato bo'lsa QAYTA URINMANG.
//
// Protokol: parolsiz so'rov → challenge (realm + random) →
// MD5(user:realm:parol) katta harf → MD5(user:random:hash) → ikkinchi login.
func (c *Client) Login(ctx context.Context) error {
	if c.session != "" {
		return nil
	}

	c.requestID++
	challenge, err := c.rpcPost(ctx, "/RPC2_Login", map[string]any{
		"method": "global.login",
		"params": map[string]any{
			"userName":   c.dev.Username,
			"password":   "",
			"clientType": "Web3.0",
			"loginType":  "Direct",
		},
		"id": c.requestID,
	})
	if err != nil {
		return err
	}

	var p struct {
		Realm  string `json:"realm"`
		Random string `json:"random"`
	}
	if err := json.Unmarshal(challenge.Params, &p); err != nil || p.Realm == "" || p.Random == "" {
		return fmt.Errorf("RPC2 challenge olinmadi")
	}

	hash := md5Hex(c.dev.Username + ":" + p.Realm + ":" + c.dev.Password)
	password := md5Hex(c.dev.Username + ":" + p.Random + ":" + hash)

	c.requestID++
	result, err := c.rpcPost(ctx, "/RPC2_Login", map[string]any{
		"method": "global.login",
		"params": map[string]any{
			"userName":      c.dev.Username,
			"password":      password,
			"clientType":    "Web3.0",
			"loginType":     "Direct",
			"authorityType": "Default",
		},
		"id":      c.requestID,
		"session": challenge.sessionString(),
	})
	if err != nil {
		return err
	}

	if result.failed() {
		msg := "noma'lum sabab"
		if result.Error != nil {
			msg = result.Error.Message
		}
		return fmt.Errorf("RPC2 login rad etildi: %s — QAYTA URINMANG, akkaunt bloklanishi mumkin", msg)
	}

	if s := result.sessionString(); s != "" {
		c.session = s
	} else {
		c.session = challenge.sessionString()
	}
	return nil
}

func (c *Client) Logout(ctx context.Context) {
	if c.session == "" {
		return
	}
	_, _ = c.rpc(ctx, "global.logout", nil, 0)
	c.session = ""
}

// SessionGeneration — sessiya nechchi marta yangilanganini bildiradi.
//
// ⚠️ Obyekt handle'lari (RecordUpdater instance) SESSIYAGA bog'liq — sessiya
// yangilansa ular yaroqsiz bo'ladi va qayta olinishi kerak.
func (c *Client) SessionGeneration() int { return c.sessionGen }

// rpc — RPC2 metodini chaqiradi, sessiya tugasa avtomatik tiklaydi.
func (c *Client) rpc(ctx context.Context, method string, params map[string]any, object int) (rpcResponse, error) {
	if err := c.Login(ctx); err != nil {
		return rpcResponse{}, err
	}

	resp, err := c.rpcSend(ctx, method, params, object)
	if err != nil {
		return resp, err
	}

	// ⚠️ Sessiya uzun sikl o'rtasida tugaydi ("Invalid session in request
	// data!"). Bir marta qayta login qilib davom etamiz. Bu "parolni qayta
	// sinash" emas — parol to'g'ri ekani ma'lum, bloklanish xavfi yo'q.
	if resp.failed() && resp.Error != nil && strings.Contains(resp.Error.Message, "Invalid session") {
		c.session = ""
		c.sessionGen++

		if err := c.Login(ctx); err != nil {
			return resp, err
		}
		resp, err = c.rpcSend(ctx, method, params, object)
		if err != nil {
			return resp, err
		}
	}

	if resp.failed() {
		// ⚠️ Qurilma xatoni MATNSIZ, faqat kod bilan qaytarishi mumkin —
		// masalan `RecordUpdater.insert` dublikat UserID da (o'lchangan
		// 2026-09-18, 172.16.50.5). Kod yozilmasa `last_error` da quruq
		// "RPC2 ... xato:" qoladi va sabab bilinmaydi.
		msg := "noma'lum"
		if resp.Error != nil {
			msg = resp.Error.Message
			if msg == "" {
				msg = fmt.Sprintf("kod %d", resp.Error.Code)
			} else {
				msg = fmt.Sprintf("%s (kod %d)", msg, resp.Error.Code)
			}
		}
		return resp, fmt.Errorf("RPC2 %s xato: %s", method, msg)
	}
	return resp, nil
}

func (c *Client) rpcSend(ctx context.Context, method string, params map[string]any, object int) (rpcResponse, error) {
	c.requestID++
	body := map[string]any{
		"method":  method,
		"session": c.session,
		"id":      c.requestID,
	}
	if params != nil {
		body["params"] = params
	}
	if object != 0 {
		body["object"] = object
	}
	return c.rpcPost(ctx, "/RPC2", body)
}

// KeepAlive — sessiya tugab qolmasligi uchun davriy signal.
func (c *Client) KeepAlive(ctx context.Context) {
	_ = c.Alive(ctx)
}

// Alive — qurilma javob berayotganini YENGIL tekshiradi (RPC2 sessiya
// ulanishi orqali).
//
// ⚠️ `Ping` emas: u CGI orqali ketadi va har chaqiruvda uchta yangi TCP
// ulanish ochadi — qurilma allaqachon ulanishdan qiynalayotgan paytda
// aynan shuni qilmaslik kerak.
func (c *Client) Alive(ctx context.Context) error {
	_, err := c.rpc(ctx, "global.keepAlive", map[string]any{"timeout": 60, "active": true}, 0)
	return err
}

// RecordUpdaterInstance — yozuv jadvali uchun obyekt handle'i oladi.
func (c *Client) RecordUpdaterInstance(ctx context.Context, table string) (int, error) {
	resp, err := c.rpc(ctx, "RecordUpdater.factory.instance", map[string]any{"name": table}, 0)
	if err != nil {
		return 0, err
	}

	var object int
	if err := json.Unmarshal(resp.Result, &object); err != nil || object == 0 {
		return 0, fmt.Errorf("RecordUpdater instance olinmadi (%s)", table)
	}
	return object, nil
}

// Probe — diagnostika uchun O'QISH metodini chaqiradi.
//
// ⚠️ Ataylab faqat ma'lum metodlar ro'yxati bilan ishlatiladi (httpapi'da) —
// bu yerdan ixtiyoriy RPC2 metodini chaqirish yo'lini ochib qo'ymaslik uchun.
func (c *Client) Probe(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	resp, err := c.rpc(ctx, method, params, 0)
	if err != nil {
		return nil, err
	}
	if len(resp.Params) > 0 {
		return resp.Params, nil
	}
	return resp.Result, nil
}

// AccessUserFind — qurilmadagi foydalanuvchi ma'lumotini o'qiydi.
//
// Qurilmaning O'Z web UI'si aynan shu ketma-ketlikni ishlatadi
// (`app.js`: startFind → doFind → stopFind). `AccessUser.getMulti` kabi
// yangi metodlar bu firmware'da YO'Q ("Method not found"), garchi
// `system.listService` xizmatni ro'yxatda ko'rsatsa ham.
func (c *Client) AccessUserFind(ctx context.Context, condition map[string]any, count int) (json.RawMessage, error) {
	resp, err := c.rpc(ctx, "AccessUser.startFind", map[string]any{"Condition": condition}, 0)
	if err != nil {
		return nil, err
	}

	var start struct {
		Token int `json:"Token"`
		Total int `json:"Total"`
	}
	if err := json.Unmarshal(resp.Params, &start); err != nil {
		return nil, fmt.Errorf("startFind javobi o'qilmadi: %w", err)
	}

	defer func() {
		_, _ = c.rpc(ctx, "AccessUser.stopFind", map[string]any{"Token": start.Token}, 0)
	}()

	found, err := c.rpc(ctx, "AccessUser.doFind", map[string]any{
		"Token": start.Token, "Offset": 0, "Count": count,
	}, 0)
	if err != nil {
		return nil, err
	}
	return found.Params, nil
}

func (c *Client) RecordUpdaterDestroy(ctx context.Context, object int) {
	_, _ = c.rpc(ctx, "RecordUpdater.destroy", nil, object)
}

// CardRecord — foydalanuvchi yozuvi.
//
// ⚠️ `CardNo` MAJBURIY. Usiz qurilma 400 qaytaradi — garchi mavjud
// yozuvlarning hammasida uning qiymati BO'SH bo'lsa ham. Hujjatda yozilmagan;
// bu bitta detal butun sync yo'lini bloklab turgan edi.
func CardRecord(userID, name, validFrom, validTo string, door, timeSection int) map[string]any {
	if validFrom == "" {
		validFrom = time.Now().Format("2006-01-02") + " 00:00:00"
	}
	if validTo == "" {
		validTo = time.Now().AddDate(10, 0, 0).Format("2006-01-02") + " 23:59:59"
	}

	return map[string]any{
		"UserID":         userID,
		"CardName":       name,
		"CardNo":         userID, // ← usiz 400
		"CardStatus":     0,
		"CardType":       0,
		"UserType":       0,
		"Doors":          []int{door},
		"TimeSections":   []int{timeSection},
		"ValidDateStart": validFrom,
		"ValidDateEnd":   validTo,
	}
}

// RemoveUserByRecNo — foydalanuvchi yozuvini RecNo bo'yicha o'chiradi.
//
// Jonli terminalda o'lchangan (2026-09-16): 90 ms, yangi TCP ulanishsiz.
// CGI yo'li (`recordUpdater.cgi?action=remove`) ~700 ms va uchta ulanish.
func (c *Client) RemoveUserByRecNo(ctx context.Context, object, recNo int) error {
	_, err := c.rpc(ctx, "RecordUpdater.remove", map[string]any{"recno": recNo}, object)
	return err
}

// InsertUser — foydalanuvchi qo'shadi, RecNo qaytaradi. Tez yo'l.
func (c *Client) InsertUser(ctx context.Context, object int, record map[string]any) (int, error) {
	resp, err := c.rpc(ctx, "RecordUpdater.insert", map[string]any{"record": record}, object)
	if err != nil {
		return 0, err
	}

	var p struct {
		RecNo int `json:"recno"`
	}
	if err := json.Unmarshal(resp.Params, &p); err != nil || p.RecNo == 0 {
		return 0, fmt.Errorf("insert recno qaytarmadi")
	}
	return p.RecNo, nil
}

// UpdateUser — mavjud karta yozuvini RecNo bo'yicha yangilaydi.
//
// Jonli terminalda tekshirilgan (2026-09-16): 131 ms, nom o'zgargani
// `RecordUpdater.get {recno}` orqali tasdiqlangan. Yozuvni QAYTA O'QISH
// uchun ham shu `get` ishlatilsin — CGI `recordFinder` bilan bitta
// foydalanuvchini qidirish 10 ming yozuvli qurilmada 1.5–2.5 DAQIQA oladi.
func (c *Client) UpdateUser(ctx context.Context, object, recNo int, record map[string]any) error {
	_, err := c.rpc(ctx, "RecordUpdater.update", map[string]any{
		"recno":  recNo,
		"record": record,
	}, object)
	return err
}

// UserExists — foydalanuvchi yozuvi qurilmada HAQIQATAN bormi.
//
// ⚠️ Bazadagi `device_recno` yozuv qurilmada turganiga kafolat EMAS:
// terminal tozalansa yoki yozuv boshqa yo'l bilan yo'qolsa, baza buni
// bilmaydi. O'lchangan (2026-09-18, 192.168.30.216): 4 odamning RecNo si
// bazada turgan, qurilmada esa o'zlari yo'q edi.
//
// ⚠️ `startFind` ning `Condition` filtri AccessUser da ISHLAYDI —
// AccessFace da e'tiborsiz qolishidan farqli (ikki terminalda
// solishtirib tekshirilgan). Shunga qaramay UserID ni o'zimiz
// solishtiramiz: filtr jimgina ishlamay qolsa ham noto'g'ri "bor"
// javobini bermaymiz.
func (c *Client) UserExists(ctx context.Context, userID string) (bool, error) {
	raw, err := c.AccessUserFind(ctx, map[string]any{"UserID": userID}, 5)
	if err != nil {
		return false, err
	}

	var found struct {
		Info []struct {
			UserID string `json:"UserID"`
		} `json:"Info"`
	}
	if err := json.Unmarshal(raw, &found); err != nil {
		return false, fmt.Errorf("doFind javobi o'qilmadi: %w", err)
	}

	for _, u := range found.Info {
		if u.UserID == userID {
			return true, nil
		}
	}

	// ⚠️ Filtr jimgina e'tiborsiz qolsa javobda BEGONA yozuvlar keladi —
	// bunda "yo'q" deb xulosa qilib bo'lmaydi: chaqiruvchi odamni qayta
	// yozib, qurilmada dublikat yaratardi.
	if len(found.Info) > 0 {
		return false, fmt.Errorf("AccessUser.startFind filtri ishlamadi: %s so'raldi, %d ta begona yozuv keldi",
			userID, len(found.Info))
	}
	return false, nil
}
