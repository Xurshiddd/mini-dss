package dahua

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

// Qurilmadagi yuz rasmini QAYTARIB O'QISH.
//
// ⚠️ O'qish RPC2 da, CGI da EMAS. `FaceInfoManager.cgi?action=find` (va
// `get`, `getFace`, `list`, `getCaps`, `export` — GET ham, JSON POST ham)
// bu firmware'da HTTP 400 qaytaradi, garchi shu CGI'ning `add` va `remove`
// action'lari ishlasa ham. Ya'ni CGI'da yozish bor, o'qish yo'q.
//
// Qurilmadan olingan HAQIQIY metodlar ro'yxati (`<service>.listMethod`):
//
//	FaceInfoManager: add get update remove clear startFind doFind stopFind
//	                 getCaps getFaceEigen returnUserPicture listMethod
//	AccessFace:      insertMulti list updateMulti removeAll removeMulti
//	                 startFind doFind stopFind attach/detachFaceDetection
//
// ⚠️ `system.listMethod` `name`/`service` parametrini E'TIBORSIZ qoldiradi va
// har doim `system` ning o'z metodlarini qaytaradi — xizmat metodlarini bilish
// uchun `<service>.listMethod` chaqirilsin.
//
// ⚠️ Nomlar orasidagi harf registri BIR XIL EMAS, va noto'g'risi jimgina
// e'tiborsiz qoladi yoki tushunarsiz xato beradi:
//
//	FaceInfoManager.doFind → token  offset count   (kichik), javob `info`
//	AccessFace.doFind      → Token  Offset Count   (katta), javob `Info`

// ------------------------------------------------------------- yuz YOZISH
//
// ⚠️ Yuz yozishning TO'G'RI yo'li — RPC2 `AccessFace`, CGI EMAS. Ilgari bu
// faqat CGI orqali ketardi ("RPC2 da yuz qo'shish metodi yo'q" degan xulosa
// `system.listMethod` dan olingan edi — u esa xizmat nomini e'tiborsiz
// qoldiradi). `AccessFace.listMethod` aksini ko'rsatadi.
//
// Jonli terminalda o'lchangan (2026-09-16, ASI7213Y):
//
//	CGI  FaceInfoManager.cgi?action=add  → 0.72–0.81 s
//	RPC2 AccessFace.insertMulti          → 0.44–0.50 s
//
// ⚠️ Vaqtdan ham MUHIMROG'I — ULANISHLAR SONI. Qurilma digest challenge'ida
// ulanishni yopadi (`Connection: close`), shuning uchun HAR BIR CGI so'rovi
// 3 TA YANGI TCP ulanish ochadi. ~2500 odamdan keyin terminal yangi ulanish
// qabul qilmay qo'yadi (dial timeout) va SOATLAB o'ziga kelmaydi — sync esa
// qolganlarning har biriga 10 soniya sarflab, hammasini "failed" qiladi.
// RPC2 esa bitta sessiya ulanishini qayta ishlatadi.
//
// ⚠️ `insertMulti` MAVJUD yuz ustiga yozmaydi — "Batch Process Error"
// beradi. Almashtirish uchun `updateMulti` (0.29 s).
//
// ⚠️ To'da bo'lib yozish deyarli foyda bermaydi: 1 ta → 469 ms/yuz,
// 4 talik to'da → 402 ms/yuz, 5 talik → 499 ms/yuz. Vaqt tarmoqda emas,
// qurilmaning yuzdan belgi (feature) ajratishida ketadi. Shu sababli
// to'da kichik: xato bo'lganda ayirib olish oson bo'lsin.

// FaceUpload — yuklanadigan bitta yuz.
type FaceUpload struct {
	UserID string
	JPEG   []byte
}

// FaceWriteBatch — bitta `insertMulti` so'roviga nechta yuz.
const FaceWriteBatch = 4

