package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"minidss/api/internal/dahua"
	"minidss/api/internal/store"
)

// Qoralamalar — terminaldan olingan yuz rasmlari.
//
// ⚠️ Qoralama hech kimga tegishli emas: u alohida jadvalda va alohida
// katalogda yotadi. Odamga biriktirilgandagina uning rasmiga aylanadi va
// keyingi sync'da terminalga ketadi.

func (a *API) draftRoutes(r chi.Router) {
	r.Route("/api/drafts", func(dr chi.Router) {
		dr.Get("/", a.listDrafts)
		dr.Get("/stats", a.draftStats)
		dr.Post("/auto-attach", a.autoAttachDrafts)
		dr.Post("/bulk-delete", a.bulkDeleteDrafts)
		dr.Get("/{id}/photo", a.draftPhoto)
		dr.Post("/{id}/attach", a.attachDraft)
		dr.Post("/{id}/detach", a.detachDraft)
		dr.Delete("/{id}", a.deleteDraft)
	})

	r.Post("/api/sync/drafts", a.startDraftPull)
	r.Get("/api/sync/drafts", a.draftPullProgress)
}

func (a *API) listDrafts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	drafts, total, err := a.store.Drafts(r.Context(), store.DraftFilter{
		Query:    q.Get("q"),
		Status:   q.Get("status"),
		Attached: q.Get("attached"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{
		"data": drafts, "total": total, "limit": limit, "offset": offset,
	})
}

func (a *API) draftStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.store.DraftStats(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, stats)
}

func (a *API) draftPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	d, err := a.store.Draft(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// filepath.Base — yo'lni chegaralaymiz, katalogdan chiqib ketmasin.
	f, err := os.Open(filepath.Join(a.sync.DraftDir(), filepath.Base(d.FileName)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, filepath.Base(d.FileName), time.Time{}, f)
}

// attachDraft — qoralamani odamga biriktiradi.
//
// Fayl NUSXALANADI: qoralama keyin o'chirilsa ham odamning rasmi joyida
// qolishi kerak.
func (a *API) attachDraft(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	var in struct {
		PersonID int64 `json:"person_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.PersonID == 0 {
		fail(w, http.StatusUnprocessableEntity, "person_id kerak")
		return
	}

	draft, err := a.store.Draft(r.Context(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "qoralama topilmadi")
		return
	}

	person, err := a.store.Person(r.Context(), in.PersonID)
	if err != nil {
		fail(w, http.StatusNotFound, "odam topilmadi")
		return
	}

	fileName, err := a.copyDraftFile(draft, person.UserID)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	if _, err := a.store.AttachDraft(r.Context(), id, in.PersonID, fileName); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _ := a.store.Person(r.Context(), in.PersonID)
	write(w, http.StatusOK, map[string]any{"person": updated, "draft_id": id})
}

// copyDraftFile — qoralama faylini odam rasmlari katalogiga nusxalaydi.
func (a *API) copyDraftFile(draft store.Draft, userID string) (string, error) {
	src, err := os.Open(filepath.Join(a.sync.DraftDir(), filepath.Base(draft.FileName)))
	if err != nil {
		return "", fmt.Errorf("qoralama fayli topilmadi: %w", err)
	}
	defer src.Close()

	name := fmt.Sprintf("%s-terminal-%d.jpg", safePhotoName(userID), time.Now().UnixNano())

	dst, err := os.OpenFile(filepath.Join(a.cfg.PhotoDir, name),
		os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return name, nil
}

func (a *API) detachDraft(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}
	if err := a.store.DetachDraft(r.Context(), id); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"status": "bekor qilindi"})
}

// autoAttachDrafts — mos kelgan qoralamalarni ommaviy biriktiradi.
//
// ⚠️ Faqat BIR MA'NOLI mosliklar (`AutoMatches`) va faqat rasmi yo'q yoki
// yaroqsiz odamlar. `dry_run` bilan avval ko'rib olish mumkin — bu amal
// terminalga yuz yozadi va xatosini qaytarish qimmatga tushadi.
func (a *API) autoAttachDrafts(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DryRun bool `json:"dry_run"`
		Limit  int  `json:"limit"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)

	if in.Limit <= 0 || in.Limit > 20000 {
		in.Limit = 20000
	}

	matches, err := a.store.AutoMatches(r.Context(), in.Limit)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	preview := make([]map[string]any, 0, len(matches))
	for _, m := range matches {
		preview = append(preview, map[string]any{
			"draft_id": m.DraftID, "person_id": m.PersonID,
			"user_id": m.UserID, "person": m.Person,
		})
	}

	if in.DryRun {
		write(w, http.StatusOK, map[string]any{
			"matched": len(matches), "attached": 0, "preview": preview,
		})
		return
	}

	attached, failed := 0, 0
	for _, m := range matches {
		draft, err := a.store.Draft(r.Context(), m.DraftID)
		if err != nil {
			failed++
			continue
		}

		person, err := a.store.Person(r.Context(), m.PersonID)
		if err != nil {
			failed++
			continue
		}

		fileName, err := a.copyDraftFile(draft, person.UserID)
		if err != nil {
			failed++
			continue
		}

		if _, err := a.store.AttachDraft(r.Context(), m.DraftID, m.PersonID, fileName); err != nil {
			failed++
			continue
		}
		attached++
	}

	write(w, http.StatusOK, map[string]any{
		"matched": len(matches), "attached": attached, "failed": failed,
		"preview": preview,
	})
}

func (a *API) deleteDraft(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}
	if _, err := a.store.DeleteDrafts(r.Context(), []int64{id}); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusNoContent, nil)
}

