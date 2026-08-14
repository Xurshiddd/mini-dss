package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"minidss/api/internal/store"
)

// ------------------------------------------------------------------ bo'limlar

func (a *API) listDepartments(w http.ResponseWriter, r *http.Request) {
	deps, err := a.store.Departments(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, deps)
}

type departmentInput struct {
	Name     string `json:"name"`
	ParentID *int64 `json:"parent_id"`
	Kind     string `json:"kind"`
}

func (a *API) createDepartment(w http.ResponseWriter, r *http.Request) {
	var in departmentInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
		fail(w, http.StatusUnprocessableEntity, "bo'lim nomi kerak")
		return
	}

	var kind *string
	if in.Kind != "" {
		kind = &in.Kind
	}

	d, err := a.store.CreateDepartment(r.Context(), in.Name, in.ParentID, kind)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusCreated, d)
}

func (a *API) updateDepartment(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}

	var in departmentInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
		fail(w, http.StatusUnprocessableEntity, "bo'lim nomi kerak")
		return
	}

	d, err := a.store.UpdateDepartment(r.Context(), id, in.Name, in.ParentID)
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	write(w, http.StatusOK, d)
}

func (a *API) deleteDepartment(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}
	if err := a.store.DeleteDepartment(r.Context(), id); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusNoContent, nil)
}

// ------------------------------------------------------- odam qo'lda qo'shish

type personInput struct {
	UserID        string  `json:"user_id"`
	FullName      string  `json:"full_name"`
	PersonType    string  `json:"person_type"`
	DepartmentID  *int64  `json:"department_id"`
	Gender        *string `json:"gender"`
	BirthDate     *string `json:"birth_date"` // "2001-05-14"
	Status        *string `json:"status"`
	AccessStatus  string  `json:"access_status"` // permanent | temporary
	ValidTo       *string `json:"valid_to"`      // "2026-12-31", temporary uchun
	StaffPosition *string `json:"staff_position"`
	Specialty     *string `json:"specialty"`
	StudentGroup  *string `json:"student_group"`
	LevelName     *string `json:"level_name"`
	IsActive      *bool   `json:"is_active"`
}

func (a *API) createPerson(w http.ResponseWriter, r *http.Request) {
	var in personInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "so'rov o'qilmadi")
		return
	}

	switch {
	case in.UserID == "":
		fail(w, http.StatusUnprocessableEntity, "UserID kerak — terminalga shu ID yoziladi")
		return
	case in.FullName == "":
		fail(w, http.StatusUnprocessableEntity, "ism kerak")
		return
	case in.PersonType != "employee" && in.PersonType != "student" && in.PersonType != "other":
		fail(w, http.StatusUnprocessableEntity, "tur employee, student yoki other bo'lishi kerak")
		return
	}

	var birth *time.Time
	if in.BirthDate != nil && *in.BirthDate != "" {
		t, err := time.Parse("2006-01-02", *in.BirthDate)
		if err != nil {
			fail(w, http.StatusUnprocessableEntity, "tug'ilgan sana YYYY-MM-DD bo'lsin")
			return
		}
		birth = &t
	}

	// Vaqtincha kirish uchun muddat. Bo'sh qoldirilsa cheksiz turadi.
	var validTo *time.Time
	if in.AccessStatus == "temporary" && in.ValidTo != nil && *in.ValidTo != "" {
		t, err := time.Parse("2006-01-02", *in.ValidTo)
		if err != nil {
			fail(w, http.StatusUnprocessableEntity, "muddat sanasi noto'g'ri")
			return
		}
		validTo = &t
	}

	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}

	// ⚠️ Muddati allaqachon o'tgan bo'lsa darhol faolsiz qilamiz — aks holda
	// keyingi tekshiruvgacha terminalga yuborilib qolishi mumkin.
	if validTo != nil && validTo.Before(time.Now().Truncate(24*time.Hour)) {
		active = false
	}

	id, created, err := a.store.UpsertPerson(r.Context(), store.PersonInput{
		Source:        "manual",
		PersonType:    in.PersonType,
		UserID:        in.UserID,
		FullName:      in.FullName,
		Gender:        in.Gender,
		BirthDate:     birth,
		DepartmentID:  in.DepartmentID,
		Status:        in.Status,
		AccessStatus:  in.AccessStatus,
		ValidTo:       validTo,
		StaffPosition: in.StaffPosition,
		Specialty:     in.Specialty,
		StudentGroup:  in.StudentGroup,
		LevelName:     in.LevelName,
		IsActive:      active,
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	person, _ := a.store.Person(r.Context(), id)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	write(w, status, person)
}

// -------------------------------------------------- ommaviy holat o'zgartirish

func (a *API) bulkStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs      []int64 `json:"ids"`
		IsActive bool    `json:"is_active"`
		// `all: true` bo'lsa — FILTRGA tushgan hammasi. UI'da "hammasini
		// tanlash" shu orqali ishlaydi, 10 000 ta id yubormaslik uchun.
		All    bool `json:"all"`
		Filter struct {
			Query        string `json:"q"`
			Source       string `json:"source"`
			PhotoStatus  string `json:"photo_status"`
			PersonType   string `json:"person_type"`
			DepartmentID int64  `json:"department_id"`
			Gender       string `json:"gender"`
		} `json:"filter"`
	}

	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "so'rov o'qilmadi")
		return
	}

	var (
		affected int64
		err      error
	)

	if in.All {
		affected, err = a.store.SetPeopleActiveByFilter(r.Context(), store.PeopleFilter{
			Query:        in.Filter.Query,
			Source:       in.Filter.Source,
			PhotoStatus:  in.Filter.PhotoStatus,
			PersonType:   in.Filter.PersonType,
			DepartmentID: in.Filter.DepartmentID,
			Gender:       in.Filter.Gender,
		}, in.IsActive)
	} else {
		if len(in.IDs) == 0 {
			fail(w, http.StatusUnprocessableEntity, "hech kim tanlanmadi")
			return
		}
		affected, err = a.store.SetPeopleActive(r.Context(), in.IDs, in.IsActive)
	}

	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"affected": affected})
}

