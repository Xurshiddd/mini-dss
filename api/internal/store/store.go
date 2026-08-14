// Package store — PostgreSQL bilan ishlash.
//
// ⚠️ Sxema Laravel davridan MEROS QOLGAN va bazada real ma'lumot bor
// (10 509 odam, 1140 rasm, sync holatlari). Jadval/ustun nomlari
// O'ZGARTIRILMAYDI — aks holda mavjud ma'lumot yo'qoladi.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ------------------------------------------------------------------ qurilmalar

type Device struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	IP         string     `json:"ip"`
	Port       int        `json:"port"`
	Username   string     `json:"username"`
	Password   string     `json:"-"` // shifrlangan holda saqlanadi
	Direction  string     `json:"direction"`
	Location   *string    `json:"location"`
	Model      *string    `json:"model"`
	Firmware   *string    `json:"firmware"`
	Serial     *string    `json:"serial"`
	IsActive   bool       `json:"is_active"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	LastError  *string    `json:"last_error"`
}

const deviceColumns = `id, name, ip, port, username, password, direction, location,
	model, firmware, serial, is_active, last_seen_at, last_error`

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.Name, &d.IP, &d.Port, &d.Username, &d.Password,
		&d.Direction, &d.Location, &d.Model, &d.Firmware, &d.Serial,
		&d.IsActive, &d.LastSeenAt, &d.LastError)
	return d, err
}

func (s *Store) Devices(ctx context.Context) ([]Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+deviceColumns+` FROM devices ORDER BY direction, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) Device(ctx context.Context, id int64) (Device, error) {
	return scanDevice(s.pool.QueryRow(ctx,
		`SELECT `+deviceColumns+` FROM devices WHERE id = $1`, id))
}

func (s *Store) ActiveDevices(ctx context.Context) ([]Device, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+deviceColumns+` FROM devices WHERE is_active ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) CreateDevice(ctx context.Context, d Device) (Device, error) {
	return scanDevice(s.pool.QueryRow(ctx, `
		INSERT INTO devices (name, ip, port, username, password, direction, location,
			is_active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8, now(), now())
		RETURNING `+deviceColumns,
		d.Name, d.IP, d.Port, d.Username, d.Password, d.Direction, d.Location, d.IsActive))
}

// UpdateDevice — parol bo'sh bo'lsa eskisi saqlanadi.
func (s *Store) UpdateDevice(ctx context.Context, id int64, d Device, newPassword string) (Device, error) {
	return scanDevice(s.pool.QueryRow(ctx, `
		UPDATE devices SET
			name = $2, ip = $3, port = $4, username = $5,
			password = COALESCE(NULLIF($6, ''), password),
			direction = $7, location = $8, is_active = $9, updated_at = now()
		WHERE id = $1
		RETURNING `+deviceColumns,
		id, d.Name, d.IP, d.Port, d.Username, newPassword, d.Direction, d.Location, d.IsActive))
}

func (s *Store) DeleteDevice(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	return err
}

func (s *Store) MarkDeviceSeen(ctx context.Context, id int64, ident map[string]string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE devices SET model = $2, firmware = $3, serial = $4,
			last_seen_at = now(), last_error = NULL, updated_at = now()
		WHERE id = $1`,
		id, ident["model"], ident["firmware"], ident["serial"])
	return err
}

func (s *Store) MarkDeviceError(ctx context.Context, id int64, msg string) {
	_, _ = s.pool.Exec(ctx,
		`UPDATE devices SET last_error = $2, updated_at = now() WHERE id = $1`, id, msg)
}

// --------------------------------------------------------------------- odamlar

