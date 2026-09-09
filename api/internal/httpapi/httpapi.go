// Package httpapi — REST endpointlar.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"minidss/api/internal/auth"
	"minidss/api/internal/config"
	"minidss/api/internal/dahua"
	"minidss/api/internal/store"
	"minidss/api/internal/syncsvc"
)

type API struct {
	cfg       config.Config
	store     *store.Store
	auth      *auth.Service
	sync      *syncsvc.Service
	loginRate *rateLimiter

	// HEMIS filtr ro'yxatlari keshi.
	hemisOptions optionsCache
}

func New(cfg config.Config, st *store.Store, a *auth.Service, sy *syncsvc.Service) *API {
	return &API{
		cfg: cfg, store: st, auth: a, sync: sy,
		// IP bo'yicha 5 muvaffaqiyatsiz urinish / 15 daqiqa.
		loginRate: newRateLimiter(5, 15*time.Minute),
	}
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func fail(w http.ResponseWriter, status int, msg string) {
	write(w, status, map[string]string{"error": msg})
}

func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	// ⚠️ So'rov tanasi hajmini cheklaymiz — cheksiz body bilan xotira
	// to'ldirishning oldini oladi. 16 MB eng katta yuk (12 MB rasm)dan
	// kattaroq. Login o'zi 4 KB ga cheklangan (login handler).
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req.Body = http.MaxBytesReader(w, req.Body, 16<<20)
			next.ServeHTTP(w, req)
		})
	})
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
	}))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Post("/api/login", a.login)

	// Qolgan hamma narsa token talab qiladi.
	r.Group(func(pr chi.Router) {
		pr.Use(a.auth.Middleware)

		pr.Post("/api/logout", a.logout)
		pr.Get("/api/summary", a.summary)
		a.directoryRoutes(pr)
		a.reportRoutes(pr)
		a.draftRoutes(pr)

		pr.Route("/api/devices", func(dr chi.Router) {
			dr.Get("/", a.listDevices)
			dr.Post("/", a.createDevice)
			dr.Get("/{id}", a.getDevice)
			dr.Put("/{id}", a.updateDevice)
			dr.Delete("/{id}", a.deleteDevice)
			dr.Post("/{id}/test", a.testDevice)
			dr.Post("/{id}/time-fix", a.fixDeviceTime)
			dr.Get("/{id}/lookup", a.lookupDeviceUser)
			dr.Get("/{id}/events", a.deviceEvents)
			dr.Get("/{id}/caps", a.deviceCaps)
			dr.Get("/{id}/face-check", a.faceCheck)
			dr.Get("/{id}/face-probe", a.faceProbe)
			dr.Post("/{id}/sync", a.startSync)
			dr.Get("/{id}/sync", a.syncProgress)
			dr.Post("/{id}/wipe", a.wipeDevice)
			dr.Post("/{id}/clean-faces", a.cleanFaces)
		})

		pr.Route("/api/people", func(pr2 chi.Router) {
			pr2.Get("/", a.listPeople)
			pr2.Post("/", a.createPerson)
			pr2.Post("/bulk-status", a.bulkStatus)
			pr2.Post("/bulk-delete", a.bulkDelete)
			pr2.Get("/{id}", a.getPerson)
			pr2.Delete("/{id}", a.deletePerson)
			pr2.Get("/{id}/photo", a.personPhoto)
			pr2.Post("/{id}/photo", a.uploadPhoto)
			pr2.Delete("/{id}/photo", a.revertPhoto)
			pr2.Get("/{id}/photos", a.personPhotos)
			pr2.Post("/{id}/photos/{photoID}/use", a.usePhoto)
		})
	})

	return r
}

// ----------------------------------------------------------------------- auth

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	// ⚠️ Brute-force'ga qarshi: IP bo'yicha 5 urinish / 15 daqiqa. Chegaradan
	// oshsa parol umuman tekshirilmaydi. `r.RemoteAddr` — chi RealIP
	// middleware'i sabab to'g'ri IP (proxy ortida bo'lsa e'tibor bering:
	// audit bandi 19).
	ip := clientIP(r)
	if !a.loginRate.allow(ip) {
		fail(w, http.StatusTooManyRequests, "urinishlar chegarasidan oshdi — biroz kuting")
		return
	}

	// Body hajmini cheklaymiz — kichik JSON kutiladi.
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "so'rov o'qilmadi")
		return
	}

	token, expires, err := a.auth.Login(body.Username, body.Password)
	if err != nil {
		// Qaysi maydon xato ekanini aytmaymiz — foydalanuvchi nomini
		// taxmin qilishni osonlashtirmaslik uchun.
		fail(w, http.StatusUnauthorized, "login yoki parol noto'g'ri")
		return
	}

	// Muvaffaqiyatli login — bu IP uchun urinishlar hisobini tozalaymiz.
	a.loginRate.reset(ip)

	write(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": expires,
		"user":       body.Username,
	})
}

