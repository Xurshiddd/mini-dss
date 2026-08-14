package store

import (
	"context"
	"time"
)

// DayRow — bitta odamning bitta kundagi davomati.
type DayRow struct {
	PersonID       int64      `json:"person_id"`
	UserID         string     `json:"user_id"`
	FullName       string     `json:"full_name"`
	DepartmentName *string    `json:"department_name"`
	PersonType     string     `json:"person_type"`
	Day            time.Time  `json:"day"`
	FirstIn        *time.Time `json:"first_in"`
	LastOut        *time.Time `json:"last_out"`
	Hours          *float64   `json:"hours"`
	Events         int        `json:"events"`

	// Kirgan, lekin chiqishi yozilmagan.
	//
	// ⚠️ Bu "ishdan ketmagan" degani EMAS: odam boshqa chiqish terminalidan
	// o'tgan bo'lishi mumkin, o'sha terminal esa tizimga qo'shilmagan yoki
	// o'chiq bo'lishi mumkin. Shuning uchun soat hisoblanmaydi, faqat
	// belgilanadi.
	NoExit bool `json:"no_exit"`
}

// AttendanceFilter — hisobot filtri.
type AttendanceFilter struct {
	From     time.Time // kun (shu kun kiradi)
	To       time.Time // kun (shu kun kiradi)
	PersonID int64     // 0 = hammasi
	Query    string    // ism yoki UserID bo'yicha
	Limit    int
	Offset   int
}

// attendanceCTE — kunlik yig'ma. Ikkala so'rov ham shundan foydalanadi.
//
// ⚠️ Faqat `status = 1` — ya'ni terminal odamni HAQIQATAN tanigan hodisalar.
// Tanilmagan urinishlar (`status = 0`, `UserID` bo'sh) davomat emas.
//
// ⚠️ Kun chegarasi Toshkent zonasida hisoblanadi: `happened_at` UTC saqlanadi,
// aks holda kechqurungi kirish ertangi kunga tushib qolardi.
const attendanceCTE = `
	WITH ev AS (
		SELECT e.person_id,
		       (e.happened_at AT TIME ZONE $1)::date AS day,
		       e.direction, e.happened_at
		FROM access_events e
		WHERE e.status = 1
		  AND e.person_id IS NOT NULL
		  AND (e.happened_at AT TIME ZONE $1)::date BETWEEN $2 AND $3
	),
	agg AS (
		SELECT person_id, day,
		       min(happened_at) FILTER (WHERE direction = 'entry') AS first_in,
		       max(happened_at) FILTER (WHERE direction = 'exit')  AS last_out,
		       count(*) AS events
		FROM ev
		GROUP BY person_id, day
	)`