type Person struct {
	ID           int64      `json:"id"`
	UserID       string     `json:"user_id"`
	LegacyUserID *string    `json:"legacy_user_id"`
	FullName     string     `json:"full_name"`
	Source       string     `json:"source"`
	PhotoPath    *string    `json:"photo_path"`
	PhotoStatus  string     `json:"photo_status"`
	PhotoReason  *string    `json:"photo_reject_reason"`
	ValidFrom    *time.Time `json:"valid_from"`
	ValidTo      *time.Time `json:"valid_to"`
	IsActive     bool       `json:"is_active"`
	// O'chirishga belgilangan, lekin hali hamma terminaldan tozalanmagan.
	PendingDelete bool `json:"pending_delete"`

	PersonType     string  `json:"person_type"`
	DepartmentID   *int64  `json:"department_id"`
	DepartmentName *string `json:"department_name"`
	Gender         *string `json:"gender"`
	Status         *string `json:"status"`        // HEMIS holati (Ishlamoqda / O'qimoqda)
	AccessStatus   string  `json:"access_status"` // permanent | temporary
	StaffPosition  *string `json:"staff_position"`
	Specialty      *string `json:"specialty"`
	StudentGroup   *string `json:"student_group"`
	LevelName      *string `json:"level_name"`

	SyncedOn int `json:"synced_on"` // nechta qurilmaga yozilgan
	// Shu qurilmadagi RecNo — faqat PeopleToSync to'ldiradi.
	// Bo'sh bo'lmasa odam qurilmada allaqachon bor: qayta qo'shilmaydi.
	DeviceRecNo *int `json:"-"`
}

type PeopleFilter struct {
	Query        string
	Source       string
	PhotoStatus  string
	PersonType   string
	DepartmentID int64  // 0 = filtrlanmaydi
	Gender       string // "Erkak" | "Ayol" | "" = hammasi
	Limit        int
	Offset       int
}

func (s *Store) People(ctx context.Context, f PeopleFilter) ([]Person, int, error) {
	where := `WHERE ($1 = '' OR p.full_name ILIKE '%'||$1||'%' OR p.user_id ILIKE '%'||$1||'%')
		AND ($2 = '' OR p.source = $2)
		AND ($3 = '' OR p.photo_status = $3)
		AND ($4 = '' OR p.person_type = $4)
		AND ($5 = 0 OR p.department_id = $5)
		AND ($6 = '' OR p.gender = $6)`

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM people p `+where,
		f.Query, f.Source, f.PhotoStatus, f.PersonType, f.DepartmentID, f.Gender).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.user_id, p.legacy_user_id, p.full_name, p.source,
		       p.photo_path, p.photo_status, p.photo_reject_reason,
		       p.valid_from, p.valid_to, p.is_active, p.pending_delete,
		       p.person_type, p.department_id, dep.name, p.gender, p.status, p.access_status,
		       p.staff_position, p.specialty, p.student_group, p.level_name,
		       (SELECT count(*) FROM device_person_sync d
		         WHERE d.person_id = p.id AND d.state = 'synced')
		FROM people p
		LEFT JOIN departments dep ON dep.id = p.department_id `+where+`
		ORDER BY p.full_name
		LIMIT $7 OFFSET $8`,
		f.Query, f.Source, f.PhotoStatus, f.PersonType, f.DepartmentID, f.Gender,
		f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.UserID, &p.LegacyUserID, &p.FullName, &p.Source,
			&p.PhotoPath, &p.PhotoStatus, &p.PhotoReason,
			&p.ValidFrom, &p.ValidTo, &p.IsActive, &p.PendingDelete,
			&p.PersonType, &p.DepartmentID, &p.DepartmentName, &p.Gender, &p.Status, &p.AccessStatus,
			&p.StaffPosition, &p.Specialty, &p.StudentGroup, &p.LevelName,
			&p.SyncedOn); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func (s *Store) Person(ctx context.Context, id int64) (Person, error) {
	var p Person
	err := s.pool.QueryRow(ctx, `
		SELECT p.id, p.user_id, p.legacy_user_id, p.full_name, p.source,
		       p.photo_path, p.photo_status, p.photo_reject_reason,
		       p.valid_from, p.valid_to, p.is_active, p.pending_delete,
		       p.person_type, p.department_id, dep.name, p.gender, p.status, p.access_status,
		       p.staff_position, p.specialty, p.student_group, p.level_name,
		       (SELECT count(*) FROM device_person_sync d
		         WHERE d.person_id = p.id AND d.state = 'synced')
		FROM people p
		LEFT JOIN departments dep ON dep.id = p.department_id
		WHERE p.id = $1`, id).
		Scan(&p.ID, &p.UserID, &p.LegacyUserID, &p.FullName, &p.Source,
			&p.PhotoPath, &p.PhotoStatus, &p.PhotoReason,
			&p.ValidFrom, &p.ValidTo, &p.IsActive, &p.PendingDelete,
			&p.PersonType, &p.DepartmentID, &p.DepartmentName, &p.Gender, &p.Status, &p.AccessStatus,
			&p.StaffPosition, &p.Specialty, &p.StudentGroup, &p.LevelName,
			&p.SyncedOn)
	return p, err
}

