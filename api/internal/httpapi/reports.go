package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"minidss/api/internal/store"
)

func (a *API) reportRoutes(r chi.Router) {
	r.Route("/api/reports", func(rr chi.Router) {
		rr.Post("/import", a.importEvents)
		rr.Get("/days", a.reportDays)
		rr.Get("/totals", a.reportTotals)
		rr.Get("/export", a.exportReport)
		rr.Get("/person/{id}", a.reportPerson)
	})
}

// reportRange — so'rovdagi `from`/`to` sanalari.
//
// Berilmasa — joriy kun. Sana Toshkent kuni sifatida tushuniladi.
func reportRange(r *http.Request) (time.Time, time.Time, error) {
	parse := func(key string, fallback time.Time) (time.Time, error) {
		v := r.URL.Query().Get(key)
		if v == "" {
			return fallback, nil
		}
		return time.Parse("2006-01-02", v)
	}

	today := time.Now().Format("2006-01-02")
	base, _ := time.Parse("2006-01-02", today)

	from, err := parse("from", base)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := parse("to", from)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to, nil
}

func (a *API) filterFrom(r *http.Request) (store.AttendanceFilter, error) {
	from, to, err := reportRange(r)
	if err != nil {
		return store.AttendanceFilter{}, err
	}

	limit := 500
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 5000 {
		limit = n
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	personID, _ := strconv.ParseInt(r.URL.Query().Get("person_id"), 10, 64)

	return store.AttendanceFilter{
		From:     from,
		To:       to,
		PersonID: personID,
		Query:    r.URL.Query().Get("q"),
		Limit:    limit,
		Offset:   offset,
	}, nil
}

// importEvents — terminallardan yangi yozuvlarni tortib oladi.
func (a *API) importEvents(w http.ResponseWriter, r *http.Request) {
	pages := 5
	if n, err := strconv.Atoi(r.URL.Query().Get("pages")); err == nil && n > 0 && n <= 500 {
		pages = n
	}

	total, errs := a.sync.ImportAllEvents(r.Context(), pages)

	if _, err := a.store.RelinkEvents(r.Context()); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{"yangi": total, "xatolar": errs})
}

func (a *API) reportDays(w http.ResponseWriter, r *http.Request) {
	f, err := a.filterFrom(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "sana YYYY-MM-DD bo'lsin")
		return
	}

	rows, err := a.store.AttendanceDays(r.Context(), a.cfg.Timezone.String(), f)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{
		"from": f.From.Format("2006-01-02"),
		"to":   f.To.Format("2006-01-02"),
		"rows": rows,
	})
}

func (a *API) reportTotals(w http.ResponseWriter, r *http.Request) {
	f, err := a.filterFrom(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "sana YYYY-MM-DD bo'lsin")
		return
	}

	rows, err := a.store.AttendanceTotals(r.Context(), a.cfg.Timezone.String(), f)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{
		"from": f.From.Format("2006-01-02"),
		"to":   f.To.Format("2006-01-02"),
		"rows": rows,
	})
}

// reportPerson — profil oynasi uchun: kunlar + xom hodisalar.
func (a *API) reportPerson(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	f, err := a.filterFrom(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "sana YYYY-MM-DD bo'lsin")
		return
	}
	f.PersonID = id

	person, err := a.store.Person(r.Context(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "odam topilmadi")
		return
	}

	tz := a.cfg.Timezone.String()

	days, err := a.store.AttendanceDays(r.Context(), tz, f)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	events, err := a.store.PersonEvents(r.Context(), tz, id, f.From, f.To)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{
		"person": person,
		"days":   days,
		"events": events,
	})
}
