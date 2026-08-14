package httpapi

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/xuri/excelize/v2"

	"minidss/api/internal/store"
)

// exportReport — hisobotni XLSX qilib beradi.
//
// Ekrandagi ko'rinishni takrorlaydi: bir kunlik oraliqda oddiy jadval,
// uzunroq oraliqda odam × kun to'ri (dam olish kunlari ajratilgan).
func (a *API) exportReport(w http.ResponseWriter, r *http.Request) {
	f, err := a.filterFrom(r)
	if err != nil {
		fail(w, http.StatusBadRequest, "sana YYYY-MM-DD bo'lsin")
		return
	}
	f.Limit = 20000 // eksportda sahifalash yo'q

	rows, err := a.store.AttendanceDays(r.Context(), a.cfg.Timezone.String(), f)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	file := excelize.NewFile()
	defer file.Close()

	const sheet = "Hisobot"
	file.SetSheetName(file.GetSheetName(0), sheet)

	if f.From.Equal(f.To) {
		err = a.writeDaySheet(file, sheet, rows)
	} else {
		err = a.writeGridSheet(file, sheet, f, rows)
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}

	name := fmt.Sprintf("hisobot-%s_%s.xlsx",
		f.From.Format("2006-01-02"), f.To.Format("2006-01-02"))

	w.Header().Set("Content-Type",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)

	if _, err := file.WriteTo(w); err != nil {
		// Sarlavhalar allaqachon yuborilgan — faqat log qoladi.
		return
	}
}

// styles — jadval uchun tayyor uslublar.
type styles struct {
	head    int
	weekend int
	warn    int
}

func newStyles(file *excelize.File) (styles, error) {
	head, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EEF2F7"}},
	})
	if err != nil {
		return styles{}, err
	}

	// Shanba va yakshanba — qizil.
	weekend, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "C00000"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FDECEC"}},
	})
	if err != nil {
		return styles{}, err
	}

	// Chiqishi yozilmagan kun.
	warn, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "9C6500"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFF4CE"}},
	})
	if err != nil {
		return styles{}, err
	}

	return styles{head: head, weekend: weekend, warn: warn}, nil
}

func (a *API) localTime(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.In(a.cfg.Timezone).Format("15:04")
}