func (s *Store) UpdatePersonPhoto(ctx context.Context, id int64, path, status, reason string, metrics []byte) error {
	var reasonArg *string
	if reason != "" {
		reasonArg = &reason
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE people SET photo_path = $2, photo_status = $3, photo_reject_reason = $4,
			photo_metrics = $5, updated_at = now()
		WHERE id = $1`, id, path, status, reasonArg, metrics)
	return err
}

// MarkPhotoChanged — rasm o'zgargach, uni BARCHA qurilmaga qayta yuborish
// kerakligini belgilaydi.
//
// ⚠️ Bunsiz odam `synced` bo'lib qolaveradi va sync uni o'tkazib yuboradi —
// yangi rasm terminalga hech qachon bormaydi.
//
// `device_recno` SAQLANADI: odam qurilmada allaqachon bor, uni qaytadan
// qo'shish kerak emas — faqat yuzi yangilanadi.
func (s *Store) MarkPhotoChanged(ctx context.Context, personID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE device_person_sync
		SET state = 'pending', face_synced = false, updated_at = now()
		WHERE person_id = $1 AND state <> 'removed'`, personID)
	return err
}

// studentDuplicate — shu odam FAOL XODIM sifatida ham ro'yxatda bormi.
//
// HEMIS o'qiyotgan xodimni ikki marta qaytaradi: xodim modulida bitta ID
// bilan, talaba modulida BOSHQA ID bilan. Ikkalasi ham terminalga yozilsa,
// BITTA YUZ ikki foydalanuvchiga bog'lanadi — terminal tanishda chalkashadi.
//
// Bunday holatda xodim yozuvi ustun: odam shu yerda ishlaydi va uning
// eshik ruxsatlari xodim yozuviga bog'langan.
//
// ⚠️ Xodim yozuvi NOFAOL bo'lsa (ishdan bo'shagan, lekin o'qishda davom
// etayotgan) bu shart ISHLAMAYDI — aks holda odam ikkala yozuvdan ham
// mahrum bo'lib, kirish huquqini butunlay yo'qotardi.
const studentDuplicate = `(p.source = 'hemis_student' AND EXISTS (
		SELECT 1 FROM people e
		WHERE e.source = 'hemis_employee'
		  AND e.is_active AND NOT e.pending_delete AND e.photo_status = 'valid'
		  AND norm_name(e.full_name) = norm_name(p.full_name)))`