// InsertFaces — yangi yuzlarni yozadi (`AccessFace.insertMulti`).
//
// ⚠️ Parametr nomi `FaceList`. `FaceDataList` (o'qishdagi javob nomi)
// "Request invalid param!" beradi — yozish va o'qish nomlari har xil.
func (c *Client) InsertFaces(ctx context.Context, faces []FaceUpload) error {
	return c.writeFaces(ctx, "AccessFace.insertMulti", faces)
}

// UpdateFaces — qurilmada ALLAQACHON yuzi borlarni almashtiradi.
func (c *Client) UpdateFaces(ctx context.Context, faces []FaceUpload) error {
	return c.writeFaces(ctx, "AccessFace.updateMulti", faces)
}

// RemoveFaces — yuzlarni UserID bo'yicha o'chiradi (bitta so'rovda).
func (c *Client) RemoveFaces(ctx context.Context, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	_, err := c.rpc(ctx, "AccessFace.removeMulti", map[string]any{"UserIDList": userIDs}, 0)
	return err
}

func (c *Client) writeFaces(ctx context.Context, method string, faces []FaceUpload) error {
	if len(faces) == 0 {
		return nil
	}

	list := make([]map[string]any, 0, len(faces))
	for _, f := range faces {
		if !isJPEG(f.JPEG) {
			return fmt.Errorf("%s: rasm JPEG emas", f.UserID)
		}
		list = append(list, map[string]any{
			"UserID":    f.UserID,
			"PhotoData": []string{base64.StdEncoding.EncodeToString(f.JPEG)},
		})
	}

	_, err := c.rpc(ctx, method, map[string]any{"FaceList": list}, 0)
	return err
}

// RateLimited — qurilma yozish sur'atidan shikoyat qilyaptimi.
//
// ⚠️ Terminal ketma-ket yozuvda "MAX INSERT RATE EXCEEDED" qaytaradi. Bu
// yozuvning aybi emas: biroz kutib qayta urinilsa o'tadi. Xato deb
// belgilansa, odam sync'dan tushib qolardi.
func RateLimited(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "MAX INSERT RATE")
}

// RecordMissing — qurilma "bunday yozuv yo'q" deyaptimi ("NO RECORDS").
//
// ⚠️ Bazadagi `device_recno` eskirgan bo'lishi mumkin: terminal tozalangan,
// qayta yuklangan yoki yozuv boshqa yo'l bilan yo'qolgan. Bunday holatda
// kartani YANGIDAN yozish kerak — aks holda odam har sync'da
// "RecordUpdater.update: NO RECORDS" bilan abadiy `failed` bo'lib qolardi
// (jonli terminalda 48 ta odam shunday qolib ketdi).
func RecordMissing(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "NO RECORDS")
}