func (a *API) bulkDeleteDrafts(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.IDs) == 0 {
		fail(w, http.StatusUnprocessableEntity, "hech narsa tanlanmadi")
		return
	}

	n, err := a.store.DeleteDrafts(r.Context(), in.IDs)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"deleted": n})
}

// startDraftPull — terminaldan yuzlarni yig'ishni boshlaydi.
//
// `device_id` berilmasa terminal avtomatik tanlanadi: yozuvi eng ko'pi.
func (a *API) startDraftPull(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeviceID int64 `json:"device_id"`
		Limit    int   `json:"limit"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)

	if err := a.sync.StartDraftPull(a.cfg.FaceAPI, in.DeviceID, in.Limit); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	write(w, http.StatusAccepted, a.sync.DraftProgress())
}

func (a *API) draftPullProgress(w http.ResponseWriter, r *http.Request) {
	write(w, http.StatusOK, a.sync.DraftProgress())
}

// faceProbe — terminaldan yuzni QAYTIB O'QISH yo'lini aniqlaydi.
//
// ⚠️ Bu firmware'da yuzni o'qish TASDIQLANMAGAN (device-probe/API-MATRIX.md,
// 8.2-band). Qoralama yig'ish shu yo'lga tayanadi, shuning uchun jonli
// terminalda avval SHU chaqirilsin: javobda qaysi action rasm qaytargani
// ko'rinadi. FAQAT O'QISH.
func (a *API) faceProbe(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		fail(w, http.StatusBadRequest, "user_id kerak")
		return
	}

	d, err := a.store.Device(r.Context(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "qurilma topilmadi")
		return
	}

	password, err := a.auth.Decrypt(d.Password)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// ⛔ Jonli oqim ochiq turganda qurilma yangi ulanish qabul qilmaydi.
	resume := a.sync.PauseDevice(id)
	defer resume()

	client := dahua.New(dahua.Device{
		IP: d.IP, Port: d.Port, Username: d.Username, Password: password,
	}, time.Minute)
	defer client.Close()
	defer client.Logout(r.Context())

	write(w, http.StatusOK, client.FaceProbe(r.Context(), userID))
}

// safePhotoName — UserID fayl nomiga aylanadi.
//
// ⚠️ UserID ixtiyoriy alfanumerik satr (raqam emas): `/` yoki `..` tushsa
// fayl rasm katalogidan chiqib ketardi.
func safePhotoName(userID string) string {
	out := make([]rune, 0, len(userID))
	for _, r := range userID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}
