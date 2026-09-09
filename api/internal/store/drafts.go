package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Terminaldan olingan yuz rasmlari — QORALAMA.
//
// Eski DSS orqali kiritilgan odamlarning rasmi faqat terminalda qolgan:
// HEMIS ular uchun yuz bermaydi yoki yaroqsiz rasm qaytaradi. Qoralama —
// o'sha rasmning platformadagi nusxasi; u odamga BIRIKTIRILGUNCHA hech
// qayerga ta'sir qilmaydi.

// Draft — bitta qoralama.
type Draft struct {
	ID           int64      `json:"id"`
	UserID       string     `json:"user_id"`
	FullName     string     `json:"full_name"`
	DeviceID     *int64     `json:"device_id"`
	DeviceName   *string    `json:"device_name"`
	FileName     string     `json:"file_name"`
	Status       string     `json:"status"`
	RejectReason *string    `json:"reject_reason"`
	PersonID     *int64     `json:"person_id"`
	PersonName   *string    `json:"person_name"`
	AttachedAt   *time.Time `json:"attached_at"`
	CreatedAt    time.Time  `json:"created_at"`

	// Taklif: qoralama kimga tegishli bo'lishi mumkin.
	MatchID     *int64  `json:"match_id"`
	MatchName   *string `json:"match_name"`
	MatchUserID *string `json:"match_user_id"`
	MatchPhoto  *string `json:"match_photo_status"`
	// 'id' — UserID (yoki eski UserID) mos keldi; 'name' — faqat ism.
	MatchHow *string `json:"match_how"`
}

// DraftInput — terminaldan olingan yozuv.
type DraftInput struct {
	UserID       string
	FullName     string
	DeviceID     int64
	FileName     string
	Status       string
	RejectReason string
	Metrics      []byte
}

// UpsertDraft — qoralamani saqlaydi (`user_id` bo'yicha).
//
// ⚠️ Biriktirilgan qoralamaga TEGILMAYDI: operator uni allaqachon kimgadir
// bog'lagan, terminaldan qayta o'qish esa o'sha qarorni bekor qilib
// yubormasligi kerak.
func (s *Store) UpsertDraft(ctx context.Context, in DraftInput) (int64, bool, error) {
	var metrics any
	if len(in.Metrics) > 0 && string(in.Metrics) != "null" {
		metrics = json.RawMessage(in.Metrics)
	}

	var (
		id      int64
		created bool
	)
	err := s.pool.QueryRow(ctx, `
		INSERT INTO photo_drafts
			(user_id, full_name, device_id, file_name, status, reject_reason, metrics)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (user_id) DO UPDATE SET
			full_name = EXCLUDED.full_name,
			device_id = EXCLUDED.device_id,
			file_name = EXCLUDED.file_name,
			status = EXCLUDED.status,
			reject_reason = EXCLUDED.reject_reason,
			metrics = EXCLUDED.metrics,
			updated_at = now()
		WHERE photo_drafts.person_id IS NULL
		RETURNING id, (xmax = 0) AS created`,
		in.UserID, in.FullName, in.DeviceID, in.FileName,
		in.Status, nullIfEmpty(in.RejectReason), metrics).Scan(&id, &created)

	// `WHERE` sharti tufayli hech qanday qator qaytmasligi mumkin — bu xato
	// emas, "biriktirilgan qoralamaga tegmadik" degani.
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return id, created, err
}

// DraftFilter — ro'yxat filtri.
type DraftFilter struct {
	Query  string
	Status string
	// "" = hammasi, "yes" = biriktirilgan, "no" = biriktirilmagan
	Attached string
	Limit    int
	Offset   int
}

// ⚠️ `COALESCE(..., false)` SHART: `legacy_user_id` ko'pincha NULL va
// `false OR NULL` SQL'da NULL beradi, false emas. Bunsiz:
//   - `ORDER BY ... DESC` NULL'ni BIRINCHI qo'yadi, ya'ni ism bo'yicha
//     tasodifiy moslik aniq UserID moslikni siqib chiqaradi;
//   - `NOT by_id` NULL bo'lib, `AutoMatches` ism bo'yicha topilgan
//     mosliklarni JIMGINA tashlab yuboradi.
const matchedByID = `COALESCE(p.user_id = d.user_id OR p.legacy_user_id = d.user_id, false)`