// PeopleToSync — qurilmaga yozilishi kerak bo'lganlar.
//
// Faqat rasmi tekshiruvdan o'tganlar: yuzsiz foydalanuvchi terminalda hech
// kimni kiritmaydi, ya'ni foydasiz yozuv bo'ladi.
func (s *Store) PeopleToSync(ctx context.Context, deviceID int64, retryFailed bool, limit int) ([]Person, error) {
	condition := `NOT EXISTS (SELECT 1 FROM device_person_sync d
		WHERE d.person_id = p.id AND d.device_id = $1 AND d.state = 'synced')`
	if retryFailed {
		condition = `EXISTS (SELECT 1 FROM device_person_sync d
			WHERE d.person_id = p.id AND d.device_id = $1 AND d.state = 'failed')`
	}

	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.user_id, p.legacy_user_id, p.full_name, p.source,
		       p.photo_path, p.photo_status, p.photo_reject_reason,
		       p.valid_from, p.valid_to, p.is_active,
		       p.person_type, p.department_id, NULL, p.gender, p.status, p.access_status,
		       p.staff_position, p.specialty, p.student_group, p.level_name, 0,
		       (SELECT d.device_recno FROM device_person_sync d
		         WHERE d.person_id = p.id AND d.device_id = $1)
		FROM people p
		WHERE p.is_active AND NOT p.pending_delete AND p.photo_status = 'valid'
		  AND (p.valid_to IS NULL OR p.valid_to >= CURRENT_DATE)
		  AND NOT `+studentDuplicate+`
		  AND `+condition+`
		ORDER BY p.id
		LIMIT $2`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Person
	for rows.Next() {
		var p Person
		if err := rows.Scan(&p.ID, &p.UserID, &p.LegacyUserID, &p.FullName, &p.Source,
			&p.PhotoPath, &p.PhotoStatus, &p.PhotoReason,
			&p.ValidFrom, &p.ValidTo, &p.IsActive,
			&p.PersonType, &p.DepartmentID, &p.DepartmentName, &p.Gender, &p.Status, &p.AccessStatus,
			&p.StaffPosition, &p.Specialty, &p.StudentGroup, &p.LevelName,
			&p.SyncedOn, &p.DeviceRecNo); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeviceRemoval — terminaldan olib tashlanishi kerak bo'lgan yozuv.
type DeviceRemoval struct {
	SyncID   int64
	PersonID int64
	FullName string
	// Qurilma odamni QAYSI ID bilan biladi — birlashtirilganlarda bu eski ID.
	UserID string
	RecNo  *int
}

// PeopleToRemove — qurilmada bor, lekin endi BO'LMASLIGI kerak bo'lganlar.
//
// Bunsiz status o'zgarishi terminalga yetib bormaydi: `PeopleToSync` bunday
// odamni shunchaki tanlamay qo'yadi, lekin u qurilmada qolib ketadi va
// eshikni ochaveradi.
//
// Shartlar:
//   - `NOT is_active` — qo'lda yoki HEMIS orqali faolsizlantirilgan;
//   - `pending_delete` — o'chirishga belgilangan;
//   - `valid_to` o'tib ketgan — vaqtinchalik ruxsat muddati tugagan;
//   - faol xodim yozuvi bor odamning talaba nusxasi (`studentDuplicate`) —
//     bitta yuz ikki foydalanuvchiga bog'lanib qolmasligi uchun.
//
// ⚠️ `photo_status` ATAYLAB yo'q. Rasm yaroqsiz bo'lib qolishi odamni
// terminaldan chiqarib yubormasligi kerak — manbadagi rasm buzuq kelsa,
// ishlab turgan xodim eshikdan qolib ketardi. Rasm o'zgarishi `MarkPhotoChanged`
// orqali QAYTA YOZISH bilan hal qilinadi, o'chirish bilan emas.
func (s *Store) PeopleToRemove(ctx context.Context, deviceID int64, limit int) ([]DeviceRemoval, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, p.id, p.full_name, COALESCE(p.legacy_user_id, p.user_id), d.device_recno
		FROM device_person_sync d
		JOIN people p ON p.id = d.person_id
		WHERE d.device_id = $1
		  AND d.state = 'synced'
		  AND (NOT p.is_active
		       OR p.pending_delete
		       OR (p.valid_to IS NOT NULL AND p.valid_to < CURRENT_DATE)
		       OR `+studentDuplicate+`)
		ORDER BY d.id
		LIMIT $2`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeviceRemoval
	for rows.Next() {
		var r DeviceRemoval
		if err := rows.Scan(&r.SyncID, &r.PersonID, &r.FullName, &r.UserID, &r.RecNo); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkPersonRemoved — odam qurilmadan olib tashlangani belgilanadi.
//
// `device_recno` NULL qilinadi: qurilmadagi yozuv endi yo'q, eski RecNo
// boshqa odamga berilishi mumkin va uni saqlab qolish xato o'chirishga
// olib keladi.
func (s *Store) MarkPersonRemoved(ctx context.Context, syncID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE device_person_sync
		SET state = 'removed', device_recno = NULL, face_synced = false,
		    last_error = NULL, updated_at = now()
		WHERE id = $1`, syncID)
	return err
}

// MarkRemovalFailed — o'chirish xatosi yoziladi, holat 'synced' bo'lib
// qoladi: odam hali ham qurilmada, keyingi sync qayta urinadi.
func (s *Store) MarkRemovalFailed(ctx context.Context, syncID int64, msg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE device_person_sync SET last_error = $2, updated_at = now()
		WHERE id = $1`, syncID, msg)
	return err
}

// ------------------------------------------------------------ odamni o'chirish

// SoftDeletePerson — o'chirishga belgilaydi (darhol o'chirmaydi).
//
// `is_active = false` ham qo'yiladi: shu daqiqadan boshlab odam yangi
// terminallarga yozilmaydi, hozirgilaridan esa sync olib tashlaydi.
func (s *Store) SoftDeletePerson(ctx context.Context, ids []int64) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE people SET pending_delete = true, is_active = false, updated_at = now()
		WHERE id = ANY($1)`, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// PurgeDeletedPeople — belgilangan va BARCHA terminaldan tozalanganlarni
// bazadan butunlay o'chiradi.
//
// Hech qaysi qurilmaga yozilmaganlar ham shu yerda o'chadi — ularda
// `device_person_sync` yozuvi umuman yo'q.
func (s *Store) PurgeDeletedPeople(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM people p
		WHERE p.pending_delete
		  AND NOT EXISTS (SELECT 1 FROM device_person_sync d
		                   WHERE d.person_id = p.id AND d.state <> 'removed')`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// PendingDeleteCount — hali terminallardan tozalanmaganlar soni.
func (s *Store) PendingDeleteCount(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM people WHERE pending_delete`).Scan(&n)
	return n, err
}

// ------------------------------------------------------------- sync holatlari

func (s *Store) SaveSyncState(ctx context.Context, deviceID, personID int64,
	state string, recNo *int, faceSynced bool, errMsg *string) error {

	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_person_sync
			(device_id, person_id, state, device_recno, face_synced, last_error,
			 attempts, synced_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6, 1, now(), now(), now())
		ON CONFLICT (device_id, person_id) DO UPDATE SET
			state = EXCLUDED.state,
			device_recno = COALESCE(EXCLUDED.device_recno, device_person_sync.device_recno),
			face_synced = EXCLUDED.face_synced,
			last_error = EXCLUDED.last_error,
			attempts = device_person_sync.attempts + 1,
			synced_at = now(),
			updated_at = now()`,
		deviceID, personID, state, recNo, faceSynced, errMsg)
	return err
}

