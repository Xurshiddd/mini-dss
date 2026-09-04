// Package hemis — HEMIS API'sidan xodim va talaba ma'lumotlarini olish.
//
// Javob shakli: {"success":true,"error":null,"data":{"items":[...]}}
// Sahifalash `page` va `limit` orqali; `items` bo'sh kelganda tugaydi.
package hemis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	base  string
	token string
	http  *http.Client
}

func New(base, token string) *Client {
	return &Client{
		base:  strings.TrimSuffix(base, "/"),
		token: token,
		http:  &http.Client{Timeout: 90 * time.Second},
	}
}

const (
	employeePath = "/rest/v1/data/employee-list"
	studentPath  = "/rest/v1/data/student-list"
)

// ---------------------------------------------------------------- javob shakli

type ref struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// ParentRef — ota bo'lim havolasi.
//
// ⚠️ HEMIS buni IKKI XIL shaklda qaytaradi: ba'zan to'liq obyekt
// ({"id":56,"name":...}), ba'zan shunchaki RAQAM (ota bo'lim id'si),
// ba'zan `null`. Bitta shaklga mo'ljallangan struct hammasini yiqitadi.
type ParentRef struct {
	ID int64
}

func (p *ParentRef) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "null" || s == "" {
		return nil
	}

	// Raqam shakli
	if s[0] != '{' {
		var id int64
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		p.ID = id
		return nil
	}

	// Obyekt shakli — bizga faqat id kerak.
	var obj struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	p.ID = obj.ID
	return nil
}

// Department — HEMIS bo'limi. Ichma-ich bo'lishi mumkin (`parent`).
type Department struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Code          string     `json:"code"`
	StructureType ref        `json:"structureType"`
	Parent        *ParentRef `json:"parent"`
	Active        bool       `json:"active"`
}

type Employee struct {
	ID         int64       `json:"id"`
	FullName   string      `json:"full_name"`
	IDNumber   string      `json:"employee_id_number"`
	Gender     ref         `json:"gender"`
	BirthDate  int64       `json:"birth_date"` // unix
	Image      string      `json:"image"`
	Department *Department `json:"department"`
	StaffPos   ref         `json:"staffPosition"`
	Status     ref         `json:"employeeStatus"`
	Active     bool        `json:"active"`
}

