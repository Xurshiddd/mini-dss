package hemis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Filtr ro'yxatlari uchun klassifikator kodlari (HEMIS `classifier-list`).
const (
	clsEducationForm = "h_education_form"
	clsEducationType = "h_education_type"
	clsPaymentForm   = "h_payment_form"
	clsStudentStatus = "h_student_status"
	clsLevel         = "h_course" // ⚠️ "h_level" YO'Q — kurslar `h_course` da
	clsSemester      = "h_semester"
	clsGender        = "h_gender"
	clsCitizenship   = "h_citizenship_type"
	clsAccommodation = "h_accommodation"
	clsSoato         = "h_soato" // viloyat + tuman bitta ro'yxatda
)

// Option — klassifikator qiymati (kod + nom).
type Option struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"` // faqat tumanlar uchun (viloyat kodi)
}

// ⚠️ HEMIS ota kodini `_parent` deb beradi (pastki chiziq bilan), biz esa
// tashqariga `parent` deb chiqaramiz — shuning uchun o'qish qo'lda.
func (o *Option) UnmarshalJSON(data []byte) error {
	var raw struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Parent string `json:"_parent"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	o.Code, o.Name, o.Parent = raw.Code, raw.Name, raw.Parent
	return nil
}

// DepartmentOption — filtr uchun bo'lim.
type DepartmentOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
	Type string `json:"type"` // "Fakultet", "Kafedra" ...
}

// SpecialtyOption — yo'nalish; fakultet bo'yicha saralash uchun `department`.
type SpecialtyOption struct {
	ID         int64  `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	Department int64  `json:"department"`
}

// CurriculumOption — o'quv reja (yo'nalish + o'quv yili + ta'lim shakli).
type CurriculumOption struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Department int64  `json:"department"`
	Specialty  int64  `json:"specialty"`
}

// GroupOption — guruh; fakultet/yo'nalish bo'yicha saralash uchun havolalar.
type GroupOption struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Department int64  `json:"department"`
	Specialty  int64  `json:"specialty"`
}

// FilterOptions — talaba filtrlari uchun barcha tanlov ro'yxatlari.
type FilterOptions struct {
	EducationForm []Option `json:"education_form"`
	EducationType []Option `json:"education_type"`
	PaymentForm   []Option `json:"payment_form"`
	StudentStatus []Option `json:"student_status"`
	Level         []Option `json:"level"`
	Semester      []Option `json:"semester"`
	Gender        []Option `json:"gender"`
	Accommodation []Option `json:"accommodation"`
	Citizenship   []Option `json:"citizenship"`
	Province      []Option `json:"province"`
	District      []Option `json:"district"`

	Departments []DepartmentOption `json:"departments"`
	Specialties []SpecialtyOption  `json:"specialties"`
	Groups      []GroupOption      `json:"groups"`
	Curricula   []CurriculumOption `json:"curricula"`
}

// FilterOptions — filtr ro'yxatlarini HEMIS'dan yig'adi.
//
// To'rtta so'rov guruhi: klassifikatorlar (bitta so'rov), bo'limlar,
// yo'nalishlar va guruhlar. Ro'yxatlar kamdan-kam o'zgaradi — chaqiruvchi
// tomonda keshlanadi.
func (c *Client) FilterOptions(ctx context.Context) (*FilterOptions, error) {
	classifiers, err := c.classifiers(ctx)
	if err != nil {
		return nil, err
	}

	out := &FilterOptions{
		EducationForm: classifiers[clsEducationForm],
		EducationType: classifiers[clsEducationType],
		PaymentForm:   classifiers[clsPaymentForm],
		StudentStatus: classifiers[clsStudentStatus],
		Level:         classifiers[clsLevel],
		Semester:      classifiers[clsSemester],
		Gender:        classifiers[clsGender],
		Accommodation: classifiers[clsAccommodation],
		Citizenship:   classifiers[clsCitizenship],
	}
	sortByCode(out.EducationForm)
	sortByCode(out.EducationType)
	sortByCode(out.PaymentForm)
	sortByCode(out.StudentStatus)
	sortByCode(out.Level)
	sortByCode(out.Semester)
	sortByCode(out.Gender)
	sortByCode(out.Accommodation)
	sortByCode(out.Citizenship)

	// ⚠️ Viloyat va tuman bitta klassifikatorda (`h_soato`) yotadi:
	// viloyatda `_parent` o'z kodiga TENG, tumanda esa viloyat kodini
	// ko'rsatadi. Ajratishning boshqa belgisi yo'q.
	for _, o := range classifiers[clsSoato] {
		if o.Parent == o.Code {
			out.Province = append(out.Province, Option{Code: o.Code, Name: o.Name})
			continue
		}
		out.District = append(out.District, o)
	}
	sortByName(out.Province)
	sortByName(out.District)

	if err := c.loadDepartments(ctx, out); err != nil {
		return nil, err
	}
	if err := c.loadSpecialties(ctx, out); err != nil {
		return nil, err
	}
	if err := c.loadGroups(ctx, out); err != nil {
		return nil, err
	}
	if err := c.loadCurricula(ctx, out); err != nil {
		return nil, err
	}

	return out, nil
}