// ⚠️ Taklif LATERAL bilan olinadi: har qoralamaga bitta eng yaxshi nomzod.
// UserID mos kelgani ismga qaraganda ustun — ism turli manbalarda turlicha
// yoziladi va bir xil ismli ikki odam bo'lishi mumkin.
var draftMatchJoin = `
	LEFT JOIN LATERAL (
		SELECT p.id, p.full_name, p.user_id,
		       COALESCE(op.status, p.photo_status) AS photo_status,
		       CASE WHEN ` + matchedByID + ` THEN 'id' ELSE 'name' END AS how
		FROM people p
		LEFT JOIN person_photos op ON op.id = p.photo_override_id
		WHERE NOT p.pending_delete
		  AND (p.user_id = d.user_id
		       OR p.legacy_user_id = d.user_id
		       OR (norm_name(d.full_name) <> ''
		           AND norm_name(p.full_name) = norm_name(d.full_name)))
		ORDER BY ` + matchedByID + ` DESC, p.id
		LIMIT 1
	) m ON true`

func (s *Store) Drafts(ctx context.Context, f DraftFilter) ([]Draft, int, error) {
	where := `WHERE ($1 = '' OR d.full_name ILIKE '%'||$1||'%' OR d.user_id ILIKE '%'||$1||'%')
		AND ($2 = '' OR d.status = $2)
		AND ($3 = '' OR ($3 = 'yes') = (d.person_id IS NOT NULL))`

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM photo_drafts d `+where,
		f.Query, f.Status, f.Attached).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.user_id, d.full_name, d.device_id, dev.name, d.file_name,
		       d.status, d.reject_reason, d.person_id, per.full_name,
		       d.attached_at, d.created_at,
		       m.id, m.full_name, m.user_id, m.photo_status, m.how
		FROM photo_drafts d
		LEFT JOIN devices dev ON dev.id = d.device_id
		LEFT JOIN people per ON per.id = d.person_id`+
		draftMatchJoin+` `+where+`
		ORDER BY (d.person_id IS NOT NULL), d.id DESC
		LIMIT $4 OFFSET $5`,
		f.Query, f.Status, f.Attached, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Draft
	for rows.Next() {
		var d Draft
		if err := rows.Scan(&d.ID, &d.UserID, &d.FullName, &d.DeviceID, &d.DeviceName,
			&d.FileName, &d.Status, &d.RejectReason, &d.PersonID, &d.PersonName,
			&d.AttachedAt, &d.CreatedAt,
			&d.MatchID, &d.MatchName, &d.MatchUserID, &d.MatchPhoto, &d.MatchHow); err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

func (s *Store) Draft(ctx context.Context, id int64) (Draft, error) {
	var d Draft
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, full_name, device_id, file_name, status,
		       reject_reason, person_id, attached_at, created_at
		FROM photo_drafts WHERE id = $1`, id).
		Scan(&d.ID, &d.UserID, &d.FullName, &d.DeviceID, &d.FileName, &d.Status,
			&d.RejectReason, &d.PersonID, &d.AttachedAt, &d.CreatedAt)
	return d, err
}

// AttachDraft — qoralamani odamga biriktiradi.
//
// `fileName` — qoralama faylining odam rasmlari orasidagi NUSXASI.
// Nusxa ataylab olinadi: qoralama keyin o'chirilsa ham odamning rasmi
// joyida qolishi kerak.
//
// ⚠️ Hammasi bitta tranzaksiyada: yarim bajarilgan holatda odam
// mavjud bo'lmagan rasmga ishora qilib qolardi.
func (s *Store) AttachDraft(ctx context.Context, draftID, personID int64, fileName string) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var (
		userID   string
		status   string
		reason   *string
		metrics  []byte
		deviceID *int64
	)
	if err := tx.QueryRow(ctx, `
		SELECT user_id, status, reject_reason, metrics, device_id
		FROM photo_drafts WHERE id = $1 FOR UPDATE`, draftID).
		Scan(&userID, &status, &reason, &metrics, &deviceID); err != nil {
		return 0, fmt.Errorf("qoralama topilmadi: %w", err)
	}

	var m any
	if len(metrics) > 0 && string(metrics) != "null" {
		m = json.RawMessage(metrics)
	}

	var photoID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO person_photos
			(person_id, source, file_name, source_url, status, reject_reason, metrics, device_id)
		VALUES ($1, 'terminal', $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		personID, fileName, deviceSourceURL(deviceID, userID), status, reason, m, deviceID).
		Scan(&photoID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE people SET photo_override_id = $2, updated_at = now()
		WHERE id = $1`, personID, photoID); err != nil {
		return 0, err
	}

	// ⚠️ Bunsiz odam `synced` bo'lib qolaveradi va yangi yuz terminalga
	// hech qachon bormaydi.
	if _, err := tx.Exec(ctx, `
		UPDATE device_person_sync
		SET state = 'pending', face_synced = false, updated_at = now()
		WHERE person_id = $1 AND state <> 'removed'`, personID); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE photo_drafts SET person_id = $2, attached_at = now(), updated_at = now()
		WHERE id = $1`, draftID, personID); err != nil {
		return 0, err
	}

	return photoID, tx.Commit(ctx)
}

func deviceSourceURL(deviceID *int64, userID string) *string {
	if deviceID == nil {
		return nil
	}
	url := fmt.Sprintf("device://%d/%s", *deviceID, userID)
	return &url
}

// DetachDraft — biriktirishni bekor qiladi (odam HEMIS rasmiga qaytadi).
func (s *Store) DetachDraft(ctx context.Context, draftID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE photo_drafts SET person_id = NULL, attached_at = NULL, updated_at = now()
		WHERE id = $1`, draftID)
	return err
}

