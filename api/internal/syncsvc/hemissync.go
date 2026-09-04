package syncsvc

import (
	"context"
	"fmt"
	"log"
	"time"

	"minidss/api/internal/hemis"
	"minidss/api/internal/store"
)

// HEMIS → platforma sync.
//
// Bu bosqichda TERMINALGA hech narsa yozilmaydi — faqat baza to'ldiriladi.
// Terminalga yozish alohida bosqich (`Start`).
//
// ⚠️ Rasm maydonlariga tegilmaydi: rasm yuklab olish va validatsiya alohida
// oqim. Sync ularni bekor qilib yuborsa, tekshiruvdan o'tgan rasmlar
// yo'qoladi.

// HemisOptions — sync nimani va qanday filtr bilan tortishini belgilaydi.
//
// Filtrlar faqat TALABAGA tegishli: HEMIS `employee-list` da bunday
// parametrlar yo'q.
type HemisOptions struct {
	Employees bool                `json:"employees"`
	Students  bool                `json:"students"`
	Filter    hemis.StudentFilter `json:"filter"`
}

type HemisProgress struct {
	Running   bool       `json:"running"`
	Stage     string     `json:"stage"` // 'employees' | 'students'
	Total     int        `json:"total"`
	Expected  int        `json:"expected"` // HEMIS aytgan son (progress bar uchun)
	Created   int        `json:"created"`
	Updated   int        `json:"updated"`
	Skipped   int        `json:"skipped"`
	Failed    int        `json:"failed"`
	Error     string     `json:"error,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`

	// Qaysi tanlov bilan ketgani — panelda ko'rinsin.
	Options HemisOptions `json:"options"`
}

func (s *Service) HemisProgress() *HemisProgress {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.hemis == nil {
		return nil
	}
	copied := *s.hemis
	return &copied
}

func (s *Service) StartHemisSync(base, token string, opts HemisOptions) error {
	if base == "" || token == "" {
		return fmt.Errorf("HEMIS manzili yoki tokeni sozlanmagan (HEMIS_BASE_URL, HEMIS_TOKEN)")
	}
	if !opts.Employees && !opts.Students {
		return fmt.Errorf("na xodim, na talaba tanlanmagan — tortadigan narsa yo'q")
	}
	if err := opts.Filter.Validate(); err != nil {
		return err
	}

	stage := "employees"
	if !opts.Employees {
		stage = "students"
	}

	s.mu.Lock()
	if s.hemis != nil && s.hemis.Running {
		s.mu.Unlock()
		return fmt.Errorf("HEMIS sync allaqachon ketyapti")
	}
	s.hemis = &HemisProgress{
		Running: true, StartedAt: time.Now(), Stage: stage, Options: opts,
	}
	s.mu.Unlock()

	go s.runHemis(base, token, opts)
	return nil
}