// logout — joriy token'ni server tomonda bekor qiladi.
//
// ⚠️ Ilgari chiqish faqat brauzer localStorage'ini tozalardi — token esa
// muddati (12 soat) o'tguncha amal qilaverardi. Endi u darhol yaroqsiz.
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	a.auth.Revoke(auth.TokenFromContext(r.Context()))
	write(w, http.StatusOK, map[string]string{"status": "chiqildi"})
}

func (a *API) summary(w http.ResponseWriter, r *http.Request) {
	sum, err := a.store.Summary(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, sum)
}

// -------------------------------------------------------------------- devices

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func (a *API) listDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := a.store.Devices(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Har bir qurilma uchun sync holatini ham beramiz.
	out := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		stats, _ := a.store.SyncStats(r.Context(), d.ID)
		out = append(out, map[string]any{
			"device":   d,
			"stats":    stats,
			"progress": a.sync.Progress(d.ID),
		})
	}
	write(w, http.StatusOK, out)
}

func (a *API) getDevice(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}
	d, err := a.store.Device(r.Context(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "qurilma topilmadi")
		return
	}
	stats, _ := a.store.SyncStats(r.Context(), id)
	write(w, http.StatusOK, map[string]any{"device": d, "stats": stats})
}

type deviceInput struct {
	Name      string  `json:"name"`
	IP        string  `json:"ip"`
	Port      int     `json:"port"`
	Username  string  `json:"username"`
	Password  string  `json:"password"`
	Direction string  `json:"direction"`
	Location  *string `json:"location"`
	IsActive  *bool   `json:"is_active"`
}

func (in deviceInput) validate(requirePassword bool) error {
	switch {
	case in.Name == "":
		return errors.New("nomi kerak")
	case in.IP == "":
		return errors.New("IP manzil kerak")
	case in.Username == "":
		return errors.New("foydalanuvchi nomi kerak")
	case requirePassword && in.Password == "":
		return errors.New("parol kerak")
	case in.Direction != "entry" && in.Direction != "exit":
		return errors.New("yo'nalish entry yoki exit bo'lishi kerak")
	}
	return nil
}

func (in deviceInput) toDevice() store.Device {
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	port := in.Port
	if port == 0 {
		port = 80
	}
	return store.Device{
		Name: in.Name, IP: in.IP, Port: port, Username: in.Username,
		Direction: in.Direction, Location: in.Location, IsActive: active,
	}
}

func (a *API) createDevice(w http.ResponseWriter, r *http.Request) {
	var in deviceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "so'rov o'qilmadi")
		return
	}
	if err := in.validate(true); err != nil {
		fail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Parol bazaga faqat shifrlangan holda tushadi.
	encrypted, err := a.auth.Encrypt(in.Password)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	d := in.toDevice()
	d.Password = encrypted

	created, err := a.store.CreateDevice(r.Context(), d)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusCreated, created)
}

func (a *API) updateDevice(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	var in deviceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "so'rov o'qilmadi")
		return
	}
	if err := in.validate(false); err != nil {
		fail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Parol bo'sh qoldirilsa eskisi saqlanadi.
	encrypted := ""
	if in.Password != "" {
		if encrypted, err = a.auth.Encrypt(in.Password); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	updated, err := a.store.UpdateDevice(r.Context(), id, in.toDevice(), encrypted)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, updated)
}

func (a *API) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}
	if err := a.store.DeleteDevice(r.Context(), id); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusNoContent, nil)
}