// ------------------------------------------------------------ odam o'chirish

// deletePerson / bulkDelete — odamni o'chirishga belgilaydi.
//
// Darhol o'chirilmaydi: avval har bir terminaldan olib tashlanishi kerak.
// Qurilma o'chiq bo'lsa bu keyingi sync'ga qoladi. Baza yozuvi terminallardan
// to'liq tozalangandan keyin `PurgeDeletedPeople` orqali yo'qoladi.
//
// ⚠️ Bazadan darhol o'chirib bo'lmaydi: `device_person_sync` yozuvi ham
// yo'qoladi va odamni terminaldan olib tashlash imkoni qolmaydi — u bazada
// yo'q, lekin eshikni ochaveradi.
func (a *API) deletePerson(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "id yaroqsiz")
		return
	}
	a.markDeleted(w, r, []int64{id})
}

func (a *API) bulkDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "so'rov o'qilmadi")
		return
	}
	if len(in.IDs) == 0 {
		fail(w, http.StatusUnprocessableEntity, "hech kim tanlanmadi")
		return
	}
	// ⚠️ `all + filter` ATAYLAB qo'llab-quvvatlanmaydi: bitta noto'g'ri
	// filtr bilan butun ro'yxatni o'chirib yuborish juda oson bo'lardi.
	a.markDeleted(w, r, in.IDs)
}

func (a *API) markDeleted(w http.ResponseWriter, r *http.Request, ids []int64) {
	affected, err := a.store.SoftDeletePerson(r.Context(), ids)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Hech qaysi terminalga yozilmaganlar shu yerdayoq o'chadi — ular uchun
	// sync kutishning hojati yo'q.
	purged, err := a.store.PurgeDeletedPeople(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	write(w, http.StatusOK, map[string]any{
		"affected": affected,
		"purged":   purged,
		"pending":  affected - purged,
	})
}

// --------------------------------------------------------------- HEMIS sync

func (a *API) startHemisSync(w http.ResponseWriter, r *http.Request) {
	if err := a.sync.StartHemisSync(a.cfg.HemisBase, a.cfg.HemisToken); err != nil {
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

func (a *API) startPhotoFetch(w http.ResponseWriter, r *http.Request) {
	if err := a.sync.StartPhotoFetch(a.cfg.FaceAPI); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	write(w, http.StatusAccepted, a.sync.PhotoProgress())
}

func (a *API) photoProgress(w http.ResponseWriter, r *http.Request) {
	write(w, http.StatusOK, a.sync.PhotoProgress())
}

// --------------------------------------------------------------- sync loglari

func (a *API) syncRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	runs, err := a.store.SyncRuns(r.Context(), r.URL.Query().Get("kind"), limit)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, runs)
}

// syncAll — barcha faol qurilmalarga sync boshlaydi.
//
// ⚠️ Har qurilma o'z goroutine'ida ketadi — bu XAVFSIZ, chunki cheklov
// qurilma ICHIDA (bittaga bitta jarayon), qurilmalar orasida emas.
func (a *API) syncAll(w http.ResponseWriter, r *http.Request) {
	devices, err := a.store.ActiveDevices(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	started, busy := 0, 0
	for _, d := range devices {
		if err := a.sync.Start(d.ID, false, 0); err != nil {
			busy++
			continue
		}
		started++
	}

	write(w, http.StatusAccepted, map[string]any{
		"started": started, "busy": busy, "devices": len(devices),
	})
}

func (a *API) directoryRoutes(r chi.Router) {
	r.Route("/api/departments", func(dr chi.Router) {
		dr.Get("/", a.listDepartments)
		dr.Post("/", a.createDepartment)
		dr.Put("/{id}", a.updateDepartment)
		dr.Delete("/{id}", a.deleteDepartment)
	})

	r.Post("/api/sync/hemis", a.startHemisSync)
	r.Get("/api/sync/hemis", a.hemisProgress)
	r.Post("/api/sync/photos", a.startPhotoFetch)
	r.Get("/api/sync/photos", a.photoProgress)
	r.Post("/api/sync/devices", a.syncAll)
	r.Get("/api/sync/runs", a.syncRuns)
}
