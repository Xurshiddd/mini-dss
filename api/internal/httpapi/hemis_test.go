package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"minidss/api/internal/config"
)

// TestHemisFilterEndpoints — filtr ro'yxatlari va son endpointlari.
// HEMIS_BASE_URL / HEMIS_TOKEN berilmasa o'tkazib yuboriladi.
func TestHemisFilterEndpoints(t *testing.T) {
	base, token := os.Getenv("HEMIS_BASE_URL"), os.Getenv("HEMIS_TOKEN")
	if base == "" || token == "" {
		t.Skip("HEMIS_BASE_URL / HEMIS_TOKEN berilmagan")
	}

	// Bu ikki endpoint bazaga ham, qurilmaga ham tegmaydi — API'ning
	// faqat sozlamalari kerak.
	a := &API{cfg: config.Config{HemisBase: base, HemisToken: token}}

	t.Run("filters", func(t *testing.T) {
		rec := httptest.NewRecorder()
		a.hemisFilters(rec, httptest.NewRequest(http.MethodGet, "/api/hemis/filters", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
		}

		var body struct {
			EducationForm []struct{ Code, Name string } `json:"education_form"`
			Departments   []struct {
				ID   int64
				Name string
			} `json:"departments"`
			District []struct{ Code, Name, Parent string } `json:"district"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("javob o'qilmadi: %v", err)
		}
		if len(body.EducationForm) == 0 || len(body.Departments) == 0 {
			t.Fatal("ro'yxatlar bo'sh keldi")
		}
		if body.District[0].Parent == "" {
			t.Error("tumanda viloyat kodi yo'q — `_parent` o'qilmagan")
		}
	})

	t.Run("count", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/hemis/student-count",
			strings.NewReader(`{"education_form":"11","level":"12"}`))
		a.hemisStudentCount(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
		}

		var body struct {
			Count       int  `json:"count"`
			Approximate bool `json:"approximate"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("javob o'qilmadi: %v", err)
		}
		if body.Count == 0 {
			t.Error("filtr bo'yicha 0 ta talaba — filtr noto'g'ri ketyapti")
		}
		t.Logf("kunduzgi 2-kurs: %d ta", body.Count)
	})

	t.Run("xato filtr", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/hemis/student-count",
			strings.NewReader(`{"level":"abc"}`))
		a.hemisStudentCount(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("HTTP %d kutilgan 422, javob: %s", rec.Code, rec.Body.String())
		}
	})
}
