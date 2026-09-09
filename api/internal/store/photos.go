package store

import (
	"context"
	"encoding/json"
	"time"
)

// Odamning rasmlari.
//
// ⚠️ Bitta odamda bir nechta rasm bo'lishi mumkin va ular BIR-BIRINI
// almashtirmaydi: HEMIS oqimi `people.photo_path` ni yangilab turadi,
// qo'lda yuklangan va terminaldan olingan rasmlar esa `person_photos` da
// alohida yotadi. Terminalga qaysi biri ketishini `people.photo_override_id`
// hal qiladi.
//
// Bunsiz qo'lda almashtirilgan rasm keyingi "Rasmlarni yuklash" bosqichida
// jimgina yo'qolardi — ham baza yozuvi, ham diskdagi fayl.

// Amaldagi rasm uchun SQL bo'laklari. `p` — `people`, `op` — override yozuvi.
const (
	overridePhotoJoin = `LEFT JOIN person_photos op ON op.id = p.photo_override_id`

	effectivePhotoPath   = `COALESCE(op.file_name, p.photo_path)`
	effectivePhotoStatus = `COALESCE(op.status, p.photo_status)`
	effectivePhotoReason = `CASE WHEN op.id IS NULL THEN p.photo_reject_reason ELSE op.reject_reason END`
	effectivePhotoSource = `COALESCE(op.source, 'hemis')`
)

// PersonPhoto — bitta rasm yozuvi.
type PersonPhoto struct {
	ID       int64  `json:"id"`
	PersonID int64  `json:"person_id"`
	Source   string `json:"source"` // manual | terminal | hemis
	// ⚠️ Faqat fayl nomi — yo'lsiz. Fayl `PHOTO_DIR` ichida yotadi.
	FileName     string    `json:"file_name"`
	SourceURL    *string   `json:"source_url"`
	Status       string    `json:"status"`
	RejectReason *string   `json:"reject_reason"`
	DeviceID     *int64    `json:"device_id"`
	DeviceName   *string   `json:"device_name"`
	CreatedAt    time.Time `json:"created_at"`
	// Shu rasm hozir terminalga ketadiganmi.
	Active bool `json:"active"`
}

// PersonPhotoInput — yangi rasm yozuvi.
type PersonPhotoInput struct {
	PersonID     int64
	Source       string
	FileName     string
	SourceURL    string
	Status       string
	RejectReason string
	Metrics      []byte
	DeviceID     *int64
}

// AddPersonPhoto — rasmni tarixga yozadi. Eskisiga TEGMAYDI.
func (s *Store) AddPersonPhoto(ctx context.Context, in PersonPhotoInput) (int64, error) {
	// ⚠️ `json.Marshal(nil)` "null" beradi va uzunligi 4 — ya'ni bo'sh emas
	// deb o'tib ketadi. Bunday qiymat bazada foydasiz, NULL qilib qo'yamiz.
	var metrics any
	if len(in.Metrics) > 0 && string(in.Metrics) != "null" {
		metrics = json.RawMessage(in.Metrics)
	}

	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO person_photos
			(person_id, source, file_name, source_url, status, reject_reason,
			 metrics, device_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		in.PersonID, in.Source, in.FileName, nullIfEmpty(in.SourceURL),
		in.Status, nullIfEmpty(in.RejectReason), metrics, in.DeviceID).Scan(&id)
	return id, err
}

// SetPhotoOverride — terminalga ketadigan rasmni belgilaydi.
//
// `photoID` nil bo'lsa odam HEMIS rasmiga qaytadi.
//
// ⚠️ `MarkPhotoChanged` shu yerda chaqiriladi: bunsiz odam `synced` bo'lib
// qolaveradi va yangi rasm terminalga HECH QACHON bormaydi.
func (s *Store) SetPhotoOverride(ctx context.Context, personID int64, photoID *int64) error {
	if _, err := s.pool.Exec(ctx, `
		UPDATE people SET photo_override_id = $2, updated_at = now()
		WHERE id = $1`, personID, photoID); err != nil {
		return err
	}
	return s.MarkPhotoChanged(ctx, personID)
}

// PersonPhotos — odamning rasm tarixi, yangisidan eskisiga.
//
// HEMIS rasmi bu ro'yxatda YO'Q — u `people.photo_path` da yotadi va
// almashtirilgan rasm o'chirilganda odam avtomatik unga qaytadi.
func (s *Store) PersonPhotos(ctx context.Context, personID int64) ([]PersonPhoto, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ph.id, ph.person_id, ph.source, ph.file_name, ph.source_url,
		       ph.status, ph.reject_reason, ph.device_id, d.name, ph.created_at,
		       (p.photo_override_id = ph.id) AS active
		FROM person_photos ph
		JOIN people p ON p.id = ph.person_id
		LEFT JOIN devices d ON d.id = ph.device_id
		WHERE ph.person_id = $1
		ORDER BY ph.id DESC`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PersonPhoto
	for rows.Next() {
		var p PersonPhoto
		var active *bool
		if err := rows.Scan(&p.ID, &p.PersonID, &p.Source, &p.FileName, &p.SourceURL,
			&p.Status, &p.RejectReason, &p.DeviceID, &p.DeviceName, &p.CreatedAt,
			&active); err != nil {
			return nil, err
		}
		p.Active = active != nil && *active
		out = append(out, p)
	}
	return out, rows.Err()
}

// PersonPhoto — bitta rasm yozuvi (fayl berishdan oldin egasini tekshirish uchun).
func (s *Store) PersonPhotoByID(ctx context.Context, id int64) (PersonPhoto, error) {
	var p PersonPhoto
	err := s.pool.QueryRow(ctx, `
		SELECT id, person_id, source, file_name, source_url, status,
		       reject_reason, device_id, created_at
		FROM person_photos WHERE id = $1`, id).
		Scan(&p.ID, &p.PersonID, &p.Source, &p.FileName, &p.SourceURL, &p.Status,
			&p.RejectReason, &p.DeviceID, &p.CreatedAt)
	return p, err
}

// DeletePersonPhoto — rasm yozuvini o'chiradi.
//
// ⚠️ Bu rasm hozir terminalga ketayotgan bo'lsa, `photo_override_id`
// `ON DELETE SET NULL` orqali bo'shaydi va odam HEMIS rasmiga qaytadi —
// shuning uchun qayta yozish ham belgilanadi.
//
// Diskdagi fayl bu yerda O'CHIRILMAYDI: xato biriktirishni qaytarish
// imkoniyati qolsin.
func (s *Store) DeletePersonPhoto(ctx context.Context, id int64) (int64, error) {
	var personID int64
	if err := s.pool.QueryRow(ctx,
		`DELETE FROM person_photos WHERE id = $1 RETURNING person_id`, id).
		Scan(&personID); err != nil {
		return 0, err
	}
	return personID, s.MarkPhotoChanged(ctx, personID)
}