// DeleteDrafts — qoralamalarni o'chiradi.
//
// ⚠️ Biriktirilganlari ham o'chishi mumkin — odamning rasmi buzilmaydi,
// chunki `person_photos` da NUSXA yotadi.
func (s *Store) DeleteDrafts(ctx context.Context, ids []int64) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM photo_drafts WHERE id = ANY($1)`, ids)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DraftMatch — avtomatik biriktirishga tayyor juftlik.
type DraftMatch struct {
	DraftID  int64
	PersonID int64
	UserID   string
	FileName string
	Person   string
}

// AutoMatches — rasmi yo'q yoki yaroqsiz odamlarga MOS qoralamalar.
//
// ⚠️ Faqat BIR MA'NOLI mosliklar qaytadi:
//   - UserID bo'yicha topilgan bo'lsa — o'sha bitta odam bo'lishi shart;
//   - faqat ism bo'yicha topilgan bo'lsa — butun bazada bitta nomzod
//     bo'lishi shart.
//
// Bunsiz bir xil ismli ikki odamdan biriga BEGONA yuz biriktirilib, u
// boshqa odamning ismi bilan eshikdan o'tib ketardi.
func (s *Store) AutoMatches(ctx context.Context, limit int) ([]DraftMatch, error) {
	rows, err := s.pool.Query(ctx, `
		WITH cand AS (
			SELECT d.id AS draft_id, d.user_id, d.file_name,
			       p.id AS person_id, p.full_name,
			       `+matchedByID+` AS by_id
			FROM photo_drafts d
			JOIN people p ON
				(p.user_id = d.user_id
				 OR p.legacy_user_id = d.user_id
				 OR (norm_name(d.full_name) <> ''
				     AND norm_name(p.full_name) = norm_name(d.full_name)))
			LEFT JOIN person_photos op ON op.id = p.photo_override_id
			WHERE d.person_id IS NULL
			  AND d.status = 'valid'
			  AND p.is_active AND NOT p.pending_delete
			  AND COALESCE(op.status, p.photo_status) <> 'valid'
		), ranked AS (
			SELECT *,
			       count(*) FILTER (WHERE by_id) OVER (PARTITION BY draft_id) AS id_hits,
			       count(*) OVER (PARTITION BY draft_id) AS all_hits
			FROM cand
		)
		SELECT draft_id, person_id, user_id, file_name, full_name
		FROM ranked
		WHERE (by_id AND id_hits = 1)
		   OR (NOT by_id AND id_hits = 0 AND all_hits = 1)
		ORDER BY draft_id
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DraftMatch
	for rows.Next() {
		var m DraftMatch
		if err := rows.Scan(&m.DraftID, &m.PersonID, &m.UserID, &m.FileName, &m.Person); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DraftUserIDs — allaqachon olingan qoralamalar (terminaldan qayta
// o'qimaslik uchun).
func (s *Store) DraftUserIDs(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.pool.Query(ctx, `SELECT user_id FROM photo_drafts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// DraftStats — qoralamalar bo'yicha qisqacha.
type DraftStats struct {
	Total      int `json:"total"`
	Valid      int `json:"valid"`
	Rejected   int `json:"rejected"`
	Attached   int `json:"attached"`
	Unattached int `json:"unattached"`
	// Avtomatik biriktirishga tayyor mosliklar soni.
	Matched int `json:"matched"`
}

func (s *Store) DraftStats(ctx context.Context) (DraftStats, error) {
	var st DraftStats
	err := s.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'valid'),
		       count(*) FILTER (WHERE status = 'rejected'),
		       count(*) FILTER (WHERE person_id IS NOT NULL),
		       count(*) FILTER (WHERE person_id IS NULL)
		FROM photo_drafts`).
		Scan(&st.Total, &st.Valid, &st.Rejected, &st.Attached, &st.Unattached)
	if err != nil {
		return st, err
	}

	matches, err := s.AutoMatches(ctx, 100000)
	if err != nil {
		return st, err
	}
	st.Matched = len(matches)
	return st, nil
}
