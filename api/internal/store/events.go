package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// AccessEvent — bazaga yoziladigan kirish/chiqish yozuvi.
type AccessEvent struct {
	RecNo       int64
	UserID      string
	HappenedAt  time.Time
	Status      int
	ErrorCode   int
	Method      int
	SnapshotURL string
}

// LastEventRecNo — shu qurilma uchun bazadagi eng katta RecNo.
//
// Yig'ishni qayerdan davom ettirishni shu belgilaydi: qurilma logi halqa
// bufer bo'lgani uchun offset emas, aynan RecNo kursor bo'lib ishlaydi.
func (s *Store) LastEventRecNo(ctx context.Context, deviceID int64) (int64, error) {
	var recNo *int64
	err := s.pool.QueryRow(ctx,
		`SELECT max(rec_no) FROM access_events WHERE device_id = $1`, deviceID).Scan(&recNo)
	if err != nil {
		return 0, err
	}
	if recNo == nil {
		return 0, nil
	}
	return *recNo, nil
}

// SaveEvents — hodisalarni yozadi va nechtasi YANGI ekanini qaytaradi.
//
// Takrorlanganlari `ON CONFLICT DO NOTHING` bilan jimgina o'tkazib
// yuboriladi — qayta yig'ish xavfsiz.
//
// `direction` QURILMADAN keladi (`devices.direction`), hodisadan emas.
func (s *Store) SaveEvents(
	ctx context.Context, deviceID int64, direction string, events []AccessEvent,
) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, e := range events {
		var snapshot *string
		if e.SnapshotURL != "" {
			snapshot = &e.SnapshotURL
		}

		// ⚠️ `person_id` yozish paytida bog'lanadi: odam keyin o'chirilsa
		// ham hodisa yo'qolmaydi (`ON DELETE SET NULL`). Qurilma odamni eski
		// ID bilan bilishi mumkin — shuning uchun `legacy_user_id` ham
		// tekshiriladi.
		batch.Queue(`
			INSERT INTO access_events
				(device_id, rec_no, user_id, person_id, happened_at, direction,
				 status, error_code, method, snapshot_url)
			-- ⚠️ ::varchar cast SHART: bitta parametr ham qiymat, ham
			-- solishtirish uchun ishlatilganda Postgres uning turini
			-- aniqlay olmaydi (SQLSTATE 42P08).
			SELECT $1, $2, $3::varchar,
				(SELECT id FROM people
				  WHERE user_id = $3::varchar OR legacy_user_id = $3::varchar
				  ORDER BY id LIMIT 1),
				$4, $5, $6, $7, $8, $9
			-- ⚠️ Jonli oqim bu hodisani allaqachon yozgan bo'lishi mumkin
			-- (o'shanda rec_no NULL). Tanilmaganlarga tegilmaydi: ularda
			-- user_id bo'sh va bir soniyada bir nechtasi bo'ladi.
			WHERE $3::varchar = '' OR NOT EXISTS (
				SELECT 1 FROM access_events e
				WHERE e.device_id = $1 AND e.user_id = $3::varchar
				  AND e.happened_at = $4
			)
			ON CONFLICT (device_id, rec_no) DO NOTHING`,
			deviceID, e.RecNo, e.UserID, e.HappenedAt, direction,
			e.Status, e.ErrorCode, e.Method, snapshot)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()

	inserted := 0
	for range events {
		tag, err := results.Exec()
		if err != nil {
			return inserted, err
		}
		inserted += int(tag.RowsAffected())
	}
	return inserted, nil
}

// SaveLiveEvent — jonli oqimdan kelgan bitta hodisani yozadi.
//
// `rec_no` NULL: u faqat qurilma jurnalida bo'ladi. Keyinroq jurnal
// yig'ilganda o'sha hodisa qayta kelsa, `(device_id, user_id, happened_at)`
// kaliti uni to'xtatadi.
//
// Qaytaradi: yozuv YANGI edimi.
func (s *Store) SaveLiveEvent(
	ctx context.Context, deviceID int64, direction string, e AccessEvent,
) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO access_events
			(device_id, rec_no, user_id, person_id, happened_at, direction,
			 status, error_code, method)
		SELECT $1, NULL, $2::varchar,
			(SELECT id FROM people
			  WHERE user_id = $2::varchar OR legacy_user_id = $2::varchar
			  ORDER BY id LIMIT 1),
			$3, $4, $5, $6, $7
		-- ⚠️ Bu hodisa keyinroq jurnal yig'ishda RecNo bilan qayta keladi.
		-- Unikal indeks qo'yib bo'lmaydi (qurilma bir soniyada takror
		-- yozishi mumkin), shuning uchun tekshiruv shu yerda.
		WHERE NOT EXISTS (
			SELECT 1 FROM access_events e
			WHERE e.device_id = $1 AND e.user_id = $2::varchar
			  AND e.happened_at = $3
		)`,
		deviceID, e.UserID, e.HappenedAt, direction, e.Status, e.ErrorCode, e.Method)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RelinkEvents — `person_id` si bo'sh hodisalarni odamlarga qayta bog'laydi.
//
// Kerak bo'ladi: hodisa odam bazaga qo'shilishidan OLDIN yozilgan bo'lsa
// (masalan qurilmadan eski loglar yig'ilganda), bog'lanish bo'sh qoladi.
func (s *Store) RelinkEvents(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE access_events e
		SET person_id = p.id
		FROM people p
		WHERE e.person_id IS NULL
		  AND (p.user_id = e.user_id OR p.legacy_user_id = e.user_id)`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
