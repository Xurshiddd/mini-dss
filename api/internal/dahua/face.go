package dahua

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
