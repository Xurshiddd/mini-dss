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

// EmployeeFilter — `employee-list` uchun filtrlar (HEMIS query parametrlari).
//
// Talabalarnikidan farqli o'laroq bu yerdagi HAMMA filtr HEMIS tomonida
// haqiqatan ishlaydi — jonli o'lchangan (2026-09-16): jami 1317,
// `_gender=12` → 644, `_employee_status=14` → 574, `_department=8` → 43.
//
// ⚠️ `type` MAJBURIY parametr. Bo'sh qoldirilsa biz "all" yuboramiz —
// usiz HEMIS ro'yxatning faqat bir qismini beradi.
type EmployeeFilter struct {
	Type            string `json:"type"`             // teacher | employee | all
	Department      int64  `json:"department"`       // bo'lim id
	Gender          string `json:"gender"`           // h_gender kodi
	StaffPosition   string `json:"staff_position"`   // h_teacher_position_type kodi
	EmployeeStatus  string `json:"employee_status"`  // h_teacher_status kodi
	EmploymentForm  string `json:"employment_form"`  // h_employment_form kodi
	EmploymentStaff string `json:"employment_staff"` // h_employment_staff kodi (stavka)
	EmployeeType    string `json:"employee_type"`    // h_employee_type kodi
	AcademicRank    string `json:"academic_rank"`    // h_academic_rank kodi
	AcademicDegree  string `json:"academic_degree"`  // h_academic_degree kodi
	Search          string `json:"search"`           // ism yoki xodim ID raqami
	PassportPIN     string `json:"passport_pin"`     // ⚠️ passport_number bilan birga
	PassportNo      string `json:"passport_number"`
}

// Xodim ro'yxati turlari (`type` parametri).
const (
	EmployeeTypeAll      = "all"
	EmployeeTypeTeacher  = "teacher"
	EmployeeTypeEmployee = "employee"
)

// Validate — filtr qiymatlarini tekshiradi.
func (f EmployeeFilter) Validate() error {
	switch f.Type {
	case "", EmployeeTypeAll, EmployeeTypeTeacher, EmployeeTypeEmployee:
	default:
		return fmt.Errorf("xodim turi %q noma'lum (teacher, employee yoki all)", f.Type)
	}

	codes := map[string]string{
		"jins":          f.Gender,
		"lavozim":       f.StaffPosition,
		"xodim holati":  f.EmployeeStatus,
		"mehnat shakli": f.EmploymentForm,
		"stavka":        f.EmploymentStaff,
		"xodim turi":    f.EmployeeType,
		"ilmiy unvon":   f.AcademicRank,
		"ilmiy daraja":  f.AcademicDegree,
	}
	if err := validateCodes(codes); err != nil {
		return err
	}

	// ⚠️ Talabanikidek: HEMIS ikkalasini birga talab qiladi, bittasi
	// berilsa filtr jimgina e'tiborsiz qoladi (o'lchangan: faqat
	// `passport_pin` bilan to'liq ro'yxat qaytdi).
	if (f.PassportPIN == "") != (f.PassportNo == "") {
		return fmt.Errorf("passport JSHSHIR va seriya-raqam BIRGA berilishi kerak")
	}

	return nil
}

// values — bo'sh bo'lmagan filtrlarni query parametrlariga aylantiradi.
func (f EmployeeFilter) values() map[string]string {
	m := map[string]string{"type": EmployeeTypeAll}
	if t := strings.TrimSpace(f.Type); t != "" {
		m["type"] = t
	}

	str := func(key, v string) {
		if v = strings.TrimSpace(v); v != "" {
			m[key] = v
		}
	}

	str("_gender", f.Gender)
	str("_staff_position", f.StaffPosition)
	str("_employee_status", f.EmployeeStatus)
	str("_employment_form", f.EmploymentForm)
	str("_employment_staff", f.EmploymentStaff)
	str("_employee_type", f.EmployeeType)
	str("_academic_rank", f.AcademicRank)
	str("_academic_degree", f.AcademicDegree)
	str("search", f.Search)
	str("passport_pin", f.PassportPIN)
	str("passport_number", f.PassportNo)

	if f.Department > 0 {
		m["_department"] = strconv.FormatInt(f.Department, 10)
	}

	return m
}

// validateCodes — klassifikator kodlari faqat raqamdan iboratmi.
//
// ⚠️ Raqam kutilgan joyga harf yuborilsa HEMIS talaba ro'yxatida 500
// qaytaradi (400 emas) — shuning uchun xato bizda, so'rovdan OLDIN tutiladi.
func validateCodes(codes map[string]string) error {
	for name, v := range codes {
		if v == "" {
			continue
		}
		if !isDigits(v) {
			return fmt.Errorf("%s kodi faqat raqamdan iborat bo'lishi kerak (%q)", name, v)
		}
	}
	return nil
}