// Unreachable — qurilma umuman javob bermayaptimi (ulanish darajasidagi xato).
//
// Bunday xatoda odamni "failed" deb belgilash noto'g'ri — aybdor qurilma.
func Unreachable(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	msg := err.Error()
	for _, s := range []string{"dial tcp", "connection refused", "connection reset",
		"no route to host", "i/o timeout", "EOF", "broken pipe"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// JPEG boshlanishi — base64 to'g'ri dekodlanganini shundan bilamiz.
var jpegMagic = []byte{0xFF, 0xD8, 0xFF}

func isJPEG(b []byte) bool {
	return len(b) > 3 && b[0] == jpegMagic[0] && b[1] == jpegMagic[1] && b[2] == jpegMagic[2]
}

// FaceBatchSize — bitta `AccessFace.list` so'roviga nechta UserID beriladi.
//
// ⚠️ Chegara so'rovlar soni emas, JAVOB HAJMI: bitta rasm ~15–65 KB base64,
// ya'ni 10 ta ~0.6 MB. Kattaroq to'da javobni qurilma xotirasida yig'ishga
// majbur qiladi.
const FaceBatchSize = 10

// FetchFace — UserID bo'yicha qurilmadagi yuz rasmini oladi.
//
// Ikkinchi qaytish qiymati — ishlagan yo'l nomi (loglarda ko'rinsin).
// Ko'p odam uchun `FetchFaces` ishlatilsin: u bitta so'rovda bir nechtasini
// oladi.
func (c *Client) FetchFace(ctx context.Context, userID string) ([]byte, string, error) {
	resp, err := c.rpc(ctx, "FaceInfoManager.get", map[string]any{"UserID": userID}, 0)
	if err != nil {
		return nil, "", fmt.Errorf("yuz o'qilmadi: %w", err)
	}

	var out struct {
		Info struct {
			UserID    string   `json:"UserID"`
			PhotoData []string `json:"PhotoData"`
		} `json:"info"`
	}
	if err := json.Unmarshal(resp.Params, &out); err != nil {
		return nil, "", fmt.Errorf("yuz javobi o'qilmadi: %w", err)
	}
	if len(out.Info.PhotoData) == 0 {
		return nil, "", fmt.Errorf("qurilmada bu odamning yuzi yo'q (%s)", userID)
	}

	jpeg, ok := decodeBase64JPEG(out.Info.PhotoData[0])
	if !ok {
		return nil, "", fmt.Errorf("yuz JPEG emas (%s)", userID)
	}
	return jpeg, "FaceInfoManager.get", nil
}

// FetchFaces — bir nechta yuzni BITTA so'rovda oladi.
//
// ⚠️ Qurilmada yuzi yo'q UserID javobga umuman tushmaydi — xato ham
// bermaydi. Shuning uchun natija map: kim qaytmagani chaqiruvchida ko'rinadi.
func (c *Client) FetchFaces(ctx context.Context, userIDs []string) (map[string][]byte, error) {
	if len(userIDs) == 0 {
		return map[string][]byte{}, nil
	}

	resp, err := c.rpc(ctx, "AccessFace.list", map[string]any{"UserIDList": userIDs}, 0)
	if err != nil {
		return nil, fmt.Errorf("yuzlar o'qilmadi: %w", err)
	}

	var out struct {
		FaceDataList []struct {
			UserID    string   `json:"UserID"`
			PhotoData []string `json:"PhotoData"`
		} `json:"FaceDataList"`
	}
	if err := json.Unmarshal(resp.Params, &out); err != nil {
		return nil, fmt.Errorf("yuz javobi o'qilmadi: %w", err)
	}

	faces := make(map[string][]byte, len(out.FaceDataList))
	for _, entry := range out.FaceDataList {
		if len(entry.PhotoData) == 0 {
			continue
		}
		if jpeg, ok := decodeBase64JPEG(entry.PhotoData[0]); ok {
			faces[entry.UserID] = jpeg
		}
	}
	return faces, nil
}

// CountFaces — qurilmada nechta yuz saqlangan.
//
// ⚠️ Bu son foydalanuvchilar sonidan FARQ QILADI: yuzsiz karta ham bo'ladi.
func (c *Client) CountFaces(ctx context.Context) (int, error) {
	token, total, err := c.faceStartFind(ctx)
	if err != nil {
		return 0, err
	}
	c.faceStopFind(ctx, token)
	return total, nil
}

// FaceUserIDs — qurilmada yuzi BOR foydalanuvchilar ro'yxati.
//
// ⚠️ `startFind` `condition` parametrini e'tiborsiz qoldiradi (qanday
// berilmasin `Total` o'zgarmaydi) — filtrlash mumkin emas, faqat sahifalash.
func (c *Client) FaceUserIDs(ctx context.Context, offset, count int) ([]string, error) {
	token, _, err := c.faceStartFind(ctx)
	if err != nil {
		return nil, err
	}
	defer c.faceStopFind(ctx, token)

	resp, err := c.rpc(ctx, "AccessFace.doFind", map[string]any{
		"Token": token, "Offset": offset, "Count": count,
	}, 0)
	if err != nil {
		return nil, err
	}

	var out struct {
		Info []struct {
			UserID string `json:"UserID"`
		} `json:"Info"`
	}
	if err := json.Unmarshal(resp.Params, &out); err != nil {
		return nil, fmt.Errorf("yuz ro'yxati o'qilmadi: %w", err)
	}

	ids := make([]string, 0, len(out.Info))
	for _, e := range out.Info {
		if e.UserID != "" {
			ids = append(ids, e.UserID)
		}
	}
	return ids, nil
}

func (c *Client) faceStartFind(ctx context.Context) (token, total int, err error) {
	resp, err := c.rpc(ctx, "AccessFace.startFind", nil, 0)
	if err != nil {
		return 0, 0, err
	}

	var out struct {
		Token int `json:"Token"`
		Total int `json:"Total"`
	}
	if err := json.Unmarshal(resp.Params, &out); err != nil || out.Token == 0 {
		return 0, 0, fmt.Errorf("yuz qidiruvi ochilmadi")
	}
	return out.Token, out.Total, nil
}

// faceStopFind — qidiruv token'ini bo'shatadi.
//
// ⚠️ Token qurilma resursi: bo'shatilmasa sessiya davomida yig'ilib boradi.
func (c *Client) faceStopFind(ctx context.Context, token int) {
	_, _ = c.rpc(ctx, "AccessFace.stopFind", map[string]any{"Token": token}, 0)
}

func decodeBase64JPEG(raw string) ([]byte, bool) {
	// ⚠️ Qurilma base64'ni satrlarga bo'lib qaytarishi mumkin — bo'shliqlar
	// tashlanmasa dekodlash yiqiladi.
	cleaned := strings.NewReplacer("\r", "", "\n", "", " ", "", `"`, "").Replace(raw)
	if cleaned == "" {
		return nil, false
	}

	jpeg, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		if jpeg, err = base64.RawStdEncoding.DecodeString(cleaned); err != nil {
			return nil, false
		}
	}
	if !isJPEG(jpeg) {
		return nil, false
	}
	return jpeg, true
}

// FaceProbe — yuz o'qish yo'li hali ham ishlayotganini tekshiradi.
//
// Diagnostika uchun: qoralama yig'ish ishlamay qolsa, avval shu chaqirilsin.
// Metodlar ro'yxati QATTIQ belgilangan — bu yerdan ixtiyoriy RPC2 chaqiruv
// yo'lini ochib qo'ymaslik uchun.
func (c *Client) FaceProbe(ctx context.Context, userID string) map[string]any {
	out := map[string]any{"user_id": userID}

	// Qurilma metodlari — kutilgani bilan solishtirish uchun.
	if raw, err := c.Probe(ctx, "FaceInfoManager.listMethod", nil); err == nil {
		out["metodlar_FaceInfoManager"] = json.RawMessage(raw)
	}
	if raw, err := c.Probe(ctx, "AccessFace.listMethod", nil); err == nil {
		out["metodlar_AccessFace"] = json.RawMessage(raw)
	}

	if total, err := c.CountFaces(ctx); err != nil {
		out["yuzlar_soni"] = map[string]string{"error": err.Error()}
	} else {
		out["yuzlar_soni"] = total
	}

	// Bitta o'qish.
	if jpeg, how, err := c.FetchFace(ctx, userID); err != nil {
		out["bitta"] = map[string]string{"error": err.Error()}
	} else {
		out["bitta"] = map[string]any{"usul": how, "bayt": len(jpeg)}
	}

	// Ommaviy o'qish — qoralama yig'ish shu yo'ldan ketadi.
	ids, err := c.FaceUserIDs(ctx, 0, 3)
	if err != nil {
		out["royxat"] = map[string]string{"error": err.Error()}
		return out
	}
	out["royxat"] = ids

	faces, err := c.FetchFaces(ctx, ids)
	if err != nil {
		out["ommaviy"] = map[string]string{"error": err.Error()}
		return out
	}

	sizes := map[string]int{}
	for id, jpeg := range faces {
		sizes[id] = len(jpeg)
	}
	out["ommaviy"] = sizes

	return out
}
