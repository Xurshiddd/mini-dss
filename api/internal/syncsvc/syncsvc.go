// Package syncsvc — odamlarni terminallarga yozish.
//
// DSS ning modeli: BITTA jarayon, bitta RPC2 sessiya, bitta keep-alive
// ulanish. Har odam uchun alohida job ochilsa, har biriga login/logout
// qo'shiladi va butun tezlik yo'qoladi.
//
// O'lchangan: user yozish 30 ms (RPC2), yuz yuklash ~450 ms (CGI, base64
// hajmi tufayli) → ~500 ms/odam.
package syncsvc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"minidss/api/internal/dahua"
	"minidss/api/internal/store"
)

type Progress struct {
	DeviceID  int64      `json:"device_id"`
	Total     int        `json:"total"`
	Done      int        `json:"done"`
	Users     int        `json:"users"`
	Faces     int        `json:"faces"`
	Failed    int        `json:"failed"`
	Removed   int        `json:"removed"` // terminaldan olib tashlanganlar
	Running   bool       `json:"running"`
	Error     string     `json:"error,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// Service — qurilma bo'yicha bitta sync jarayonini boshqaradi.
//
// ⚠️ Qurilma bir vaqtda juda kam ulanish qabul qiladi — bir sikl ishlab
// turganda boshqasi "connection refused" oladi. Shu sababli har qurilmaga
// BITTADAN sync ruxsat etiladi.
type Service struct {
	store    *store.Store
	photoDir string
	decrypt  func(string) (string, error)
	tz       *time.Location

	// Rasm olishga ruxsat etilgan ishonchli hostlar (SSRF allowlist).
	photoHosts []string

	mu       sync.Mutex
	progress map[int64]*Progress
	hemis    *HemisProgress
	photos   *HemisProgress

	// Hodisa yig'ish (qo'lda va avtomatik) bir vaqtda ketmasin.
	eventsMu sync.Mutex

	// ⚠️ Qurilmaga YOZISH boshlanishidan oldin uning jonli oqimi yopilishi
	// SHART: stream ochiq turganda qurilma port 80'ga yangi ulanish qabul
	// qilmaydi va sync "connection refused" oladi.
	listeners   map[int64]*listener
	listenersMu sync.Mutex
}

func New(st *store.Store, photoDir string, decrypt func(string) (string, error), tz *time.Location, photoHosts []string) *Service {
	return &Service{
		store:      st,
		photoDir:   photoDir,
		decrypt:    decrypt,
		photoHosts: photoHosts,
		tz:         tz,
		progress:   map[int64]*Progress{},
		listeners:  map[int64]*listener{},
	}
}

func (s *Service) Progress(deviceID int64) *Progress {
	s.mu.Lock()
	defer s.mu.Unlock()

	if p, ok := s.progress[deviceID]; ok {
		copied := *p
		return &copied
	}
	return nil
}

func (s *Service) AllProgress() map[int64]Progress {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := map[int64]Progress{}
	for id, p := range s.progress {
		out[id] = *p
	}
	return out
}

// Start — sync'ni fonda boshlaydi. Allaqachon ishlayotgan bo'lsa xato.
func (s *Service) Start(deviceID int64, retryFailed bool, limit int) error {
	s.mu.Lock()
	if p, ok := s.progress[deviceID]; ok && p.Running {
		s.mu.Unlock()
		return fmt.Errorf("bu qurilmada sync allaqachon ketyapti")
	}
	s.progress[deviceID] = &Progress{
		DeviceID:  deviceID,
		Running:   true,
		StartedAt: time.Now(),
	}
	s.mu.Unlock()

	go s.run(deviceID, retryFailed, limit)
	return nil
}

func (s *Service) update(deviceID int64, fn func(*Progress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.progress[deviceID]; ok {
		fn(p)
	}
}

func (s *Service) run(deviceID int64, retryFailed bool, limit int) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	// ⛔ Jonli oqim ochiq turganda qurilma port 80'ga yangi ulanish qabul
	// qilmaydi — sync boshidanoq "connection refused" olardi.
	resume := s.PauseDevice(deviceID)
	defer resume()

	finish := func(err error) {
		now := time.Now()
		s.update(deviceID, func(p *Progress) {
			p.Running = false
			p.EndedAt = &now
			if err != nil {
				p.Error = err.Error()
			}
		})
	}

	dev, err := s.store.Device(ctx, deviceID)
	if err != nil {
		finish(err)
		return
	}

	password, err := s.decrypt(dev.Password)
	if err != nil {
		finish(fmt.Errorf("qurilma paroli ochilmadi: %w", err))
		return
	}

	if limit <= 0 {
		limit = 100000
	}
	people, err := s.store.PeopleToSync(ctx, deviceID, retryFailed, limit)
	if err != nil {
		finish(err)
		return
	}

	// Olib tashlanishi kerak bo'lganlar — nofaol, muddati tugagan yoki
	// o'chirishga belgilanganlar. `retryFailed` faqat yozishni qayta uradi,
	// o'chirishga aloqasi yo'q.
	var removals []store.DeviceRemoval
	if !retryFailed {
		removals, err = s.store.PeopleToRemove(ctx, deviceID, limit)
		if err != nil {
			finish(err)
			return
		}
	}

	s.update(deviceID, func(p *Progress) { p.Total = len(people) + len(removals) })

	client := dahua.New(dahua.Device{
		IP: dev.IP, Port: dev.Port, Username: dev.Username, Password: password,
	}, 60*time.Second)
	defer client.Close()

	// Soatni to'g'rilab olamiz — qurilma soati kuniga bir necha soniya
	// qochadi va NTP tarmoqdan yopilgan bo'lsa boshqa hech kim to'g'rilamaydi.
	// Xatosi sync'ni to'xtatmaydi: bu yordamchi amal, asosiy ish emas.
	_ = client.SetTime(ctx, s.tz)

	if err := client.Login(ctx); err != nil {
		s.store.MarkDeviceError(ctx, deviceID, err.Error())
		finish(err)
		return
	}
	defer client.Logout(ctx)

	// Obyekt handle'i SESSIYAGA bog'liq — sessiya yangilansa qayta olinadi.
	object := 0
	objectGen := -1

	ensureObject := func() (int, error) {
		if object == 0 || objectGen != client.SessionGeneration() {
			obj, err := client.RecordUpdaterInstance(ctx, "AccessControlCard")
			if err != nil {
				return 0, err
			}
			object, objectGen = obj, client.SessionGeneration()
		}
		return object, nil
	}

	// ------------------------------------------------------- olib tashlash
	//
	// Yozishdan OLDIN bajariladi: qurilmada joy bo'shaydi va ruxsati
	// tugaganlar ortiqcha turmaydi.
	//
	// ⚠️ Ikki amal kerak: foydalanuvchi yozuvi RecNo bo'yicha, yuz shabloni
	// esa UserID bo'yicha ALOHIDA o'chiriladi (`RemoveUser` yuzga tegmaydi).
	for i, rm := range removals {
		if ctx.Err() != nil {
			finish(ctx.Err())
			return
		}

		// RecNo bo'lmasa qurilmadagi yozuvni topib bo'lmaydi. Holatni
		// 'synced' qoldiramiz — odam hali qurilmada, buni yashirmaymiz.
		if rm.RecNo == nil {
			_ = s.store.MarkRemovalFailed(ctx, rm.SyncID, "device_recno yo'q — o'chirib bo'lmadi")
			s.update(deviceID, func(p *Progress) { p.Done++; p.Failed++ })
			continue
		}

		if err := client.RemoveUser(ctx, *rm.RecNo); err != nil {
			_ = s.store.MarkRemovalFailed(ctx, rm.SyncID, err.Error())
			s.update(deviceID, func(p *Progress) { p.Done++; p.Failed++ })
			continue
		}

		faceErr := client.RemoveFace(ctx, rm.UserID)

		// ⚠️ Yuz o'chmasa ham 'removed' deb belgilaymiz. Sabab: qurilmadagi
		// foydalanuvchi yozuvi ALLAQACHON yo'q, ya'ni saqlangan RecNo endi
		// yaroqsiz. Uni saqlab qolsak, o'sha raqam boshqa odamga berilganda
		// keyingi sync BEGONA odamni o'chirib yuborardi. Yuz esa qurilmada
		// yetim qolishi mumkin — xatosi `last_error` da ko'rinadi.
		_ = s.store.MarkPersonRemoved(ctx, rm.SyncID)
		if faceErr != nil {
			_ = s.store.MarkRemovalFailed(ctx, rm.SyncID, "yuz o'chmadi: "+faceErr.Error())
		}

		s.update(deviceID, func(p *Progress) { p.Done++; p.Removed++ })

		if i > 0 && i%100 == 0 {
			client.KeepAlive(ctx)
		}
	}

	// O'chirilganlar hamma terminaldan tozalangan bo'lsa — bazadan ham olamiz.
	if _, err := s.store.PurgeDeletedPeople(ctx); err != nil {
		// Tozalash keyingi sync'da qayta uriniladi, sync'ni yiqitmaymiz.
		_ = err
	}

	for i, person := range people {
		if ctx.Err() != nil {
			finish(ctx.Err())
			return
		}

		recNo, faceOK, err := s.pushOne(ctx, client, ensureObject, person)

		state := "pending"
		var errMsg *string
		switch {
		case err != nil:
			msg := err.Error()
			errMsg = &msg
			state = "failed"
		case faceOK:
			state = "synced"
		}

		var recArg *int
		if recNo > 0 {
			recArg = &recNo
		}
		_ = s.store.SaveSyncState(ctx, deviceID, person.ID, state, recArg, faceOK, errMsg)

		s.update(deviceID, func(p *Progress) {
			p.Done++
			if err != nil {
				p.Failed++
				return
			}
			if recNo > 0 {
				p.Users++
			}
			if faceOK {
				p.Faces++
			}
		})

		// Sessiya tugab qolmasligi uchun davriy signal.
		if i > 0 && i%100 == 0 {
			client.KeepAlive(ctx)
		}
	}

	if ident, err := client.Identify(ctx); err == nil {
		_ = s.store.MarkDeviceSeen(ctx, deviceID, map[string]string{
			"model": ident.Model, "firmware": ident.Firmware, "serial": ident.Serial,
		})
	}

	finish(nil)
}

func (s *Service) pushOne(ctx context.Context, client *dahua.Client,
	ensureObject func() (int, error), p store.Person) (int, bool, error) {

	// ⚠️ Odam qurilmada allaqachon bo'lsa (masalan faqat rasmi o'zgargan) —
	// QAYTA QO'SHMAYMIZ, aks holda bir xil UserID ikki marta tushadi.
	// Bunday holatda faqat yuz yangilanadi.
	recNo := 0
	if p.DeviceRecNo != nil {
		recNo = *p.DeviceRecNo
	}

	if recNo == 0 {
		object, err := ensureObject()
		if err != nil {
			return 0, false, err
		}

		validFrom, validTo := "", ""
		if p.ValidFrom != nil {
			validFrom = p.ValidFrom.Format("2006-01-02") + " 00:00:00"
		}
		if p.ValidTo != nil {
			validTo = p.ValidTo.Format("2006-01-02") + " 23:59:59"
		}

		recNo, err = client.InsertUser(ctx, object,
			dahua.CardRecord(p.UserID, p.FullName, validFrom, validTo, 0, 0))
		if err != nil {
			return 0, false, err
		}
	}

	if p.PhotoPath == nil {
		return recNo, false, nil
	}

	// Rasm yo'li bazada `photos/<user_id>.jpg` ko'rinishida saqlanadi.
	jpeg, err := os.ReadFile(filepath.Join(s.photoDir, filepath.Base(*p.PhotoPath)))
	if err != nil {
		return recNo, false, fmt.Errorf("rasm o'qilmadi: %w", err)
	}

	// Odam qurilmada allaqachon bo'lsa, yuzi ham bor bo'lishi mumkin.
	// `add` bunday holatda 400 beradi — eskisini o'chirib, keyin yozamiz.
	upload := client.UploadFace
	if p.DeviceRecNo != nil {
		upload = client.ReplaceFace
	}

	if err := upload(ctx, p.UserID, jpeg); err != nil {
		return recNo, false, err
	}
	return recNo, true, nil
}

// CleanOrphanFaces — qurilmada qolgan egasiz yuz shablonlarini o'chiradi.
//
// Foydalanuvchilar `clear` bilan o'chirilganda yuzlar qoladi (ular alohida
// saqlanadi, ommaviy `clear` esa yuzlar uchun yo'q). Bu funksiya ularni
// birma-bir tozalaydi.
//
// ⚠️ Hozir sync bo'lgan odamlarga TEGMAYDI — ularning yuzi kerak
// (`OrphanFaces` so'rovi `state = 'synced'` ni chiqarib tashlaydi).
func (s *Service) StartCleanFaces(deviceID int64) error {
	s.mu.Lock()
	if p, ok := s.progress[deviceID]; ok && p.Running {
		s.mu.Unlock()
		return fmt.Errorf("bu qurilmada amal allaqachon ketyapti")
	}
	s.progress[deviceID] = &Progress{DeviceID: deviceID, Running: true, StartedAt: time.Now()}
	s.mu.Unlock()

	go s.cleanFaces(deviceID)
	return nil
}

func (s *Service) cleanFaces(deviceID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	resume := s.PauseDevice(deviceID)
	defer resume()

	finish := func(err error) {
		now := time.Now()
		s.update(deviceID, func(p *Progress) {
			p.Running = false
			p.EndedAt = &now
			if err != nil {
				p.Error = err.Error()
			}
		})
	}

	dev, err := s.store.Device(ctx, deviceID)
	if err != nil {
		finish(err)
		return
	}

	password, err := s.decrypt(dev.Password)
	if err != nil {
		finish(fmt.Errorf("qurilma paroli ochilmadi: %w", err))
		return
	}

	orphans, err := s.store.OrphanFaces(ctx, deviceID, 100000)
	if err != nil {
		finish(err)
		return
	}

	s.update(deviceID, func(p *Progress) { p.Total = len(orphans) })

	client := dahua.New(dahua.Device{
		IP: dev.IP, Port: dev.Port, Username: dev.Username, Password: password,
	}, 30*time.Second)
	defer client.Close()

	// Ketma-ket xatolar ko'paysa qurilmaga dam beramiz.
	consecutiveErrors := 0

	for _, orphan := range orphans {
		if ctx.Err() != nil {
			finish(ctx.Err())
			return
		}

		err := client.RemoveFace(ctx, orphan.UserID)

		if err == nil {
			consecutiveErrors = 0
			_ = s.store.MarkFaceRemoved(ctx, orphan.SyncID)
			s.update(deviceID, func(p *Progress) { p.Done++; p.Faces++ })
			continue
		}

		consecutiveErrors++
		s.update(deviceID, func(p *Progress) { p.Done++; p.Failed++ })

		// Qurilma bo'g'ilib qolgan bo'lishi mumkin — kutamiz.
		if consecutiveErrors >= 5 {
			consecutiveErrors = 0
			select {
			case <-time.After(30 * time.Second):
			case <-ctx.Done():
				finish(ctx.Err())
				return
			}
		}
	}

	finish(nil)
}

// Wipe — qurilmadagi BARCHA foydalanuvchini o'chiradi.
//
// ⛔ QAYTARIB BO'LMAYDI. Yuz shablonlarini qurilmadan o'qib olish mumkin
// emas (FaceInfoManager ning birorta read actioni yo'q), ya'ni zaxira
// olinmaydi.
func (s *Service) Wipe(ctx context.Context, deviceID int64) error {
	resume := s.PauseDevice(deviceID)
	defer resume()

	dev, err := s.store.Device(ctx, deviceID)
	if err != nil {
		return err
	}

	password, err := s.decrypt(dev.Password)
	if err != nil {
		return fmt.Errorf("qurilma paroli ochilmadi: %w", err)
	}

	client := dahua.New(dahua.Device{
		IP: dev.IP, Port: dev.Port, Username: dev.Username, Password: password,
	}, 60*time.Second)
	defer client.Close()

	if err := client.ClearAllUsers(ctx); err != nil {
		return err
	}

	remaining, err := client.CountUsers(ctx)
	if err != nil {
		return err
	}
	if remaining != 0 {
		return fmt.Errorf("qurilmada hali %d ta yozuv bor", remaining)
	}

	// Qurilma bo'sh — bazadagi holat ham shunga keltiriladi.
	return s.store.ResetSyncStates(ctx, deviceID)
}