// testDevice — "Ulanishni tekshirish". FAQAT O'QISH.
func (a *API) testDevice(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	d, err := a.store.Device(r.Context(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "qurilma topilmadi")
		return
	}

	password, err := a.auth.Decrypt(d.Password)
	if err != nil {
		a.store.MarkDeviceError(r.Context(), id, err.Error())
		fail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// ⛔ Jonli oqim ochiq turganda qurilma yangi ulanish qabul qilmaydi.
	resume := a.sync.PauseDevice(id)
	defer resume()

	client := dahua.New(dahua.Device{
		IP: d.IP, Port: d.Port, Username: d.Username, Password: password,
	}, 30*time.Second)
	defer client.Close()

	ident, err := client.Identify(r.Context())
	if err != nil {
		a.store.MarkDeviceError(r.Context(), id, err.Error())
		fail(w, http.StatusBadGateway, err.Error())
		return
	}

	count, _ := client.CountUsers(r.Context())
	drift, _ := client.TimeDrift(r.Context(), a.cfg.Timezone)

	_ = a.store.MarkDeviceSeen(r.Context(), id, map[string]string{
		"model": ident.Model, "firmware": ident.Firmware, "serial": ident.Serial,
	})

	write(w, http.StatusOK, map[string]any{
		"identity":     ident,
		"users":        count,
		"time_drift_s": drift,
	})
}

// fixDeviceTime — soatni darhol to'g'rilaydi va NTP'ni yoqadi.
//
// Ikkalasi ham qilinadi: NTP uzoq muddatli yechim, lekin qurilma tarmog'idan
// UDP/123 yopiq bo'lsa ishlamaydi — o'shanda ham soat hech bo'lmasa shu
// daqiqada to'g'ri bo'lib qoladi va har sinxronizatsiyada yangilanadi.
func (a *API) fixDeviceTime(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
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
	}, 30*time.Second)
	defer client.Close()

	before, err := client.TimeDrift(r.Context(), a.cfg.Timezone)
	if err != nil {
		a.store.MarkDeviceError(r.Context(), id, err.Error())
		fail(w, http.StatusBadGateway, err.Error())
		return
	}

	if err := client.SetTime(r.Context(), a.cfg.Timezone); err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}

	// NTP xatosi butun amaliyotni yiqitmasin — soat allaqachon to'g'rilangan.
	ntpErr := ""
	if err := client.EnableNTP(r.Context(), a.cfg.NTPAddress, 10); err != nil {
		ntpErr = err.Error()
	}

	after, _ := client.TimeDrift(r.Context(), a.cfg.Timezone)
	ntp, _ := client.NTPConfig(r.Context())

	write(w, http.StatusOK, map[string]any{
		"drift_before_s": before,
		"drift_after_s":  after,
		"ntp":            ntp,
		"ntp_error":      ntpErr,
	})
}

// lookupDeviceUser — qurilmada odam QANDAY saqlanganini ko'rsatadi.
//
// "UI'da sync bo'lgan deb turibdi, lekin terminal tanimayapti" turidagi
// savollar uchun: bazadagi holat bilan qurilmadagi haqiqiy yozuvni
// solishtirish imkonini beradi. FAQAT O'QISH.
func (a *API) lookupDeviceUser(w http.ResponseWriter, r *http.Request) {
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
	}, 5*time.Minute)
	defer client.Close()

	recs, err := client.FindUser(r.Context(), userID)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{
		"user_id": userID,
		"count":   len(recs),
		"records": recs,
	})
}

// deviceEvents — qurilmadagi oxirgi kirish yozuvlari. FAQAT O'QISH.
func (a *API) deviceEvents(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	count := 20
	if n, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && n > 0 && n <= 200 {
		count = n
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

	offset := -1 // oxiridan
	if n, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && n >= 0 {
		offset = n
	}

	total, events, err := client.RecentEvents(r.Context(), offset, count)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{
		"total": total, "count": len(events), "events": events,
	})
}

