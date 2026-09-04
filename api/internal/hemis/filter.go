package hemis

import (
	"fmt"
	"strconv"
	"strings"
)

// StudentFilter — `student-list` uchun filtrlar (HEMIS query parametrlari).
//
// Bo'sh maydon so'rovga QO'SHILMAYDI. Barcha filtrlar VA (AND) bilan
// birlashadi.
//
// ⚠️ Bir maydonga faqat BITTA qiymat. Vergulli ro'yxat (`_education_form=11,13`)
// xato bermaydi — jimgina 0 ta natija qaytaradi. Ikki shakl kerak bo'lsa,
// sync ikki marta, har xil filtr bilan yuritiladi.
//
// ⚠️ `_payment_form` va `_accommodation` HEMIS tomonida ISHLAMAYDI: qaysi
// kod berilmasin to'liq ro'yxat qaytadi (o'lchangan: 7120 = jami). Ikkalasi
// ham javob ustida saralanadi (`Match`), so'rovga yuborilmaydi.
type StudentFilter struct {
	EducationForm string `json:"education_form"` // h_education_form kodi
	EducationType string `json:"education_type"` // h_education_type kodi
	PaymentForm   string `json:"payment_form"`   // h_payment_form kodi (mahalliy saralash)
	Accommodation string `json:"accommodation"`  // h_accommodation kodi (15 = yotoqxona, mahalliy saralash)
	Level         string `json:"level"`          // h_course kodi (11 = 1-kurs)
	Semester      string `json:"semester"`       // h_semester kodi
	Gender        string `json:"gender"`         // h_gender kodi
	Citizenship   string `json:"citizenship"`    // h_citizenship_type kodi
	StudentStatus string `json:"student_status"` // bo'sh = 11 (O'qimoqda), "-1" = hammasi
	Province      string `json:"province"`       // h_soato kodi (viloyat)
	District      string `json:"district"`       // h_soato kodi (tuman)
	Department    int64  `json:"department"`     // fakultet id
	Specialty     int64  `json:"specialty"`      // yo'nalish id
	Group         int64  `json:"group"`          // guruh id
	Curriculum    int64  `json:"curriculum"`     // o'quv reja id
	Search        string `json:"search"`         // ism yoki talaba ID raqami
	PassportPIN   string `json:"passport_pin"`   // ⚠️ passport_number bilan birga
	PassportNo    string `json:"passport_number"`
	TutorPIN      string `json:"tutor_pin"`
	UpdatedFrom   int64  `json:"updated_at_from"` // unix, >=
	UpdatedTo     int64  `json:"updated_at_to"`   // unix, <=
}

// Validate — filtr qiymatlarini tekshiradi.
//
// ⚠️ Raqam kutilgan joyga harf yuborilsa HEMIS 500 qaytaradi ("Ichki server
// xatosi"), 400 emas — shuning uchun xato bizda, so'rovdan OLDIN tutiladi.
func (f StudentFilter) Validate() error {
	codes := map[string]string{
		"ta'lim shakli": f.EducationForm,
		"ta'lim turi":   f.EducationType,
		"to'lov shakli": f.PaymentForm,
		"kurs":          f.Level,
		"semestr":       f.Semester,
		"jins":          f.Gender,
		"fuqarolik":     f.Citizenship,
		"viloyat":       f.Province,
		"tuman":         f.District,
		"talaba holati": f.StudentStatus,
	}
	for name, v := range codes {
		if v == "" {
			continue
		}
		// Talaba holatida "-1" ("hammasi") ruxsat etilgan yagona manfiy qiymat.
		if v == "-1" && name == "talaba holati" {
			continue
		}
		if !isDigits(v) {
			return fmt.Errorf("%s kodi faqat raqamdan iborat bo'lishi kerak (%q)", name, v)
		}
	}

	if (f.PassportPIN == "") != (f.PassportNo == "") {
		// ⚠️ HEMIS ikkalasini birga talab qiladi; bittasi bo'lsa filtr jimgina
		// e'tiborsiz qoladi.
		return fmt.Errorf("passport JSHSHIR va seriya-raqam BIRGA berilishi kerak")
	}
	if f.UpdatedFrom > 0 && f.UpdatedTo > 0 && f.UpdatedFrom > f.UpdatedTo {
		return fmt.Errorf("yangilanish oralig'i teskari: boshi oxiridan katta")
	}

	return nil
}

// values — bo'sh bo'lmagan filtrlarni query parametrlariga aylantiradi.
func (f StudentFilter) values() map[string]string {
	m := map[string]string{}

	str := func(key, v string) {
		if v = strings.TrimSpace(v); v != "" {
			m[key] = v
		}
	}
	num := func(key string, v int64) {
		if v > 0 {
			m[key] = strconv.FormatInt(v, 10)
		}
	}

	str("_education_form", f.EducationForm)
	str("_education_type", f.EducationType)
	str("_level", f.Level)
	str("_semester", f.Semester)
	str("_gender", f.Gender)
	str("_citizenship", f.Citizenship)
	str("_student_status", f.StudentStatus)
	str("_province", f.Province)
	str("_district", f.District)
	str("search", f.Search)
	str("passport_pin", f.PassportPIN)
	str("passport_number", f.PassportNo)
	str("tutor_pin", f.TutorPIN)

	num("_department", f.Department)
	num("_specialty", f.Specialty)
	num("_group", f.Group)
	num("_curriculum", f.Curriculum)
	num("updated_at_from", f.UpdatedFrom)
	num("updated_at_to", f.UpdatedTo)

	// ⚠️ `_payment_form` va `_accommodation` ataylab yuborilmaydi — HEMIS
	// ularni jimgina e'tiborsiz qoldiradi ("grant" so'ralganda shartnomachi
	// ham keladi). Saralash `Match` da.

	return m
}

// Match — so'rovda amalga oshmaydigan filtrlarni javob ustida tekshiradi.
// So'rov filtri yetarli bo'lsa `true`.
func (f StudentFilter) Match(s Student) bool {
	if f.PaymentForm != "" && s.PaymentForm.Code != f.PaymentForm {
		return false
	}
	if f.Accommodation != "" && s.Accommodation.Code != f.Accommodation {
		return false
	}
	return true
}

// Local — natija javob ustida saralanadimi (ya'ni HEMIS bergan son
// haqiqiydan katta bo'ladimi).
func (f StudentFilter) Local() bool {
	return f.PaymentForm != "" || f.Accommodation != ""
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
