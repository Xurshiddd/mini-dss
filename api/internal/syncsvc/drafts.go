package syncsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"minidss/api/internal/dahua"
	"minidss/api/internal/store"
)

// Terminaldan yuz rasmlarini QORALAMA sifatida yig'ish.
//
// Nega kerak: eski DSS orqali kiritilgan odamlarning rasmi faqat
// terminalda qolgan. HEMIS ular uchun yuz bermaydi yoki yaroqsiz rasm
// qaytaradi — o'shanda terminaldagi nusxa yagona manba bo'lib qoladi.
//
// ⚠️ Yig'ish hech kimning rasmini O'ZGARTIRMAYDI. Qoralama alohida
// jadvalda yotadi va faqat operator (yoki avtomatik moslashtirish)
// biriktirgandan keyin terminalga ketadi.

const (
	// ⚠️ Qurilma ketma-ket so'rovlarga sezgir: 300 ms oraliqda ~90 ta
	// so'rovdan keyin javob bermay qo'ygani o'lchangan. Bitta so'rovda
	// `FaceBatchSize` ta yuz kelgani uchun oraliq kengroq.
	faceReadDelay = 700 * time.Millisecond

	// Ketma-ket shuncha XATO so'rovdan keyin to'xtaymiz. Yuzi yo'q odam
	// xato EMAS — u shunchaki javobga tushmaydi.
	faceReadGiveUp = 5
)

// DraftDir — qoralama rasmlari katalogi. Odam rasmlari bilan ARALASHMAYDI:
// qoralama hali hech kimga tegishli emas va uni tasodifan terminalga
// yuborib bo'lmasligi kerak.
func (s *Service) DraftDir() string { return filepath.Join(s.photoDir, "drafts") }

func (s *Service) DraftProgress() *HemisProgress {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.drafts == nil {
		return nil
	}
	copied := *s.drafts
	return &copied
}