// faceCheck — odamning yuzi qurilmada HAQIQATAN bormi.
//
// `FaceInfoManager.cgi?action=add` `OK` qaytarishi yuz saqlanganini
// ANGLATMAYDI. Qurilma caps'ida `IsSupportGetPhoto: 1` — demak saqlangan
// yuzni qaytib o'qish mumkin, va bu yagona ishonchli tekshiruv.
func (a *API) faceCheck(w http.ResponseWriter, r *http.Request) {
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

	out := map[string]any{"user_id": userID}

	// Yuzlar qaysi jadvalda? Nomini bilmasak, yuz bor-yo'qligini
	// o'lchashning iloji yo'q.
	tables := map[string]any{}
	for _, name := range []string{
		"AccessControlCard", "FaceInfo", "AccessFace", "FaceRecognition",
		"Face", "AccessControlFace", "FaceLib", "AccessControlFaceInfo",
	} {
		if n, err := client.TableSize(r.Context(), name); err == nil {
			tables[name] = n
		}
	}
	out["jadvallar"] = tables

	raw, err := client.AccessUserFind(r.Context(), map[string]any{"UserID": userID}, 5)
	if err != nil {
		out["xato"] = err.Error()
	} else {
		s := string(raw)
		if len(s) > 1200 {
			out["javob_hajmi"] = len(s)
			s = s[:1200]
		}
		out["natija"] = s
	}

	write(w, http.StatusOK, out)
}

// deviceCaps — qurilmaning yuz moduli imkoniyatlari. FAQAT O'QISH.
//
// Metodlar ro'yxati QATTIQ belgilangan — ixtiyoriy RPC2 chaqiruvi emas.
func (a *API) deviceCaps(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
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

	probes := []struct {
		name   string
		method string
		params map[string]any
	}{
		{"face", "FaceInfoManager.getCaps", nil},
		{"access", "AC.getCaps", map[string]any{"WantMethods": false, "WantCaps": true}},
		{"software", "magicBox.getSoftwareVersion", nil},
		{"services", "system.listService", nil},
		{"m_accessface", "system.listMethod", map[string]any{"name": "AccessFace"}},
		{"m_accessuser", "system.listMethod", map[string]any{"name": "AccessUser"}},
		{"m_faceinfo", "system.listMethod", map[string]any{"name": "FaceInfoManager"}},
	}

	out := map[string]any{}
	for _, p := range probes {
		raw, err := client.Probe(r.Context(), p.method, p.params)
		if err != nil {
			out[p.name] = map[string]string{"error": err.Error()}
			continue
		}
		out[p.name] = json.RawMessage(raw)
	}

	write(w, http.StatusOK, out)
}

func (a *API) startSync(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	var body struct {
		RetryFailed bool `json:"retry_failed"`
		Limit       int  `json:"limit"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := a.sync.Start(id, body.RetryFailed, body.Limit); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	write(w, http.StatusAccepted, a.sync.Progress(id))
}

// cleanFaces — qurilmada qolgan egasiz yuz shablonlarini tozalaydi.
// Hozir sync bo'lgan odamlarga tegmaydi.
func (a *API) cleanFaces(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	if err := a.sync.StartCleanFaces(id); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	write(w, http.StatusAccepted, a.sync.Progress(id))
}

func (a *API) syncProgress(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}
	write(w, http.StatusOK, a.sync.Progress(id))
}

func (a *API) wipeDevice(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	// ⛔ Qaytarib bo'lmaydigan operatsiya — tasdiq so'zi talab qilinadi.
	var body struct {
		Confirm string `json:"confirm"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Confirm != "WIPE" {
		fail(w, http.StatusPreconditionRequired,
			`tasdiqlash kerak: {"confirm":"WIPE"} — bu qaytarib bo'lmaydi`)
		return
	}

	if err := a.sync.Wipe(r.Context(), id); err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"status": "tozalandi"})
}

// --------------------------------------------------------------------- people

func (a *API) listPeople(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	depID, _ := strconv.ParseInt(q.Get("department_id"), 10, 64)

	people, total, err := a.store.People(r.Context(), store.PeopleFilter{
		Query:        q.Get("q"),
		Source:       q.Get("source"),
		PhotoStatus:  q.Get("photo_status"),
		PersonType:   q.Get("person_type"),
		DepartmentID: depID,
		Gender:       q.Get("gender"),
		Limit:        limit,
		Offset:       offset,
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{
		"data": people, "total": total, "limit": limit, "offset": offset,
	})
}

func (a *API) getPerson(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}
	p, err := a.store.Person(r.Context(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "topilmadi")
		return
	}
	write(w, http.StatusOK, p)
}

func (a *API) personPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	p, err := a.store.Person(r.Context(), id)
	if err != nil || p.PhotoPath == nil {
		http.NotFound(w, r)
		return
	}

	// filepath.Base — yo'lni chegaralaymiz, katalogdan chiqib ketmasin.
	full := filepath.Join(a.cfg.PhotoDir, filepath.Base(*p.PhotoPath))
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, filepath.Base(full), time.Time{}, f)
}