type Student struct {
	ID         int64       `json:"id"`
	FullName   string      `json:"full_name"`
	IDNumber   string      `json:"student_id_number"`
	Gender     ref         `json:"gender"`
	BirthDate  int64       `json:"birth_date"`
	Image      string      `json:"image"`
	Department *Department `json:"department"`
	Specialty  *struct {
		ID   int64  `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"specialty"`
	Group *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"group"`
	Level         ref `json:"level"`
	EducationForm ref `json:"educationForm"`
	EducationType ref `json:"educationType"`
	PaymentForm   ref `json:"paymentForm"`
	StudentStatus ref `json:"studentStatus"`

	// Turar joy turi (h_accommodation): 15 = talabalar turar joyi.
	Accommodation ref `json:"accommodation"`
}

type envelope[T any] struct {
	Success bool `json:"success"`
	Error   any  `json:"error"`
	Data    struct {
		Items      []T `json:"items"`
		Pagination struct {
			TotalCount int `json:"totalCount"`
			PageCount  int `json:"pageCount"`
			Page       int `json:"page"`
		} `json:"pagination"`
	} `json:"data"`
}

func (c *Client) fetch(ctx context.Context, path string, page, limit int,
	extra map[string]string, out any) error {

	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))
	for k, v := range extra {
		q.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("HEMIS tokeni qabul qilinmadi (401) — HEMIS_TOKEN ni tekshiring")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HEMIS HTTP %d (%s)", resp.StatusCode, path)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// EachEmployee — barcha xodimlarni sahifalab qaytaradi.
//
// Kimni qabul qilish/rad etish qarori BU YERDA emas, sync qatlamida —
// shunda rad etilganlar sanaladi va logda ko'rinadi.
func (c *Client) EachEmployee(ctx context.Context, fn func(Employee) error) error {
	// `type=all` bo'lmasa HEMIS faqat bir qismini qaytaradi.
	return paginate(ctx, c, employeePath, map[string]string{"type": "all"}, fn)
}

// Ishdan bo'shatilgan xodim holati kodi.
// Bunday xodim terminalga yozilmaydi va bazada bo'lsa faolsizlantiriladi.
const EmployeeStatusDismissed = "14"

// EachStudent — filtrga tushgan talabalarni sahifalab qaytaradi.
//
// Filtr bo'sh bo'lsa HEMIS o'z standartini qo'llaydi: `studentStatus = 11`
// (O'qimoqda). Barcha holatlar kerak bo'lsa `StudentStatus: "-1"`.
func (c *Client) EachStudent(ctx context.Context, f StudentFilter, fn func(Student) error) error {
	return paginate(ctx, c, studentPath, f.values(), func(s Student) error {
		// So'rovda bajarilmagan filtrlar (masalan to'lov shakli) shu yerda.
		if !f.Match(s) {
			return nil
		}
		return fn(s)
	})
}

// CountStudents — filtrga tushadigan talabalar soni (bitta yengil so'rov).
//
// HEMIS `pagination.totalCount` beradi — butun ro'yxatni tortmasdan
// "nechta topildi" ni bilish uchun shu yagona yo'l.
//
// ⚠️ Mahalliy filtrlar (to'lov shakli) BU YERDA hisobga olinmaydi: son
// so'rov filtri bo'yicha, ya'ni haqiqiy natijadan katta bo'lishi mumkin.
func (c *Client) CountStudents(ctx context.Context, f StudentFilter) (int, error) {
	return c.count(ctx, studentPath, f.values())
}

// CountStudentsExact — mahalliy saralanadigan filtrlarni ham hisobga olib
// sanaydi.
//
// ⚠️ Butun ro'yxatni tortadi (7120 talaba ≈ 36 ta so'rov, ~15 s), chunki
// turar joy va to'lov shakli HEMIS so'rovida yo'q. Shuning uchun bu faqat
// ATAYLAB chaqiriladi — jonli ko'rsatkich `CountStudents` bilan.
func (c *Client) CountStudentsExact(ctx context.Context, f StudentFilter) (int, error) {
	n := 0
	err := c.EachStudent(ctx, f, func(Student) error {
		n++
		return nil
	})
	return n, err
}

// CountEmployees — xodimlar soni.
func (c *Client) CountEmployees(ctx context.Context) (int, error) {
	return c.count(ctx, employeePath, map[string]string{"type": "all"})
}

func (c *Client) count(ctx context.Context, path string, extra map[string]string) (int, error) {
	var env envelope[json.RawMessage]
	if err := c.fetch(ctx, path, 1, 1, extra, &env); err != nil {
		return 0, err
	}
	if !env.Success {
		return 0, fmt.Errorf("HEMIS xato javob berdi (%s): %v", path, env.Error)
	}
	return env.Data.Pagination.TotalCount, nil
}

func paginate[T any](ctx context.Context, c *Client, path string,
	extra map[string]string, fn func(T) error) error {

	const limit = 200

	for page := 1; ; page++ {
		var env envelope[T]
		if err := c.fetch(ctx, path, page, limit, extra, &env); err != nil {
			return err
		}
		if !env.Success {
			return fmt.Errorf("HEMIS xato javob berdi (%s): %v", path, env.Error)
		}
		if len(env.Data.Items) == 0 {
			return nil
		}

		for _, item := range env.Data.Items {
			if err := fn(item); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}

		// Oxirgi sahifa to'liq emas — tugadi.
		if len(env.Data.Items) < limit {
			return nil
		}
	}
}

// BirthDate — HEMIS unix sekundlarda beradi; 0 bo'lsa sana yo'q.
func BirthDate(unix int64) *time.Time {
	if unix <= 0 {
		return nil
	}
	t := time.Unix(unix, 0).UTC()
	return &t
}