func (s *Service) updateHemis(fn func(*HemisProgress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hemis != nil {
		fn(s.hemis)
	}
}

func (s *Service) runHemis(base, token string, opts HemisOptions) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()

	runID, _ := s.store.StartRun(ctx, "hemis_pull", nil)
	client := hemis.New(base, token)

	var total, created, updated, skipped, failed, deactivated int

	finish := func(err error) {
		now := time.Now()
		status := "done"
		var errMsg *string
		if err != nil {
			status = "failed"
			msg := err.Error()
			errMsg = &msg
		}
		if deactivated > 0 {
			log.Printf("HEMIS sync: %d ta ishdan bo'shatilgan xodim faolsizlantirildi", deactivated)
		}
		if err := s.store.FinishRun(ctx, runID, status, total, created, updated, skipped, failed, errMsg); err != nil {
			log.Printf("sync log yozilmadi: %v", err)
		}

		s.updateHemis(func(p *HemisProgress) {
			p.Running = false
			p.EndedAt = &now
			if err != nil {
				p.Error = err.Error()
			}
		})
	}

	// Nechta yozuv kutilayotganini oldindan so'raymiz — progress bar shunga
	// tayanadi. Xato bo'lsa ham to'xtamaymiz: son bo'lmasa bar ko'rsatilmaydi.
	expected := 0
	if opts.Employees {
		if n, cErr := client.CountEmployees(ctx); cErr == nil {
			expected += n
		}
	}
	if opts.Students {
		if n, cErr := client.CountStudents(ctx, opts.Filter); cErr == nil {
			expected += n
		}
	}
	s.updateHemis(func(p *HemisProgress) { p.Expected = expected })

	// --- Xodimlar ---
	var err error

	if opts.Employees {
		err = client.EachEmployee(ctx, func(e hemis.Employee) error {
			total++

			if e.IDNumber == "" {
				skipped++ // ID'siz odamni terminalga yuborib bo'lmaydi
				return nil
			}

			// ⚠️ Ishdan bo'shatilgan (employeeStatus.code = 14) va faol bo'lmagan
			// xodimlar QABUL QILINMAYDI.
			//
			// Shunchaki "o'tkazib yuborish" yetarli emas: odam ilgari bazaga
			// tushgan bo'lsa, faol holda qolaverardi va terminalda eshik
			// ochaverardi. Shuning uchun mavjudini faolsizlantiramiz.
			if e.Status.Code == hemis.EmployeeStatusDismissed || !e.Active {
				skipped++
				if n, err := s.store.DeactivateByUserID(ctx, e.IDNumber); err == nil && n > 0 {
					deactivated++
				}
				return nil
			}

			depID := s.saveDepartment(ctx, e.Department)

			_, isNew, err := s.store.UpsertPerson(ctx, store.PersonInput{
				ExternalID:    strPtr(fmt.Sprint(e.ID)),
				Source:        "hemis_employee",
				PersonType:    "employee",
				UserID:        e.IDNumber,
				FullName:      e.FullName,
				Gender:        strPtr(e.Gender.Name),
				BirthDate:     hemis.BirthDate(e.BirthDate),
				DepartmentID:  depID,
				Status:        strPtr(e.Status.Name),
				StaffPosition: strPtr(e.StaffPos.Name),
				PhotoURL:      e.Image,
				IsActive:      e.Active,
			})
			if err != nil {
				failed++
				return nil // bitta yozuv butun sync'ni to'xtatmasin
			}

			if isNew {
				created++
			} else {
				updated++
			}

			s.updateHemis(func(p *HemisProgress) {
				p.Total, p.Created, p.Updated, p.Skipped, p.Failed = total, created, updated, skipped, failed
			})
			return nil
		})
	}

	if err != nil {
		finish(err)
		return
	}

	// --- Talabalar ---
	if !opts.Students {
		finish(nil)
		return
	}

	s.updateHemis(func(p *HemisProgress) { p.Stage = "students" })

	err = client.EachStudent(ctx, opts.Filter, func(st hemis.Student) error {
		total++

		if st.IDNumber == "" {
			skipped++
			return nil
		}

		depID := s.saveDepartment(ctx, st.Department)

		var specialty, group *string
		if st.Specialty != nil {
			specialty = strPtr(st.Specialty.Name)
		}
		if st.Group != nil {
			group = strPtr(st.Group.Name)
		}

		_, isNew, err := s.store.UpsertPerson(ctx, store.PersonInput{
			ExternalID:    strPtr(fmt.Sprint(st.ID)),
			Source:        "hemis_student",
			PersonType:    "student",
			UserID:        st.IDNumber,
			FullName:      st.FullName,
			Gender:        strPtr(st.Gender.Name),
			BirthDate:     hemis.BirthDate(st.BirthDate),
			DepartmentID:  depID,
			Status:        strPtr(st.StudentStatus.Name),
			Specialty:     specialty,
			StudentGroup:  group,
			LevelName:     strPtr(st.Level.Name),
			EducationForm: strPtr(st.EducationForm.Name),
			EducationType: strPtr(st.EducationType.Name),
			PaymentForm:   strPtr(st.PaymentForm.Name),
			PhotoURL:      st.Image,
			// Talabada `active` maydoni yo'q — holati "O'qimoqda" bo'lsa faol.
			IsActive: st.StudentStatus.Code == "11",
		})
		if err != nil {
			failed++
			return nil
		}

		if isNew {
			created++
		} else {
			updated++
		}

		s.updateHemis(func(p *HemisProgress) {
			p.Total, p.Created, p.Updated, p.Skipped, p.Failed = total, created, updated, skipped, failed
		})
		return nil
	})

	// Bo'limlar daraxtini tiklaymiz — ota bo'lim keyinroq kelgan bo'lishi mumkin.
	if n, relinkErr := s.store.RelinkDepartments(ctx); relinkErr == nil && n > 0 {
		// jimgina: bu tuzatuv, xato emas
		_ = n
	}

	finish(err)
}

// saveDepartment — bo'limni (va otasini) saqlaydi, id qaytaradi.
func (s *Service) saveDepartment(ctx context.Context, d *hemis.Department) *int64 {
	if d == nil || d.ID == 0 {
		return nil
	}

	// ⚠️ Ota bo'lim ko'pincha faqat ID bo'lib keladi (nomi yo'q), shuning
	// uchun uni bu yerda YARATMAYMIZ — bo'sh nomli bo'lim hosil bo'lardi.
	// Ota bo'lim o'z yozuvi bilan baribir keladi; bog'lanish sync oxirida
	// `RelinkDepartments` orqali tiklanadi (tartib har xil bo'lishi mumkin).
	var parentExternal *int64
	if d.Parent != nil && d.Parent.ID != 0 {
		parentExternal = &d.Parent.ID
	}

	id, err := s.store.UpsertDepartment(ctx, d.ID, d.Code, d.Name, d.StructureType.Name, parentExternal)
	if err != nil {
		return nil
	}
	return &id
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
