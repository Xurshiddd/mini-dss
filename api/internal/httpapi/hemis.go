package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"minidss/api/internal/hemis"
	"minidss/api/internal/syncsvc"
)

// optionsCache — filtr ro'yxatlari keshi.
//
// Ro'yxatlarni yig'ish ~12 ta HEMIS so'rovi (guruhlar 200 tadan sahifalanadi),
// mazmuni esa semestrda bir marta o'zgaradi. Panel har ochilganda HEMIS'ni
// urmasin.
type optionsCache struct {
	mu      sync.Mutex
	data    *hemis.FilterOptions
	fetched time.Time
}

const optionsTTL = 30 * time.Minute

// --------------------------------------------------------------- HEMIS sync

// hemisSyncRequest — POST /api/sync/hemis tanasi. Tana bo'sh bo'lsa:
// xodim ham, talaba ham, filtrsiz.
type hemisSyncRequest struct {
	Employees *bool               `json:"employees"`
	Students  *bool               `json:"students"`
	Filter    hemis.StudentFilter `json:"filter"`
}

func (a *API) startHemisSync(w http.ResponseWriter, r *http.Request) {
	var in hemisSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		fail(w, http.StatusBadRequest, "so'rov o'qilmadi")
		return
	}

	opts := syncsvc.HemisOptions{
		Employees: in.Employees == nil || *in.Employees,
		Students:  in.Students == nil || *in.Students,
		Filter:    in.Filter,
	}

	// Filtr xatosi (harfli kod, yarim passport juftligi) — foydalanuvchi
	// xatosi, shuning uchun 422.
	if err := opts.Filter.Validate(); err != nil {
		fail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if err := a.sync.StartHemisSync(a.cfg.HemisBase, a.cfg.HemisToken, opts); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	write(w, http.StatusAccepted, a.sync.HemisProgress())
}

func (a *API) hemisProgress(w http.ResponseWriter, r *http.Request) {
	write(w, http.StatusOK, map[string]any{
		"progress":   a.sync.HemisProgress(),
		"configured": a.cfg.HemisBase != "" && a.cfg.HemisToken != "",
	})
}

// ------------------------------------------------------------ filtr ma'lumoti

// hemisFilters — filtr uchun tanlov ro'yxatlari (klassifikatorlar, fakultetlar,
// yo'nalishlar, guruhlar).
func (a *API) hemisFilters(w http.ResponseWriter, r *http.Request) {
	if a.cfg.HemisBase == "" || a.cfg.HemisToken == "" {
		fail(w, http.StatusServiceUnavailable, "HEMIS sozlanmagan")
		return
	}

	a.hemisOptions.mu.Lock()
	defer a.hemisOptions.mu.Unlock()

	fresh := a.hemisOptions.data != nil &&
		time.Since(a.hemisOptions.fetched) < optionsTTL
	if !fresh || r.URL.Query().Get("refresh") == "1" {
		client := hemis.New(a.cfg.HemisBase, a.cfg.HemisToken)

		opts, err := client.FilterOptions(r.Context())
		if err != nil {
			// Kesh bor bo'lsa, eskisi hech narsadan yaxshi.
			if a.hemisOptions.data == nil {
				fail(w, http.StatusBadGateway, err.Error())
				return
			}
			write(w, http.StatusOK, a.hemisOptions.data)
			return
		}

		a.hemisOptions.data = opts
		a.hemisOptions.fetched = time.Now()
	}

	write(w, http.StatusOK, a.hemisOptions.data)
}

// hemisStudentCount — filtrga nechta talaba tushishini oldindan ko'rsatadi.
//
// Standart holatda son HEMIS so'rovi bo'yicha, ya'ni mahalliy saralanadigan
// filtrlar (turar joy, to'lov shakli) hisobga olinmaydi — javobda
// `approximate: true` keladi.
//
// ⚠️ `?exact=1` aniq sanaydi, lekin butun ro'yxatni tortadi (~15 s).
// Foydalanuvchi ATAYLAB so'raganda chaqirilsin.
func (a *API) hemisStudentCount(w http.ResponseWriter, r *http.Request) {
	if a.cfg.HemisBase == "" || a.cfg.HemisToken == "" {
		fail(w, http.StatusServiceUnavailable, "HEMIS sozlanmagan")
		return
	}

	var f hemis.StudentFilter
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		fail(w, http.StatusBadRequest, "so'rov o'qilmadi")
		return
	}
	if err := f.Validate(); err != nil {
		fail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	client := hemis.New(a.cfg.HemisBase, a.cfg.HemisToken)

	exact := r.URL.Query().Get("exact") == "1" && f.Local()

	count, err := client.CountStudents(r.Context(), f)
	if exact {
		count, err = client.CountStudentsExact(r.Context(), f)
	}
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{
		"count": count,
		// Mahalliy filtr tanlangan, lekin aniq sanalmagan bo'lsa — taxminiy.
		"approximate": f.Local() && !exact,
	})
}