func fmtHours(h *float64) string {
	if h == nil {
		return "—"
	}
	total := int(*h*60 + 0.5)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// writeDaySheet — bir kunlik oddiy jadval.
func (a *API) writeDaySheet(file *excelize.File, sheet string, rows []store.DayRow) error {
	st, err := newStyles(file)
	if err != nil {
		return err
	}

	headers := []string{"Ism", "UserID", "Bo'lim", "Kirdi", "Chiqdi", "Ish vaqti", "Holat"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		file.SetCellValue(sheet, cell, h)
		file.SetCellStyle(sheet, cell, cell, st.head)
	}

	for i, r := range rows {
		row := i + 2

		holat := "To'liq"
		switch {
		case r.NoExit:
			holat = "Chiqishi yo'q"
		case r.FirstIn == nil:
			holat = "Faqat chiqish"
		}

		values := []any{
			r.FullName, r.UserID, deref(r.DepartmentName),
			a.localTime(r.FirstIn), a.localTime(r.LastOut), fmtHours(r.Hours), holat,
		}
		for c, v := range values {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			file.SetCellValue(sheet, cell, v)
		}

		if r.NoExit {
			from, _ := excelize.CoordinatesToCellName(7, row)
			file.SetCellStyle(sheet, from, from, st.warn)
		}
	}

	file.SetColWidth(sheet, "A", "A", 34)
	file.SetColWidth(sheet, "B", "B", 15)
	file.SetColWidth(sheet, "C", "C", 30)
	file.SetColWidth(sheet, "D", "G", 12)
	return nil
}

// writeGridSheet — odam × kun to'ri (haftalik/oylik).
func (a *API) writeGridSheet(
	file *excelize.File, sheet string, f store.AttendanceFilter, rows []store.DayRow,
) error {
	st, err := newStyles(file)
	if err != nil {
		return err
	}

	// Oraliqdagi hamma kun.
	var days []time.Time
	for d := f.From; !d.After(f.To); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}

	type personRow struct {
		name, userID, dep string
		byDay             map[string]store.DayRow
		totalHours        float64
		present           int
	}

	people := map[int64]*personRow{}
	for _, r := range rows {
		p := people[r.PersonID]
		if p == nil {
			p = &personRow{
				name: r.FullName, userID: r.UserID, dep: deref(r.DepartmentName),
				byDay: map[string]store.DayRow{},
			}
			people[r.PersonID] = p
		}
		p.byDay[r.Day.Format("2006-01-02")] = r
		p.present++
		if r.Hours != nil {
			p.totalHours += *r.Hours
		}
	}

	ordered := make([]*personRow, 0, len(people))
	for _, p := range people {
		ordered = append(ordered, p)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].name < ordered[j].name })

	// Sarlavha.
	head := []string{"Ism", "UserID", "Bo'lim"}
	for i, h := range head {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		file.SetCellValue(sheet, cell, h)
		file.SetCellStyle(sheet, cell, cell, st.head)
	}

	uzDow := map[time.Weekday]string{
		time.Monday: "Du", time.Tuesday: "Se", time.Wednesday: "Cho",
		time.Thursday: "Pay", time.Friday: "Ju", time.Saturday: "Sha", time.Sunday: "Yak",
	}

	for i, d := range days {
		col := len(head) + 1 + i
		cell, _ := excelize.CoordinatesToCellName(col, 1)
		file.SetCellValue(sheet, cell, fmt.Sprintf("%d %s", d.Day(), uzDow[d.Weekday()]))

		style := st.head
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			style = st.weekend
		}
		file.SetCellStyle(sheet, cell, cell, style)
	}

	totalCol := len(head) + len(days) + 1
	for i, h := range []string{"Jami soat", "Kelgan kun"} {
		cell, _ := excelize.CoordinatesToCellName(totalCol+i, 1)
		file.SetCellValue(sheet, cell, h)
		file.SetCellStyle(sheet, cell, cell, st.head)
	}

	// Qatorlar.
	for i, p := range ordered {
		row := i + 2

		for c, v := range []any{p.name, p.userID, p.dep} {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			file.SetCellValue(sheet, cell, v)
		}

		for j, d := range days {
			col := len(head) + 1 + j
			cell, _ := excelize.CoordinatesToCellName(col, row)

			key := d.Format("2006-01-02")
			day, ok := p.byDay[key]
			switch {
			case !ok:
				file.SetCellValue(sheet, cell, "")
			case day.Hours != nil:
				file.SetCellValue(sheet, cell, fmtHours(day.Hours))
			default:
				file.SetCellValue(sheet, cell, "•")
			}

			switch {
			case ok && day.NoExit:
				file.SetCellStyle(sheet, cell, cell, st.warn)
			case d.Weekday() == time.Saturday || d.Weekday() == time.Sunday:
				file.SetCellStyle(sheet, cell, cell, st.weekend)
			}
		}

		hours := p.totalHours
		cell, _ := excelize.CoordinatesToCellName(totalCol, row)
		file.SetCellValue(sheet, cell, fmtHours(&hours))
		cell, _ = excelize.CoordinatesToCellName(totalCol+1, row)
		file.SetCellValue(sheet, cell, p.present)
	}

	file.SetColWidth(sheet, "A", "A", 34)
	file.SetColWidth(sheet, "B", "B", 15)
	file.SetColWidth(sheet, "C", "C", 28)

	// Sarlavha qatori va ism ustuni doim ko'rinib tursin.
	return file.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      3,
		YSplit:      1,
		TopLeftCell: "D2",
		ActivePane:  "bottomRight",
	})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
