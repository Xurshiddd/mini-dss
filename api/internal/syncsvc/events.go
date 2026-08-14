package syncsvc

import (
	"context"
	"fmt"
	"log"
	"time"

	"minidss/api/internal/dahua"
	"minidss/api/internal/store"
)

// ImportEvents — qurilmadagi yangi kirish/chiqish yozuvlarini bazaga ko'chiradi.
//
// Bazadagi eng katta `RecNo` dan davom etadi, shuning uchun qayta chaqirish
// xavfsiz va arzon — yangi yozuv bo'lmasa bitta sahifa o'qib to'xtaydi.
//
// `maxPages` — birinchi yig'ishda 100 000 yozuvli logni butunlay tortib
// olmaslik uchun cheklov (bir sahifa = 200 yozuv).
func (s *Service) ImportEvents(ctx context.Context, deviceID int64, maxPages int) (int, error) {
	if maxPages <= 0 {
		maxPages = 5
	}

	// ⛔ Jonli oqim ochiq turganda qurilma yangi ulanish qabul qilmaydi.
	resume := s.PauseDevice(deviceID)
	defer resume()

	dev, err := s.store.Device(ctx, deviceID)
	if err != nil {
		return 0, err
	}

	password, err := s.decrypt(dev.Password)
	if err != nil {
		return 0, fmt.Errorf("qurilma paroli ochilmadi: %w", err)
	}

	client := dahua.New(dahua.Device{
		IP: dev.IP, Port: dev.Port, Username: dev.Username, Password: password,
	}, 2*time.Minute)
	defer client.Close()

	last, err := s.store.LastEventRecNo(ctx, deviceID)
	if err != nil {
		return 0, err
	}

	// ⚠️ Xato bo'lsa ham `events` bo'sh emas: qurilma yarim yo'lda javob
	// bermay qo'yishi mumkin, o'shangacha o'qilgani esa yaroqli. Avval
	// saqlaymiz, keyin xatoni qaytaramiz — aks holda har urinishda hammasi
	// boshidan o'qilib, qurilma yana bo'g'ilardi.
	events, fetchErr := client.EventsSince(ctx, last, maxPages)

	rows := make([]store.AccessEvent, 0, len(events))
	for _, e := range events {
		rows = append(rows, store.AccessEvent{
			RecNo:       e.RecNo,
			UserID:      e.UserID,
			HappenedAt:  e.HappenedAt,
			Status:      e.Status,
			ErrorCode:   e.ErrorCode,
			Method:      e.Method,
			SnapshotURL: e.SnapshotURL,
		})
	}

	// ⚠️ Yo'nalish QURILMADAN olinadi — hodisadagi `Type` maydonidan emas.
	n, err := s.store.SaveEvents(ctx, deviceID, dev.Direction, rows)
	if err != nil {
		return n, err
	}
	return n, fetchErr
}

// ImportAllEvents — barcha faol qurilmalardan yig'adi.
//
// Bitta qurilma xato bersa qolganlari davom etadi: bitta terminal o'chiq
// bo'lgani uchun butun hisobot to'xtab qolmasligi kerak.
func (s *Service) ImportAllEvents(ctx context.Context, maxPages int) (int, map[string]string) {
	// ⚠️ Avtomatik yig'ish bilan qo'lda yangilash bir vaqtda ishlamasin —
	// aks holda ikkalasi bir xil sahifalarni o'qib, qurilmani ikki barobar
	// yuklaydi.
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()

	devices, err := s.store.ActiveDevices(ctx)
	if err != nil {
		return 0, map[string]string{"": err.Error()}
	}

	total := 0
	errs := map[string]string{}

	for _, dev := range devices {
		// Xato bo'lsa ham `n` noldan katta bo'lishi mumkin — qisman yig'ilgani
		// ham hisobga olinadi.
		n, err := s.ImportEvents(ctx, dev.ID, maxPages)
		total += n
		if err != nil {
			errs[dev.Name] = err.Error()
		}
	}

	return total, errs
}

// PollEvents — hodisalarni davriy yig'ib turadi. Bloklaydi, `go` bilan
// chaqiriladi.
//
// Har qadamda BITTA sahifa o'qiladi (200 yozuv): terminal shuncha vaqtda
// undan ko'p odam o'tkazmaydi, ortiqcha sahifa esa bekorga yuk bo'ladi.
//
// ⚠️ Qo'lda yangilash ketayotgan bo'lsa, bu qadam O'TKAZIB YUBORILADI.
// Navbat kutish qurilmaga so'rovlarni to'plab yuborishga olib kelardi —
// probe hujjatiga ko'ra u ketma-ket so'rovlarga sezgir.
func (s *Service) PollEvents(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.eventsMu.TryLock() {
				continue // qo'lda yangilash ketyapti
			}
			s.eventsMu.Unlock()

			n, errs := s.ImportAllEvents(ctx, 1)
			if n > 0 {
				log.Printf("hodisa yig'ish: %d ta yangi yozuv", n)
			}
			for name, msg := range errs {
				log.Printf("hodisa yig'ish (%s): %s", name, msg)
			}
		}
	}
}