func (s *Service) updateDrafts(fn func(*HemisProgress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.drafts != nil {
		fn(s.drafts)
	}
}

// StartDraftPull — yig'ishni fonda boshlaydi.
//
// `deviceID = 0` bo'lsa terminal AVTOMATIK tanlanadi: hammasidagi
// foydalanuvchilar soni sanaladi va eng ko'pi olinadi. Sabab — eski DSS
// hamma terminalga bir xil yozmagan, eng to'la ro'yxat eng ko'p yozuvli
// qurilmada.
func (s *Service) StartDraftPull(faceAPI string, deviceID int64, limit int) error {
	s.mu.Lock()
	if s.drafts != nil && s.drafts.Running {
		s.mu.Unlock()
		return fmt.Errorf("qoralama yig'ish allaqachon ketyapti")
	}
	s.drafts = &HemisProgress{Running: true, StartedAt: time.Now(), Stage: "tanlanmoqda"}
	s.mu.Unlock()

	go s.pullDrafts(faceAPI, deviceID, limit)
	return nil
}

func (s *Service) pullDrafts(faceAPI string, deviceID int64, limit int) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	var (
		total, created, skipped, failed int
		runID                           int64
	)

	finish := func(err error) {
		now := time.Now()
		status := "done"
		var errMsg *string
		if err != nil {
			status = "failed"
			msg := err.Error()
			errMsg = &msg
		}
		if runID != 0 {
			if e := s.store.FinishRun(ctx, runID, status, total, created, 0, skipped, failed, errMsg); e != nil {
				log.Printf("sync log yozilmadi: %v", e)
			}
		}
		s.updateDrafts(func(p *HemisProgress) {
			p.Running = false
			p.EndedAt = &now
			if err != nil {
				p.Error = err.Error()
			}
		})
	}

	if err := os.MkdirAll(s.DraftDir(), 0o755); err != nil {
		finish(fmt.Errorf("qoralama katalogi yaratilmadi: %w", err))
		return
	}

	dev, err := s.pickDevice(ctx, deviceID)
	if err != nil {
		finish(err)
		return
	}

	runID, _ = s.store.StartRun(ctx, "draft_pull", &dev.ID)
	s.updateDrafts(func(p *HemisProgress) { p.Stage = dev.Name })

	password, err := s.decrypt(dev.Password)
	if err != nil {
		finish(fmt.Errorf("qurilma paroli ochilmadi: %w", err))
		return
	}

	// ⛔ Jonli oqim ochiq turganda qurilma yangi ulanish qabul qilmaydi.
	resume := s.PauseDevice(dev.ID)
	defer resume()

	client := dahua.New(dahua.Device{
		IP: dev.IP, Port: dev.Port, Username: dev.Username, Password: password,
	}, 2*time.Minute)
	defer client.Close()
	defer client.Logout(ctx)

	users, err := fetchAllUsers(ctx, client, limit)
	if err != nil {
		s.store.MarkDeviceError(ctx, dev.ID, err.Error())
		finish(err)
		return
	}

	// Allaqachon olinganlarni qayta o'qimaymiz — har biri ~0.5 s.
	existing, err := s.store.DraftUserIDs(ctx)
	if err != nil {
		finish(err)
		return
	}

	s.updateDrafts(func(p *HemisProgress) { p.Total = len(users); p.Expected = len(users) })

	// ⚠️ Yuzlar BITTALAB emas, to'da bo'lib o'qiladi (`AccessFace.list`):
	// 9 800 ta alohida so'rov qurilmani soatlab band qiladi va uni
	// bo'g'ib qo'yish xavfi bor.
	pending := make([]string, 0, dahua.FaceBatchSize)
	names := make(map[string]string, dahua.FaceBatchSize)

	consecutive := 0
	first := true

	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		defer func() {
			pending = pending[:0]
			clear(names)
		}()

		if !first {
			select {
			case <-time.After(faceReadDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		first = false

		faces, err := client.FetchFaces(ctx, pending)
		if err != nil {
			failed += len(pending)
			consecutive++
			s.updateDrafts(func(p *HemisProgress) { p.Failed = failed })

			// ⚠️ Yuz o'qish yo'li butunlay yopilgan bo'lsa (firmware
			// yangilangan, metod nomi o'zgargan) — davom etish foydasiz.
			if consecutive >= faceReadGiveUp {
				return fmt.Errorf(
					"%d ta ketma-ket so'rov natija bermadi — yuz o'qish yo'li yopiq "+
						"(GET /api/devices/%d/face-probe bilan tekshiring, oxirgi xato: %v)",
					consecutive, dev.ID, err)
			}
			return nil
		}
		consecutive = 0

		for _, userID := range pending {
			jpeg, ok := faces[userID]
			if !ok {
				// Kartasi bor, yuzi yo'q — bu odatiy hol, xato emas.
				skipped++
				s.updateDrafts(func(p *HemisProgress) { p.Skipped = skipped })
				continue
			}

			if err := s.saveDraft(ctx, faceAPI, dev.ID, userID, names[userID], jpeg); err != nil {
				log.Printf("qoralama saqlanmadi (%s): %v", userID, err)
				failed++
				s.updateDrafts(func(p *HemisProgress) { p.Failed = failed })
				continue
			}

			created++
			s.updateDrafts(func(p *HemisProgress) {
				p.Created = created
				p.Updated = created + skipped
			})
		}
		return nil
	}

	for _, u := range users {
		if ctx.Err() != nil {
			finish(ctx.Err())
			return
		}

		userID := u["UserID"]
		if userID == "" {
			continue
		}
		if _, ok := existing[userID]; ok {
			skipped++
			s.updateDrafts(func(p *HemisProgress) { p.Skipped = skipped })
			continue
		}

		pending = append(pending, userID)
		names[userID] = u["CardName"]

		if len(pending) >= dahua.FaceBatchSize {
			if err := flush(); err != nil {
				finish(err)
				return
			}
		}
	}

	if err := flush(); err != nil {
		finish(err)
		return
	}

	total = created + skipped + failed
	finish(nil)
}

// pickDevice — qaysi terminaldan yig'amiz.
//
// ⚠️ Har bir qurilmada foydalanuvchi soni sanaladi (yengil, bitta so'rov).
// Eng ko'p yozuvlisi tanlanadi: eski DSS hamma terminalga bir xil
// yozmagan, to'la ro'yxat eng to'lasida.
func (s *Service) pickDevice(ctx context.Context, deviceID int64) (store.Device, error) {
	if deviceID != 0 {
		return s.store.Device(ctx, deviceID)
	}

	devices, err := s.store.ActiveDevices(ctx)
	if err != nil {
		return store.Device{}, err
	}
	if len(devices) == 0 {
		return store.Device{}, fmt.Errorf("faol terminal yo'q")
	}

	var (
		best  store.Device
		count int
	)

	for _, d := range devices {
		password, err := s.decrypt(d.Password)
		if err != nil {
			continue
		}

		func() {
			resume := s.PauseDevice(d.ID)
			defer resume()

			client := dahua.New(dahua.Device{
				IP: d.IP, Port: d.Port, Username: d.Username, Password: password,
			}, 30*time.Second)
			defer client.Close()

			n, err := client.CountUsers(ctx)
			if err != nil {
				s.store.MarkDeviceError(ctx, d.ID, err.Error())
				return
			}
			if n > count {
				best, count = d, n
			}
		}()
	}

	if count == 0 {
		return store.Device{}, fmt.Errorf("birorta terminaldan foydalanuvchi ro'yxati olinmadi")
	}
	return best, nil
}

// fetchAllUsers — terminaldagi foydalanuvchilarni sahifalab o'qiydi.
//
// ⚠️ Vaqt bo'yicha filtr bu firmware'da jimgina ishlamaydi, stateful
// finder ham yo'q — yagona yo'l offset.
func fetchAllUsers(ctx context.Context, client *dahua.Client, limit int) ([]map[string]string, error) {
	const page = 200

	total, err := client.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	if limit > 0 && limit < total {
		total = limit
	}

	var out []map[string]string
	for offset := 0; offset < total; offset += page {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}

		count := page
		if offset+count > total {
			count = total - offset
		}

		records, err := client.FetchUsers(ctx, offset, count)
		if err != nil {
			return out, err
		}
		if len(records) == 0 {
			break
		}
		out = append(out, records...)

		// Qurilmani bo'g'ib qo'ymaslik uchun.
		select {
		case <-time.After(250 * time.Millisecond):
		case <-ctx.Done():
			return out, ctx.Err()
		}
	}
	return out, nil
}

// saveDraft — olingan rasmni diskka yozib, qoralama yozuvini saqlaydi.
func (s *Service) saveDraft(ctx context.Context, faceAPI string, deviceID int64,
	userID, name string, jpeg []byte) error {

	fileName := safeFileName(userID) + ".jpg"
	if err := os.WriteFile(filepath.Join(s.DraftDir(), fileName), jpeg, 0o644); err != nil {
		return err
	}

	// ⚠️ face-api javob bermasa rasm "yaroqsiz" deb belgilanMAYDI — bu
	// bizning nosozligimiz, rasmning aybi emas.
	valid, reasons, m := validateWithFaceAPI(ctx, faceAPI, jpeg, fileName)
	metrics, _ := json.Marshal(m)

	status, reason := "rejected", reasons
	switch {
	case reasons == "face_api_unavailable":
		status, reason = "pending", ""
	case valid:
		status, reason = "valid", ""
	}

	_, _, err := s.store.UpsertDraft(ctx, store.DraftInput{
		UserID:       userID,
		FullName:     name,
		DeviceID:     deviceID,
		FileName:     fileName,
		Status:       status,
		RejectReason: reason,
		Metrics:      metrics,
	})
	return err
}

// safeFileName — UserID fayl nomiga aylanadi.
//
// ⚠️ Terminaldagi UserID ixtiyoriy alfanumerik satr; `/` yoki `..` tushsa
// fayl katalogdan chiqib ketardi.
func safeFileName(userID string) string {
	out := make([]rune, 0, len(userID))
	for _, r := range userID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}
