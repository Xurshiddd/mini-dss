package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ------------------------------------------------------------------ bo'limlar

// Department — ichma-ich bo'lim. HEMIS'da fakultet → kafedra, markaz → bo'lim
// shaklida keladi; qo'lda ham qo'shish mumkin (o'shanda ExternalID bo'sh).
type Department struct {
	ID         int64   `json:"id"`
	ExternalID *int64  `json:"external_id"`
	Code       *string `json:"code"`
	Name       string  `json:"name"`
	ParentID   *int64  `json:"parent_id"`
	Kind       *string `json:"kind"`
	IsActive   bool    `json:"is_active"`
	People     int     `json:"people"` // shu bo'limdagi odamlar soni
}

func (s *Store) Departments(ctx context.Context) ([]Department, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.external_id, d.code, d.name, d.parent_id, d.kind, d.is_active,
		       (SELECT count(*) FROM people p WHERE p.department_id = d.id)
		FROM departments d
		ORDER BY d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Department
	for rows.Next() {
		var d Department
		if err := rows.Scan(&d.ID, &d.ExternalID, &d.Code, &d.Name, &d.ParentID,
			&d.Kind, &d.IsActive, &d.People); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) CreateDepartment(ctx context.Context, name string, parentID *int64, kind *string) (Department, error) {
	var d Department
	err := s.pool.QueryRow(ctx, `
		INSERT INTO departments (name, parent_id, kind)
		VALUES ($1, $2, $3)
		RETURNING id, external_id, code, name, parent_id, kind, is_active`,
		name, parentID, kind).
		Scan(&d.ID, &d.ExternalID, &d.Code, &d.Name, &d.ParentID, &d.Kind, &d.IsActive)
	return d, err
}

func (s *Store) UpdateDepartment(ctx context.Context, id int64, name string, parentID *int64) (Department, error) {
	// ⚠️ O'zini o'ziga ota qilib bo'lmaydi — daraxt buziladi.
	if parentID != nil && *parentID == id {
		return Department{}, fmt.Errorf("bo'lim o'zining ichida bo'la olmaydi")
	}

	var d Department
	err := s.pool.QueryRow(ctx, `
		UPDATE departments SET name = $2, parent_id = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, external_id, code, name, parent_id, kind, is_active`,
		id, name, parentID).
		Scan(&d.ID, &d.ExternalID, &d.Code, &d.Name, &d.ParentID, &d.Kind, &d.IsActive)
	return d, err
}

func (s *Store) DeleteDepartment(ctx context.Context, id int64) error {
	// Odamlar va ichki bo'limlar `ON DELETE SET NULL` bilan bo'shatiladi —
	// ular yo'qolmaydi, faqat bo'limsiz qoladi.
	_, err := s.pool.Exec(ctx, `DELETE FROM departments WHERE id = $1`, id)
	return err
}

// UpsertDepartment — HEMIS'dan kelgan bo'limni saqlaydi (external_id bo'yicha).
func (s *Store) UpsertDepartment(ctx context.Context, externalID int64, code, name, kind string,
	parentExternalID *int64) (int64, error) {

	// Ota bo'lim allaqachon saqlangan bo'lsa darhol bog'laymiz; bo'lmasa
	// havolani xom holda qoldiramiz va sync oxirida `RelinkDepartments`
	// uni tiklaydi.
	var parentID *int64
	if parentExternalID != nil {
		var pid int64
		if err := s.pool.QueryRow(ctx,
			`SELECT id FROM departments WHERE external_id = $1`, *parentExternalID).Scan(&pid); err == nil {
			parentID = &pid
		}
	}

	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO departments (external_id, code, name, kind, parent_id, parent_external_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (external_id) WHERE external_id IS NOT NULL
		DO UPDATE SET name = EXCLUDED.name, code = EXCLUDED.code,
		              kind = EXCLUDED.kind,
		              parent_id = COALESCE(EXCLUDED.parent_id, departments.parent_id),
		              parent_external_id = COALESCE(EXCLUDED.parent_external_id,
		                                            departments.parent_external_id),
		              updated_at = now()
		RETURNING id`,
		externalID, nullIfEmpty(code), name, nullIfEmpty(kind), parentID, parentExternalID).Scan(&id)
	return id, err
}

// RelinkDepartments — xom `parent_external_id` havolalarini `parent_id` ga
// aylantiradi. Sync oxirida chaqiriladi, chunki ota bo'lim bolasidan keyin
// kelgan bo'lishi mumkin.
func (s *Store) RelinkDepartments(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE departments d SET parent_id = p.id, updated_at = now()
		FROM departments p
		WHERE d.parent_external_id IS NOT NULL
		  AND p.external_id = d.parent_external_id
		  AND p.id <> d.id
		  AND (d.parent_id IS NULL OR d.parent_id <> p.id)`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func nullIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

// -------------------------------------------------------------- odam yozuvi

// PersonInput — HEMIS'dan yoki qo'lda kelgan odam ma'lumoti.
type PersonInput struct {
	ExternalID   *string
	Source       string // 'hemis_employee' | 'hemis_student' | 'manual'
	PersonType   string // 'employee' | 'student'
	UserID       string
	FullName     string
	Gender       *string
	BirthDate    *time.Time
	DepartmentID *int64
	Status       *string
	AccessStatus string     // permanent | temporary
	PhotoURL     string     // manbadagi rasm manzili
	ValidTo      *time.Time // temporary uchun muddat; NULL = cheksiz

	StaffPosition *string

	Specialty     *string
	StudentGroup  *string
	LevelName     *string
	EducationForm *string
	EducationType *string
	PaymentForm   *string

	IsActive bool
}

// UpsertPerson — `user_id` bo'yicha qo'shadi yoki yangilaydi.
//
// ⚠️ Rasm maydonlariga TEGILMAYDI — ular alohida oqim orqali boshqariladi
// (yuklab olish + validatsiya). Sync ularni bekor qilib yubormasligi kerak.
//
// Qaytaradi: person id, yangi yaratilgan bo'lsa true.
func (s *Store) UpsertPerson(ctx context.Context, in PersonInput) (int64, bool, error) {
	var id int64
	var created bool

	err := s.pool.QueryRow(ctx, `
		INSERT INTO people (
			external_id, source, person_type, user_id, full_name,
			gender, birth_date, department_id, status,
			staff_position, specialty, student_group, level_name,
			education_form, education_type, payment_form,
			is_active, access_status, valid_to, photo_source_url,
			photo_status, synced_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
		        'missing', now(), now(), now())
		ON CONFLICT (user_id) DO UPDATE SET
			external_id = EXCLUDED.external_id,
			source = EXCLUDED.source,
			person_type = EXCLUDED.person_type,
			full_name = EXCLUDED.full_name,
			gender = EXCLUDED.gender,
			birth_date = EXCLUDED.birth_date,
			department_id = EXCLUDED.department_id,
			status = EXCLUDED.status,
			staff_position = EXCLUDED.staff_position,
			specialty = EXCLUDED.specialty,
			student_group = EXCLUDED.student_group,
			level_name = EXCLUDED.level_name,
			education_form = EXCLUDED.education_form,
			education_type = EXCLUDED.education_type,
			payment_form = EXCLUDED.payment_form,
			is_active = EXCLUDED.is_active,
			access_status = EXCLUDED.access_status,
			valid_to = EXCLUDED.valid_to,
			photo_source_url = EXCLUDED.photo_source_url,
			synced_at = now(),
			updated_at = now()
		RETURNING id, (xmax = 0) AS created`,
		in.ExternalID, in.Source, in.PersonType, in.UserID, in.FullName,
		in.Gender, in.BirthDate, in.DepartmentID, in.Status,
		in.StaffPosition, in.Specialty, in.StudentGroup, in.LevelName,
		in.EducationForm, in.EducationType, in.PaymentForm, in.IsActive,
		defaultAccess(in.AccessStatus), in.ValidTo, nullIfEmpty(in.PhotoURL)).
		Scan(&id, &created)

	return id, created, err
}

// PersonPhotoURL — sync paytida rasm yuklab olinishi kerakmi, shuni bilish
// uchun saqlangan manba URL'ini qaytaradi.
func (s *Store) PersonPhotoSourceURL(ctx context.Context, id int64) (string, error) {
	var url *string
	err := s.pool.QueryRow(ctx,
		`SELECT photo_metrics->>'source_url' FROM people WHERE id = $1`, id).Scan(&url)
	if err != nil || url == nil {
		return "", err
	}
	return *url, nil
}

// SetPersonStatus — bir yoki bir nechta odamning holatini o'zgartiradi.
//
// ⚠️ Faolsizlantirilgan odam terminaldan O'CHIRILMAYDI — bu alohida amal.
// Bu yerda faqat bazadagi holat o'zgaradi va u keyingi sync'ga ta'sir qiladi.
func (s *Store) SetPeopleActive(ctx context.Context, ids []int64, active bool) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE people SET is_active = $2, updated_at = now() WHERE id = ANY($1)`,
		ids, active)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// SetPeopleActiveByFilter — filtrga tushgan HAMMASIGA qo'llaydi.
// UI'da "hammasini tanlash" shu orqali ishlaydi (10 000 ta id yubormaslik uchun).
func (s *Store) SetPeopleActiveByFilter(ctx context.Context, f PeopleFilter, active bool) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE people p SET is_active = $4, updated_at = now()
		WHERE ($1 = '' OR p.full_name ILIKE '%'||$1||'%' OR p.user_id ILIKE '%'||$1||'%')
		  AND ($2 = '' OR p.source = $2)
		  AND ($3 = '' OR p.photo_status = $3)
		  AND ($5 = 0 OR p.department_id = $5)
		  AND ($6 = '' OR p.person_type = $6)
		  AND ($7 = '' OR p.gender = $7)`,
		f.Query, f.Source, f.PhotoStatus, active, f.DepartmentID, f.PersonType, f.Gender)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ------------------------------------------------------------- sync loglari

type SyncRun struct {
	ID         int64      `json:"id"`
	DeviceID   *int64     `json:"device_id"`
	DeviceName *string    `json:"device_name"`
	Kind       string     `json:"kind"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Total      int        `json:"total"`
	OK         int        `json:"ok_count"`
	Failed     int        `json:"fail_count"`
	Created    int        `json:"created"`
	Updated    int        `json:"updated"`
	Skipped    int        `json:"skipped"`
	Status     string     `json:"status"`
	Error      *string    `json:"error"`
}

func (s *Store) SyncRuns(ctx context.Context, kindPrefix string, limit int) ([]SyncRun, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.device_id, d.name, r.kind, r.started_at, r.finished_at,
		       r.total, r.ok_count, r.fail_count, r.created, r.updated, r.skipped,
		       r.status, r.error
		FROM sync_runs r
		LEFT JOIN devices d ON d.id = r.device_id
		WHERE ($1 = '' OR r.kind LIKE $1 || '%')
		ORDER BY r.id DESC
		LIMIT $2`, kindPrefix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SyncRun
	for rows.Next() {
		var r SyncRun
		if err := rows.Scan(&r.ID, &r.DeviceID, &r.DeviceName, &r.Kind,
			&r.StartedAt, &r.FinishedAt, &r.Total, &r.OK, &r.Failed,
			&r.Created, &r.Updated, &r.Skipped, &r.Status, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) StartRun(ctx context.Context, kind string, deviceID *int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sync_runs (device_id, kind, started_at, status, created_at, updated_at)
		VALUES ($1, $2, now(), 'running', now(), now())
		RETURNING id`, deviceID, kind).Scan(&id)
	return id, err
}

func (s *Store) FinishRun(ctx context.Context, id int64, status string,
	total, created, updated, skipped, failed int, errMsg *string) error {

	if id == 0 {
		return fmt.Errorf("sync_run id berilmagan")
	}

	// ⚠️ `ok_count = $4 + $5` YOZILMAYDI: PostgreSQL ikkita parametrning
	// turini aniqlay olmaydi ("operator is not unique: unknown + unknown")
	// va butun UPDATE yiqiladi. Bu xato jimgina yutilib, barcha loglar
	// `running` holatida qotib qolgan edi. Yig'indi Go tomonida hisoblanadi.
	okCount := created + updated

	tag, err := s.pool.Exec(ctx, `
		UPDATE sync_runs SET finished_at = now(), status = $2, total = $3,
			created = $4, updated = $5, skipped = $6, fail_count = $7,
			ok_count = $8, error = $9, updated_at = now()
		WHERE id = $1`,
		id, status, total, created, updated, skipped, failed, okCount, errMsg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("sync_run %d topilmadi", id)
	}
	return nil
}

func defaultAccess(s string) string {
	if s == "temporary" {
		return "temporary"
	}
	return "permanent"
}

// ExpirePeople — muddati o'tganlarni faolsizlantiradi.
//
// Vaqtincha kirish berilgan odam `valid_to` dan keyin avtomatik o'chadi.
// Muddat kiritilmagan bo'lsa (`valid_to IS NULL`) cheksiz turadi.
//
// ⚠️ Bu faqat BAZADAGI holatni o'zgartiradi. Terminaldan o'chirish alohida
// amal — keyingi sync bosqichida hal qilinadi.
func (s *Store) ExpirePeople(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE people SET is_active = false, updated_at = now()
		WHERE is_active
		  AND access_status = 'temporary'
		  AND valid_to IS NOT NULL
		  AND valid_to < CURRENT_DATE`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// PhotoCandidate — rasmi yuklanishi kerak bo'lgan odam.
type PhotoCandidate struct {
	ID       int64
	UserID   string
	PhotoURL string
}

// PeopleNeedingPhoto — manbada rasmi bor, lekin bizda hali yo'q yoki
// manbadagi URL o'zgargan odamlar.
func (s *Store) PeopleNeedingPhoto(ctx context.Context, limit int) ([]PhotoCandidate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, photo_source_url
		FROM people
		WHERE photo_source_url IS NOT NULL AND photo_source_url <> ''
		  AND (photo_path IS NULL
		       OR photo_status IN ('missing', 'pending')
		       OR photo_metrics->>'source_url' IS DISTINCT FROM photo_source_url)
		ORDER BY id
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PhotoCandidate
	for rows.Next() {
		var c PhotoCandidate
		if err := rows.Scan(&c.ID, &c.UserID, &c.PhotoURL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdatePersonPhotoFromSource — yuklab olingan rasm natijasini saqlaydi.
//
// ⚠️ Rasm o'zgargani uchun qurilmalarga qayta yuborilishi kerak — sync
// holati `pending` ga qaytariladi, lekin `device_recno` SAQLANADI
// (odam qurilmada bor, faqat yuzi yangilanadi).
func (s *Store) UpdatePersonPhotoFromSource(ctx context.Context, personID int64,
	path, status, reason string, metrics []byte, sourceURL string) error {

	var reasonArg *string
	if reason != "" {
		reasonArg = &reason
	}

	// metrics ichiga manba URL'ini ham yozamiz — keyingi sync o'zgarganini
	// shundan biladi.
	merged := map[string]any{}
	if len(metrics) > 0 {
		_ = json.Unmarshal(metrics, &merged)
	}

	// ⚠️ `Unmarshal` JSON `null` ni ko'rsa map'ni NIL qilib qo'yadi — yuqorida
	// bo'sh map bergan bo'lsak ham. `null` esa oson tug'iladi: face-api javob
	// bermasa chaqiruvchi `json.Marshal(nil)` yuboradi va u aynan "null" ()
	// uzunligi 4, ya'ni `len(metrics) > 0` tekshiruvidan o'tib ketadi).
	// Bunsiz keyingi qator "assignment to entry in nil map" bilan PANIC bo'ladi
	// va butun rasm yuklash jarayoni qulaydi (bir marta shunday bo'lgan).
	if merged == nil {
		merged = map[string]any{}
	}

	merged["source_url"] = sourceURL
	blob, _ := json.Marshal(merged)

	if _, err := s.pool.Exec(ctx, `
		UPDATE people SET photo_path = $2, photo_status = $3,
			photo_reject_reason = $4, photo_metrics = $5, updated_at = now()
		WHERE id = $1`, personID, path, status, reasonArg, blob); err != nil {
		return err
	}

	if status != "valid" {
		return nil
	}

	_, err := s.pool.Exec(ctx, `
		UPDATE device_person_sync
		SET state = 'pending', face_synced = false, updated_at = now()
		WHERE person_id = $1 AND state <> 'removed'`, personID)
	return err
}

// DeactivateByUserID — odam manbada ishdan bo'shatilgan bo'lsa chaqiriladi.
//
// ⚠️ Terminaldan O'CHIRMAYDI — bu alohida amal. Bu yerda faqat bazadagi
// holat o'zgaradi va odam keyingi sync'larda yuborilmaydi.
func (s *Store) DeactivateByUserID(ctx context.Context, userID string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE people SET is_active = false, updated_at = now()
		WHERE user_id = $1 AND is_active`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