func (c *Client) classifiers(ctx context.Context) (map[string][]Option, error) {
	type classifier struct {
		Classifier string   `json:"classifier"`
		Options    []Option `json:"options"`
	}

	// Klassifikatorlar bitta sahifaga sig'adi (~95 ta).
	var env envelope[classifier]
	if err := c.fetch(ctx, "/rest/v1/data/classifier-list", 1, 200, nil, &env); err != nil {
		return nil, err
	}
	if !env.Success {
		return nil, fmt.Errorf("HEMIS klassifikatorlarni bermadi: %v", env.Error)
	}

	out := make(map[string][]Option, len(env.Data.Items))
	for _, item := range env.Data.Items {
		out[item.Classifier] = item.Options
	}
	return out, nil
}

func (c *Client) loadDepartments(ctx context.Context, out *FilterOptions) error {
	type dept struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		Code          string `json:"code"`
		Active        bool   `json:"active"`
		StructureType ref    `json:"structureType"`
	}

	err := paginate(ctx, c, "/rest/v1/data/department-list", nil, func(d dept) error {
		if !d.Active {
			return nil
		}
		out.Departments = append(out.Departments, DepartmentOption{
			ID: d.ID, Name: d.Name, Code: d.Code, Type: d.StructureType.Name,
		})
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(out.Departments, func(i, j int) bool {
		return out.Departments[i].Name < out.Departments[j].Name
	})
	return nil
}

func (c *Client) loadSpecialties(ctx context.Context, out *FilterOptions) error {
	type spec struct {
		ID         int64  `json:"id"`
		Code       string `json:"code"`
		Name       string `json:"name"`
		Active     bool   `json:"active"`
		Department *struct {
			ID int64 `json:"id"`
		} `json:"department"`
	}

	err := paginate(ctx, c, "/rest/v1/data/specialty-list", nil, func(s spec) error {
		if !s.Active {
			return nil
		}
		item := SpecialtyOption{ID: s.ID, Code: s.Code, Name: s.Name}
		if s.Department != nil {
			item.Department = s.Department.ID
		}
		out.Specialties = append(out.Specialties, item)
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(out.Specialties, func(i, j int) bool {
		return out.Specialties[i].Name < out.Specialties[j].Name
	})
	return nil
}

func (c *Client) loadGroups(ctx context.Context, out *FilterOptions) error {
	type group struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Active     bool   `json:"active"`
		Department *struct {
			ID int64 `json:"id"`
		} `json:"department"`
		Specialty *struct {
			ID int64 `json:"id"`
		} `json:"specialty"`
	}

	err := paginate(ctx, c, "/rest/v1/data/group-list", nil, func(g group) error {
		if !g.Active {
			return nil
		}
		item := GroupOption{ID: g.ID, Name: g.Name}
		if g.Department != nil {
			item.Department = g.Department.ID
		}
		if g.Specialty != nil {
			item.Specialty = g.Specialty.ID
		}
		out.Groups = append(out.Groups, item)
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(out.Groups, func(i, j int) bool {
		return out.Groups[i].Name < out.Groups[j].Name
	})
	return nil
}

func (c *Client) loadCurricula(ctx context.Context, out *FilterOptions) error {
	type curriculum struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Department *struct {
			ID int64 `json:"id"`
		} `json:"department"`
		Specialty *struct {
			ID int64 `json:"id"`
		} `json:"specialty"`
	}

	// ⚠️ O'quv rejada `active` maydoni yo'q — eskisi ham ro'yxatda qoladi.
	err := paginate(ctx, c, "/rest/v1/data/curriculum-list", nil, func(cu curriculum) error {
		item := CurriculumOption{ID: cu.ID, Name: cu.Name}
		if cu.Department != nil {
			item.Department = cu.Department.ID
		}
		if cu.Specialty != nil {
			item.Specialty = cu.Specialty.ID
		}
		out.Curricula = append(out.Curricula, item)
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(out.Curricula, func(i, j int) bool {
		return out.Curricula[i].Name < out.Curricula[j].Name
	})
	return nil
}

func sortByCode(opts []Option) {
	sort.Slice(opts, func(i, j int) bool { return opts[i].Code < opts[j].Code })
}

func sortByName(opts []Option) {
	sort.Slice(opts, func(i, j int) bool { return opts[i].Name < opts[j].Name })
}