// ResetSyncStates — qurilma tozalangandan keyin chaqiriladi.
//
// ⚠️ Bunsiz bazada eskirgan `state = synced` qolib ketadi va sync o'sha
// odamlarni "allaqachon yozilgan" deb O'TKAZIB YUBORADI. Bu ilgari 597
// odamning yozilmay qolishiga sabab bo'lgan.
func (s *Store) ResetSyncStates(ctx context.Context, deviceID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE device_person_sync
		SET state = 'pending', device_recno = NULL, face_synced = false,
		    synced_at = NULL, updated_at = now()
		WHERE device_id = $1`, deviceID)
	return err
}

// OrphanFace — qurilmada qolgan, egasiz yuz shabloni.
type OrphanFace struct {
	SyncID int64
	UserID string
}

// OrphanFaces — qurilmadagi yetim yuzlar.
//
// Foydalanuvchi yozuvlari `clear` bilan o'chirilganda yuz shablonlari
// O'CHMAYDI — ular alohida saqlanadi va `FaceInfoManager.cgi?action=clear`
// bu firmware'da yo'q (400). Natijada qurilmada egasiz yuzlar qoladi.
//
// ⚠️ `state = 'synced'` bo'lganlar CHIQARIB TASHLANADI — ular hozir
// qurilmada haqiqatan mavjud va ularning yuzi kerak.
//
// ⚠️ Qurilma odamni QAYSI ID bilan bilgan — shuni ishlatamiz.
// Birlashtirilgan odamlarda `user_id` yangi (manbadagi), qurilmadagi yuz esa
// eski ID ostida. `COALESCE` shuni hal qiladi.
func (s *Store) OrphanFaces(ctx context.Context, deviceID int64, limit int) ([]OrphanFace, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, COALESCE(p.legacy_user_id, p.user_id)
		FROM device_person_sync d
		JOIN people p ON p.id = d.person_id
		WHERE d.device_id = $1
		  AND d.state <> 'synced'
		  AND d.state <> 'removed'
		ORDER BY d.id
		LIMIT $2`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OrphanFace
	for rows.Next() {
		var f OrphanFace
		if err := rows.Scan(&f.SyncID, &f.UserID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// MarkFaceRemoved — darhol belgilanadi, uzilsa shu yerdan davom etiladi.
func (s *Store) MarkFaceRemoved(ctx context.Context, syncID int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE device_person_sync SET state = 'removed', face_synced = false,
		 updated_at = now() WHERE id = $1`, syncID)
	return err
}

type SyncStats struct {
	Synced  int `json:"synced"`
	Pending int `json:"pending"`
	Failed  int `json:"failed"`
	Faces   int `json:"faces"`
}

func (s *Store) SyncStats(ctx context.Context, deviceID int64) (SyncStats, error) {
	var st SyncStats
	err := s.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE state = 'synced'),
			count(*) FILTER (WHERE state = 'pending'),
			count(*) FILTER (WHERE state = 'failed'),
			count(*) FILTER (WHERE face_synced)
		FROM device_person_sync WHERE device_id = $1`, deviceID).
		Scan(&st.Synced, &st.Pending, &st.Failed, &st.Faces)
	return st, err
}

type Summary struct {
	People      int            `json:"people"`
	PhotoStatus map[string]int `json:"photo_status"`
	Devices     int            `json:"devices"`
	Sources     map[string]int `json:"sources"`
}

func (s *Store) Summary(ctx context.Context) (Summary, error) {
	sum := Summary{PhotoStatus: map[string]int{}, Sources: map[string]int{}}

	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM people`).Scan(&sum.People); err != nil {
		return sum, err
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM devices`).Scan(&sum.Devices); err != nil {
		return sum, err
	}

	for _, q := range []struct {
		sql string
		dst map[string]int
	}{
		{`SELECT photo_status, count(*) FROM people GROUP BY 1`, sum.PhotoStatus},
		{`SELECT source, count(*) FROM people GROUP BY 1`, sum.Sources},
	} {
		rows, err := s.pool.Query(ctx, q.sql)
		if err != nil {
			return sum, err
		}
		for rows.Next() {
			var k string
			var v int
			if err := rows.Scan(&k, &v); err != nil {
				rows.Close()
				return sum, err
			}
			q.dst[k] = v
		}
		rows.Close()
	}

	return sum, nil
}

var ErrNotFound = errors.New("topilmadi")
