package httpapi

import (
	"slices"
	"testing"
)

func TestCleanPersonTypes(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
		bad  bool
	}{
		// Bo'sh ro'yxat "filtr yo'q" degani: SQL'da NULL bo'lib ketsin.
		{name: "bo'sh", in: nil, want: nil},
		{name: "bitta", in: []string{"employee"}, want: []string{"employee"}},
		{
			name: "hammasi",
			in:   []string{"employee", "student", "other"},
			want: []string{"employee", "student", "other"},
		},
		{
			name: "takror tashlanadi",
			in:   []string{"student", "student", "employee"},
			want: []string{"student", "employee"},
		},
		{name: "noma'lum tur", in: []string{"teacher"}, bad: true},
		// Bo'sh satr filtrni jimgina "hamma tur" ga aylantirmasligi kerak.
		{name: "bo'sh satr", in: []string{""}, bad: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanPersonTypes(tc.in)
			if tc.bad {
				if err == nil {
					t.Fatalf("xato kutilgandi, %v keldi", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("kutilmagan xato: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