// AttendanceDays — kun bo'yicha davomat qatorlari.
func (s *Store) AttendanceDays(ctx context.Context, tz string, f AttendanceFilter) ([]DayRow, error) {
	rows, err := s.pool.Query(ctx, attendanceCTE+`
		SELECT a.person_id, p.user_id, p.full_name, dep.name, p.person_type,
		       a.day, a.first_in, a.last_out, a.events,
		       CASE
		         WHEN a.first_in IS NOT NULL AND a.last_out > a.first_in
		         THEN EXTRACT(EPOCH FROM (a.last_out - a.first_in)) / 3600.0
		       END AS hours
		FROM agg a
		JOIN people p ON p.id = a.person_id
		LEFT JOIN departments dep ON dep.id = p.department_id
		WHERE ($4 = 0 OR a.person_id = $4)
		  AND ($5 = '' OR p.full_name ILIKE '%'||$5||'%' OR p.user_id ILIKE '%'||$5||'%')
		ORDER BY a.day DESC, p.full_name
		LIMIT $6 OFFSET $7`,
		tz, f.From, f.To, f.PersonID, f.Query, f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DayRow
	for rows.Next() {
		var r DayRow
		if err := rows.Scan(&r.PersonID, &r.UserID, &r.FullName, &r.DepartmentName,
			&r.PersonType, &r.Day, &r.FirstIn, &r.LastOut, &r.Events, &r.Hours); err != nil {
			return nil, err
		}
		r.NoExit = r.FirstIn != nil && r.LastOut == nil
		out = append(out, r)
	}
	return out, rows.Err()
}

// PersonTotals — bitta odamning oraliq bo'yicha yig'masi.
type PersonTotals struct {
	PersonID       int64   `json:"person_id"`
	UserID         string  `json:"user_id"`
	FullName       string  `json:"full_name"`
	DepartmentName *string `json:"department_name"`
	PersonType     string  `json:"person_type"`
	Days           int     `json:"days"`        // nechta kun kelgan
	NoExitDays     int     `json:"no_exit_days"`
	TotalHours     float64 `json:"total_hours"`
	AvgHours       float64 `json:"avg_hours"`
}

// AttendanceTotals — haftalik/oylik hisobot uchun odam bo'yicha yig'ma.
func (s *Store) AttendanceTotals(ctx context.Context, tz string, f AttendanceFilter) ([]PersonTotals, error) {
	rows, err := s.pool.Query(ctx, attendanceCTE+`,
		calc AS (
			SELECT person_id, day,
			       (first_in IS NOT NULL AND last_out IS NULL) AS no_exit,
			       CASE
			         WHEN first_in IS NOT NULL AND last_out > first_in
			         THEN EXTRACT(EPOCH FROM (last_out - first_in)) / 3600.0
			         ELSE 0
			       END AS hours
			FROM agg
		)
		SELECT c.person_id, p.user_id, p.full_name, dep.name, p.person_type,
		       count(*) AS days,
		       count(*) FILTER (WHERE c.no_exit) AS no_exit_days,
		       COALESCE(sum(c.hours), 0) AS total_hours,
		       COALESCE(avg(c.hours) FILTER (WHERE c.hours > 0), 0) AS avg_hours
		FROM calc c
		JOIN people p ON p.id = c.person_id
		LEFT JOIN departments dep ON dep.id = p.department_id
		WHERE ($4 = 0 OR c.person_id = $4)
		  AND ($5 = '' OR p.full_name ILIKE '%'||$5||'%' OR p.user_id ILIKE '%'||$5||'%')
		GROUP BY c.person_id, p.user_id, p.full_name, dep.name, p.person_type
		ORDER BY p.full_name
		LIMIT $6 OFFSET $7`,
		tz, f.From, f.To, f.PersonID, f.Query, f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PersonTotals
	for rows.Next() {
		var t PersonTotals
		if err := rows.Scan(&t.PersonID, &t.UserID, &t.FullName, &t.DepartmentName,
			&t.PersonType, &t.Days, &t.NoExitDays, &t.TotalHours, &t.AvgHours); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// PersonEvent — profil oynasidagi xom hodisa qatori.
type PersonEvent struct {
	HappenedAt time.Time `json:"happened_at"`
	Direction  string    `json:"direction"`
	DeviceName string    `json:"device_name"`
	Location   *string   `json:"location"`
}

// PersonEvents — bitta odamning oraliqdagi hamma kirish/chiqishlari.
func (s *Store) PersonEvents(ctx context.Context, tz string, personID int64, from, to time.Time) ([]PersonEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.happened_at, e.direction, d.name, d.location
		FROM access_events e
		JOIN devices d ON d.id = e.device_id
		WHERE e.person_id = $4 AND e.status = 1
		  AND (e.happened_at AT TIME ZONE $1)::date BETWEEN $2 AND $3
		ORDER BY e.happened_at`,
		tz, from, to, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PersonEvent
	for rows.Next() {
		var e PersonEvent
		if err := rows.Scan(&e.HappenedAt, &e.Direction, &e.DeviceName, &e.Location); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