func (a *API) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	p, err := a.store.Person(r.Context(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "topilmadi")
		return
	}

	if err := r.ParseMultipartForm(12 << 20); err != nil {
		fail(w, http.StatusBadRequest, "fayl o'qilmadi")
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		fail(w, http.StatusBadRequest, "rasm yuborilmadi")
		return
	}
	defer file.Close()

	if header.Size > 10<<20 {
		fail(w, http.StatusRequestEntityTooLarge, "rasm 10 MB dan katta")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	// ⚠️ Fayl nomi HEMIS'nikidan FARQ QILADI (`<user_id>.jpg` emas).
	// Ilgari ikkalasi bir xil faylga yozardi va "Rasmlarni yuklash"
	// bosqichi qo'lda qo'yilgan rasmni baytma-bayt bosib ketardi — na
	// bazada, na diskda undan nom-nishon qolardi.
	name := fmt.Sprintf("%s-manual-%d.jpg", safePhotoName(p.UserID), time.Now().Unix())
	if err := os.WriteFile(filepath.Join(a.cfg.PhotoDir, name), data, 0o644); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Terminal talablariga mosligini tekshiramiz.
	result, err := validatePhoto(r.Context(), a.cfg.FaceAPI, data, name)

	status, reason := "pending", ""
	switch {
	case err != nil:
		// face-api ishlamayotgan bo'lsa rasmni "yaroqsiz" demaymiz — bu
		// bizning nosozligimiz, rasmning aybi emas.
		status = "pending"
	case result.Valid:
		status = "valid"
	default:
		status = "rejected"
		reason = joinReasons(result.Reasons)
	}

	metrics, _ := json.Marshal(result.Metrics)

	photoID, err := a.store.AddPersonPhoto(r.Context(), store.PersonPhotoInput{
		PersonID:     id,
		Source:       "manual",
		FileName:     name,
		Status:       status,
		RejectReason: reason,
		Metrics:      metrics,
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	// ⚠️ Terminalga endi AYNAN shu rasm ketadi va HEMIS oqimi bu odamning
	// rasmiga boshqa tegmaydi. `SetPhotoOverride` qayta yozishni ham
	// belgilaydi — bunsiz odam `synced` bo'lib qolaverardi va yangi rasm
	// terminalga hech qachon bormasdi.
	if err := a.store.SetPhotoOverride(r.Context(), id, &photoID); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _ := a.store.Person(r.Context(), id)
	write(w, http.StatusOK, map[string]any{
		"person":  updated,
		"metrics": result.Metrics,
		"reasons": result.Reasons,
	})
}

// revertPhoto — qo'lda qo'yilgan rasmni bekor qilib, HEMIS'nikiga qaytaradi.
//
// Rasm yozuvi va fayli O'CHMAYDI — tarixda qoladi va qayta tanlash mumkin.
func (a *API) revertPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	if err := a.store.SetPhotoOverride(r.Context(), id, nil); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _ := a.store.Person(r.Context(), id)
	write(w, http.StatusOK, updated)
}

// personPhotos — odamning rasm tarixi (qo'lda yuklangan va terminaldan olingan).
func (a *API) personPhotos(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	photos, err := a.store.PersonPhotos(r.Context(), id)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, photos)
}

// usePhoto — tarixdagi rasmni qaytadan amaldagi qilib qo'yadi.
func (a *API) usePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	photoID, err := strconv.ParseInt(chi.URLParam(r, "photoID"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "rasm id yaroqsiz")
		return
	}

	photo, err := a.store.PersonPhotoByID(r.Context(), photoID)
	if err != nil {
		fail(w, http.StatusNotFound, "rasm topilmadi")
		return
	}

	// ⚠️ Boshqa odamning rasmini biriktirib bo'lmaydi — aks holda bitta
	// yuz ikki foydalanuvchiga bog'lanib, terminal tanishda chalkashardi.
	if photo.PersonID != id {
		fail(w, http.StatusForbidden, "bu rasm boshqa odamniki")
		return
	}

	if err := a.store.SetPhotoOverride(r.Context(), id, &photoID); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _ := a.store.Person(r.Context(), id)
	write(w, http.StatusOK, updated)
}
