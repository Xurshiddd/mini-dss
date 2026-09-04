package hemis

import (
	"context"
	"os"
	"testing"
)

func TestStudentFilterValues(t *testing.T) {
	f := StudentFilter{
		EducationForm: "11",
		PaymentForm:   "12", // so'rovga TUSHMAYDI (HEMIS'da ishlamaydi)
		Accommodation: "15", // shu ham — mahalliy saralanadi
		Department:    3,
		Group:         0, // bo'sh — qo'shilmaydi
		UpdatedFrom:   1788440000,
		Search:        " SAMADOV ",
	}

	got := f.values()
	want := map[string]string{
		"_education_form": "11",
		"_department":     "3",
		"updated_at_from": "1788440000",
		"search":          "SAMADOV",
	}

	if len(got) != len(want) {
		t.Fatalf("parametrlar soni %d, kutilgan %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, kutilgan %q", k, got[k], v)
		}
	}
}

func TestStudentFilterValidate(t *testing.T) {
	tests := []struct {
		name    string
		filter  StudentFilter
		wantErr bool
	}{
		{"bo'sh", StudentFilter{}, false},
		{"barcha holat", StudentFilter{StudentStatus: "-1"}, false},
		{"harfli kod", StudentFilter{EducationForm: "abc"}, true},
		{"manfiy kurs", StudentFilter{Level: "-1"}, true},
		{"yarim passport", StudentFilter{PassportPIN: "123"}, true},
		{"to'liq passport", StudentFilter{PassportPIN: "123", PassportNo: "AA1"}, false},
		{"teskari oraliq", StudentFilter{UpdatedFrom: 20, UpdatedTo: 10}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.filter.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, xato kutilgani: %v", err, tt.wantErr)
			}
		})
	}
}

func TestStudentFilterMatch(t *testing.T) {
	dorm := Student{}
	dorm.Accommodation.Code = "15"
	dorm.PaymentForm.Code = "11"

	home := Student{}
	home.Accommodation.Code = "11"
	home.PaymentForm.Code = "12"

	tests := []struct {
		name    string
		filter  StudentFilter
		student Student
		want    bool
	}{
		{"filtrsiz", StudentFilter{}, home, true},
		{"yotoqxona — mos", StudentFilter{Accommodation: "15"}, dorm, true},
		{"yotoqxona — mos emas", StudentFilter{Accommodation: "15"}, home, false},
		{"grant + yotoqxona", StudentFilter{Accommodation: "15", PaymentForm: "11"}, dorm, true},
		{"yotoqxona, lekin shartnoma", StudentFilter{Accommodation: "15", PaymentForm: "12"}, dorm, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Match(tt.student); got != tt.want {
				t.Fatalf("Match() = %v, kutilgan %v", got, tt.want)
			}
		})
	}
}

// TestLiveHEMIS — haqiqiy HEMIS bilan tekshiruv.
// HEMIS_BASE_URL va HEMIS_TOKEN berilmasa o'tkazib yuboriladi.
func TestLiveHEMIS(t *testing.T) {
	base, token := os.Getenv("HEMIS_BASE_URL"), os.Getenv("HEMIS_TOKEN")
	if base == "" || token == "" {
		t.Skip("HEMIS_BASE_URL / HEMIS_TOKEN berilmagan")
	}

	c := New(base, token)
	ctx := context.Background()

	total, err := c.CountStudents(ctx, StudentFilter{})
	if err != nil {
		t.Fatalf("CountStudents: %v", err)
	}
	filtered, err := c.CountStudents(ctx, StudentFilter{EducationForm: "11"})
	if err != nil {
		t.Fatalf("CountStudents (filtrli): %v", err)
	}
	if filtered == 0 || filtered >= total {
		t.Errorf("filtr ishlamadi: jami %d, kunduzgi %d", total, filtered)
	}

	// ⚠️ Turar joy HEMIS so'rovida ishlamaydi — mahalliy saralanadi, ya'ni
	// aniq son so'rov soniidan KICHIK bo'lishi shart.
	dormFilter := StudentFilter{Accommodation: "15"}
	if n, err := c.CountStudents(ctx, dormFilter); err != nil {
		t.Fatalf("CountStudents (yotoqxona): %v", err)
	} else if n != total {
		t.Errorf("_accommodation so'rovda ishlab ketibdi: %d != %d — Match'ni qayta ko'ring", n, total)
	}

	exact, err := c.CountStudentsExact(ctx, dormFilter)
	if err != nil {
		t.Fatalf("CountStudentsExact: %v", err)
	}
	if exact == 0 || exact >= total {
		t.Errorf("yotoqxona soni shubhali: %d (jami %d)", exact, total)
	}
	t.Logf("yotoqxonada: %d / %d", exact, total)

	opts, err := c.FilterOptions(ctx)
	if err != nil {
		t.Fatalf("FilterOptions: %v", err)
	}
	switch {
	case len(opts.EducationForm) == 0:
		t.Error("ta'lim shakllari bo'sh")
	case len(opts.Level) == 0:
		t.Error("kurslar bo'sh")
	case len(opts.Departments) == 0:
		t.Error("bo'limlar bo'sh")
	case len(opts.Groups) == 0:
		t.Error("guruhlar bo'sh")
	case len(opts.Province) == 0 || len(opts.District) == 0:
		t.Error("viloyat/tuman ajratilmadi")
	case len(opts.Accommodation) == 0:
		t.Error("turar joy turlari bo'sh")
	}

	t.Logf("shakl=%d kurs=%d bo'lim=%d yo'nalish=%d guruh=%d reja=%d viloyat=%d tuman=%d",
		len(opts.EducationForm), len(opts.Level), len(opts.Departments),
		len(opts.Specialties), len(opts.Groups), len(opts.Curricula),
		len(opts.Province), len(opts.District))
}
