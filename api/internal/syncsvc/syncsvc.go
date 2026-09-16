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
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"minidss/api/internal/dahua"
	"minidss/api/internal/store"
)

type Progress struct {
	DeviceID  int64      `json:"device_id"`
	Operation string     `json:"operation"`
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
	drafts   *HemisProgress

	// Hodisa yig'ish (qo'lda va avtomatik) bir vaqtda ketmasin.
	eventsMu sync.Mutex

	// ⚠️ Qurilmaga YOZISH boshlanishidan oldin uning jonli oqimi yopilishi
	// SHART: stream ochiq turganda qurilma port 80'ga yangi ulanish qabul
	// qilmaydi va sync "connection refused" oladi.
	listeners   map[int64]*listener
	listenersMu sync.Mutex

	// Terminal bir vaqtda faqat bitta boshqaruv operatsiyasini ko'taradi.
	operations   map[int64]string
	operationsMu sync.Mutex
}

var ErrDeviceBusy = errors.New("qurilma boshqa amal bilan band")

func New(st *store.Store, photoDir string, decrypt func(string) (string, error), tz *time.Location, photoHosts []string) *Service {
	return &Service{
		store:      st,
		photoDir:   photoDir,
		decrypt:    decrypt,
		photoHosts: photoHosts,
		tz:         tz,
		progress:   map[int64]*Progress{},
		listeners:  map[int64]*listener{},
		operations: map[int64]string{},
	}
}

// BeginDeviceOperation qurilmaga murojaatlarni bitta jarayonga ketma-ketlaydi.
// Qaytgan funksiya barcha chiqish yo'llarida chaqirilishi shart.
func (s *Service) BeginDeviceOperation(deviceID int64, operation string) (func(), error) {
	s.operationsMu.Lock()
	if current, ok := s.operations[deviceID]; ok {
		s.operationsMu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrDeviceBusy, current)
	}
	s.operations[deviceID] = operation
	s.operationsMu.Unlock()

	return func() {
		s.operationsMu.Lock()
		delete(s.operations, deviceID)
		s.operationsMu.Unlock()
	}, nil
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

// syncJob — bitta sync siklining tanlovi.
type syncJob struct {
	retryFailed bool
	limit       int

	// Faqat shu odamlar (bo'sh = qurilmaga tegishli hammasi). Tanlangan
	// odam allaqachon yozilgan bo'lsa ham qayta yuboriladi va bu siklda
	// hech kim terminaldan OLIB TASHLANMAYDI.
	personIDs []int64
}

// Start — sync'ni fonda boshlaydi. Allaqachon ishlayotgan bo'lsa xato.
func (s *Service) Start(deviceID int64, retryFailed bool, limit int) error {
	return s.start(deviceID, syncJob{retryFailed: retryFailed, limit: limit})
}

// StartSelected — faqat tanlangan odamlarni qurilmaga yozadi.
func (s *Service) StartSelected(deviceID int64, personIDs []int64) error {
	if len(personIDs) == 0 {
		return fmt.Errorf("hech kim tanlanmagan")
	}
	return s.start(deviceID, syncJob{personIDs: personIDs})
}

func (s *Service) start(deviceID int64, job syncJob) error {
	release, err := s.BeginDeviceOperation(deviceID, "sync")
	if err != nil {
		return err
	}
	s.mu.Lock()
	if p, ok := s.progress[deviceID]; ok && p.Running {
		s.mu.Unlock()
		release()
		return fmt.Errorf("bu qurilmada sync allaqachon ketyapti")
	}
	s.progress[deviceID] = &Progress{
		DeviceID:  deviceID,
		Operation: "sync",
		Running:   true,
		StartedAt: time.Now(),
	}
	s.mu.Unlock()

	go s.run(deviceID, job, release)
	return nil
}

func (s *Service) update(deviceID int64, fn func(*Progress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.progress[deviceID]; ok {
		fn(p)
	}
}

func (s *Service) run(deviceID int64, job syncJob, release func()) {
	defer release()
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

	limit := job.limit
	if limit <= 0 {
		limit = 100000
	}

	var (
		people   []store.Person
		removals []store.DeviceRemoval
	)

	if len(job.personIDs) > 0 {
		// Tanlab yuborish: faqat shular, olib tashlash yo'q.
		people, err = s.store.PeopleToSyncSelected(ctx, deviceID, job.personIDs)
		if err != nil {
			finish(err)
			return
		}
	} else {
		people, err = s.store.PeopleToSync(ctx, deviceID, job.retryFailed, limit)
		if err != nil {
			finish(err)
			return
		}

		// Olib tashlanishi kerak bo'lganlar — nofaol, muddati tugagan yoki
		// o'chirishga belgilanganlar. `retryFailed` faqat yozishni qayta uradi,
		// o'chirishga aloqasi yo'q.
		if !job.retryFailed {
			removals, err = s.store.PeopleToRemove(ctx, deviceID, limit)
			if err != nil {
				finish(err)
				return
			}
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

	// ⚠️ Qurilma "o'lganda" (yangi TCP ulanish qabul qilmay qo'yganda) har
	// bir odam 10 soniyalik dial timeout'ga tushadi. Bir marta shunday
	// bo'lgan: 2509 odam yozilgach terminal javob bermay qo'ydi va sync
	// qolgan 718 odamni IKKI SOAT davomida "failed" qilib chiqdi, keyin
	// terminal soatlab o'ziga kelmadi.
	//
	// Shuning uchun ulanish xatolari sanaladi: chegaradan oshsa qurilmaga
	// dam beriladi, u ham yordam bermasa sync TO'XTAYDI — qolganlari
	// keyingi siklga qoladi va "failed" bo'lib iflos bo'lmaydi.
	unreachable := 0

	stopped := func(err error) bool {
		if !dahua.Unreachable(err) {
			unreachable = 0
			return false
		}

		unreachable++
		if unreachable < deviceRetries {
			return false
		}
		return !s.restDevice(ctx, client, deviceID, &unreachable)
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

		if err := s.removeUser(ctx, client, ensureObject, *rm.RecNo); err != nil {
			if stopped(err) {
				finish(deviceGaveUp(err, i))
				return
			}
			_ = s.store.MarkRemovalFailed(ctx, rm.SyncID, err.Error())
			s.update(deviceID, func(p *Progress) { p.Done++; p.Failed++ })
			continue
		}
		unreachable = 0

		// ⚠️ Yuz RPC2 orqali o'chiriladi (70 ms), CGI orqali emas (~700 ms
		// va uchta yangi TCP ulanish).
		faceErr := client.RemoveFaces(ctx, []string{rm.UserID})

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

	// ⚠️ Odam yozish IKKI BOSQICH: foydalanuvchi yozuvi (RPC2, ~0.26 s) va
	// yuz (RPC2, ~0.45 s). Yuzlar KICHIK TO'DA bo'lib ketadi — to'da katta
	// tezlik bermaydi (o'lchangan: 4 talikda atigi ~14%), lekin so'rovlar
	// sonini to'rt barobar kamaytiradi va qurilmaning yozish sur'ati
	// chegarasiga kamroq uriladi.
	faces := make([]faceJob, 0, dahua.FaceWriteBatch)

	flush := func() error {
		if len(faces) == 0 {
			return nil
		}
		err := s.writeFaces(ctx, client, deviceID, faces)
		faces = faces[:0]
		return err
	}

	for i, person := range people {
		if ctx.Err() != nil {
			finish(ctx.Err())
			return
		}

		recNo, err := s.ensureUser(ctx, client, ensureObject, deviceID, person)
		if err != nil {
			if stopped(err) {
				finish(deviceGaveUp(err, i))
				return
			}
			msg := err.Error()
			_ = s.store.SaveSyncState(ctx, deviceID, person.ID, "failed", nil, false, "", &msg)
			s.update(deviceID, func(p *Progress) { p.Done++; p.Failed++ })
			continue
		}

		// Faqat karta ma'lumoti o'zgargan bo'lsa mavjud yuzni qayta yozmaymiz.
		if len(job.personIDs) == 0 && person.DeviceFaceSynced &&
			person.DeviceUserHash != person.DesiredUserHash {
			if err := s.saveSyncState(ctx, deviceID, person.ID, "synced", &recNo, true,
				person.DesiredUserHash, nil); err != nil {
				finish(fmt.Errorf("karta holati bazaga yozilmadi: %w", err))
				return
			}
			s.update(deviceID, func(p *Progress) { p.Done++ })
			continue
		}

		jpeg, err := s.readPhoto(person)
		if err != nil {
			// Foydalanuvchi qurilmada bor, faqat yuzi yo'q — RecNo saqlanadi.
			msg := err.Error()
			var recArg *int
			if recNo > 0 {
				recArg = &recNo
			}
			_ = s.store.SaveSyncState(ctx, deviceID, person.ID, "failed", recArg, false, person.DesiredUserHash, &msg)
			s.update(deviceID, func(p *Progress) { p.Done++; p.Failed++ })
			continue
		}

		faces = append(faces, faceJob{
			person: person,
			recNo:  recNo,
			jpeg:   jpeg,
			// Bazamiz bo'yicha qurilmada yuzi bormi: `insertMulti` mavjud
			// yuz ustiga yozmaydi, `updateMulti` yozadi.
			replace: person.DeviceFaceSynced,
		})

		if len(faces) >= dahua.FaceWriteBatch {
			switch err := flush(); {
			case err == nil:
				unreachable = 0 // qurilma javob beryapti
			case stopped(err):
				finish(deviceGaveUp(err, i))
				return
			}
		}

		// Sessiya tugab qolmasligi uchun davriy signal.
		if i > 0 && i%200 == 0 {
			client.KeepAlive(ctx)
		}
	}

	if err := flush(); err != nil && dahua.Unreachable(err) {
		finish(deviceGaveUp(err, len(people)))
		return
	}

	if ident, err := client.Identify(ctx); err == nil {
		_ = s.store.MarkDeviceSeen(ctx, deviceID, map[string]string{
			"model": ident.Model, "firmware": ident.Firmware, "serial": ident.Serial,
		})
	}

	finish(nil)
}

// faceJob — yuzi yozilishi kutilayotgan odam.
type faceJob struct {
	person  store.Person
	recNo   int
	jpeg    []byte
	replace bool
}

const (
	// Ketma-ket nechta ulanish xatosidan keyin qurilmaga dam beriladi.
	deviceRetries = 3
	// Dam muddati va nechta marta urinib ko'riladi.
	deviceRest     = 45 * time.Second
	deviceRestTry  = 2
	rateLimitPause = 3 * time.Second
)

func deviceGaveUp(err error, done int) error {
	return fmt.Errorf("qurilma javob bermay qo'ydi, sync to'xtatildi (%d ta ko'rib chiqilgandan keyin): %w", done, err)
}

// restDevice — qurilmaga dam berib, qaytadan ulanishga urinadi.
//
// `true` — qurilma qaytdi, davom etsa bo'ladi.
func (s *Service) restDevice(ctx context.Context, client *dahua.Client, deviceID int64, unreachable *int) bool {
	for try := range deviceRestTry {
		log.Printf("qurilma %d javob bermayapti — %v dam berib ko'ramiz (%d/%d)",
			deviceID, deviceRest, try+1, deviceRestTry)

		// Ochiq turgan ulanishlarni tashlaymiz: qurilma tomonda ular
		// allaqachon yopilgan bo'lishi mumkin.
		client.Close()

		select {
		case <-time.After(deviceRest):
		case <-ctx.Done():
			return false
		}

		if err := client.Alive(ctx); err == nil {
			log.Printf("qurilma %d qaytdi — sync davom etmoqda", deviceID)
			*unreachable = 0
			return true
		}
	}
	return false
}

// removeUser — qurilmadagi yozuvni o'chiradi.
//
// RPC2 (90 ms) sinaladi, u ishlamasa eski CGI yo'li (~700 ms va uchta
// yangi TCP ulanish) zaxira sifatida qoladi.
func (s *Service) removeUser(ctx context.Context, client *dahua.Client,
	ensureObject func() (int, error), recNo int) error {

	object, err := ensureObject()
	if err == nil {
		err = client.RemoveUserByRecNo(ctx, object, recNo)
		if err == nil || dahua.Unreachable(err) {
			return err
		}
	}
	return client.RemoveUser(ctx, recNo)
}

// ensureUser — odam qurilmada bo'lmasa yozadi va RecNo qaytaradi.
//
// ⚠️ Odam allaqachon bo'lsa QAYTA QO'SHILMAYDI, aks holda bir xil UserID
// ikki marta tushadi.
//
// Yangi yozilgan RecNo DARHOL bazaga saqlanadi: sync o'rtada uzilsa,
// keyingi sikl o'sha odamni qaytadan qo'shib, qurilmada dublikat
// yaratardi.
func (s *Service) ensureUser(ctx context.Context, client *dahua.Client,
	ensureObject func() (int, error), deviceID int64, p store.Person) (int, error) {

	object, err := ensureObject()
	if err != nil {
		return 0, err
	}

	validFrom, validTo := "", ""
	if p.ValidFrom != nil {
		validFrom = p.ValidFrom.Format("2006-01-02") + " 00:00:00"
	}
	if p.ValidTo != nil {
		validTo = p.ValidTo.Format("2006-01-02") + " 23:59:59"
	}

	record := dahua.CardRecord(p.UserID, p.FullName, validFrom, validTo, 0, 0)
	if p.DeviceRecNo != nil && *p.DeviceRecNo > 0 {
		if p.DeviceUserHash == p.DesiredUserHash {
			return *p.DeviceRecNo, nil
		}

		err := client.UpdateUser(ctx, object, *p.DeviceRecNo, record)
		switch {
		case err == nil:
			if err := s.saveSyncState(ctx, deviceID, p.ID, "pending", p.DeviceRecNo,
				p.DeviceFaceSynced, p.DesiredUserHash, nil); err != nil {
				return 0, fmt.Errorf("karta yangilandi, holat bazaga yozilmadi: %w", err)
			}
			s.update(deviceID, func(pr *Progress) { pr.Users++ })
			return *p.DeviceRecNo, nil

		case dahua.RecordMissing(err):
			// Saqlangan RecNo eskirgan — pastda karta YANGIDAN yoziladi.
			log.Printf("qurilma %d: %s uchun RecNo %d yo'q — karta qaytadan yoziladi",
				deviceID, p.UserID, *p.DeviceRecNo)

		default:
			return 0, err
		}
	}

	recNo, err := client.InsertUser(ctx, object, record)
	if err != nil {
		return 0, err
	}

	if err := s.saveSyncState(ctx, deviceID, p.ID, "pending", &recNo, false,
		p.DesiredUserHash, nil); err != nil {
		// DB holati yo'qolsa keyingi sync shu odamni yana insert qilib dublikat
		// yaratadi. Eng xavfsiz kompensatsiya — hozirgina yozilgan kartani qaytarish.
		if removeErr := client.RemoveUserByRecNo(ctx, object, recNo); removeErr != nil {
			return 0, fmt.Errorf("karta yozildi, holat bazaga yozilmadi (%v), kompensatsion o'chirish ham bajarilmadi: %w", err, removeErr)
		}
		return 0, fmt.Errorf("karta yozildi, holat bazaga yozilmadi: %w", err)
	}
	s.update(deviceID, func(pr *Progress) { pr.Users++ })

	return recNo, nil
}

// saveSyncState vaqtinchalik DB uzilishida terminalda bajarilgan amalning
// holatini yo'qotmaslik uchun qisqa qayta urinish qiladi.
func (s *Service) saveSyncState(ctx context.Context, deviceID, personID int64,
	state string, recNo *int, faceSynced bool, userHash string, errMsg *string) error {
	var err error
	for attempt := range 3 {
		err = s.store.SaveSyncState(ctx, deviceID, personID, state, recNo, faceSynced, userHash, errMsg)
		if err == nil {
			return nil
		}
		if attempt < 2 {
			select {
			case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return err
}

// readPhoto — terminalga ketadigan rasmni diskdan o'qiydi.
//
// Rasm yo'li bazada `photos/<user_id>.jpg` ko'rinishida saqlanadi.
func (s *Service) readPhoto(p store.Person) ([]byte, error) {
	if p.PhotoPath == nil {
		return nil, fmt.Errorf("rasm yo'q")
	}

	jpeg, err := os.ReadFile(filepath.Join(s.photoDir, filepath.Base(*p.PhotoPath)))
	if err != nil {
		return nil, fmt.Errorf("rasm o'qilmadi: %w", err)
	}
	return jpeg, nil
}

// writeFaces — to'dadagi yuzlarni qurilmaga yozadi va holatni saqlaydi.
//
// Qaytgan xato — faqat QURILMA darajasidagi nosozlik (sync to'xtashi
// kerak). Bitta odamning xatosi qaytarilmaydi: u `device_person_sync` ga
// yoziladi va sikl davom etadi.
func (s *Service) writeFaces(ctx context.Context, client *dahua.Client,
	deviceID int64, jobs []faceJob) error {

	// ⚠️ Yangi va almashtiriladigan yuzlar ARALASHMAYDI: `insertMulti`
	// mavjud yuz ustiga yozmaydi, `updateMulti` esa yo'g'ini yozadi —
	// aralash to'da butunlay rad etilardi.
	fresh, replace := splitFaces(jobs)

	failed := map[int64]error{}
	for _, group := range []struct {
		jobs   []faceJob
		update bool
	}{{fresh, false}, {replace, true}} {
		if len(group.jobs) == 0 {
			continue
		}

		err := s.writeFaceGroup(ctx, client, group.jobs, group.update)
		if err == nil {
			continue
		}
		if dahua.Unreachable(err) {
			return err
		}

		// To'da yiqildi — bittalab urinamiz: bitta yaroqsiz rasm
		// qolganlarini olib ketmasin.
		for _, job := range group.jobs {
			one := []faceJob{job}
			if err := s.writeFaceGroup(ctx, client, one, job.replace); err != nil {
				if dahua.Unreachable(err) {
					return err
				}
				failed[job.person.ID] = err
			}
		}
	}

	for _, job := range jobs {
		var recArg *int
		if job.recNo > 0 {
			recArg = &job.recNo
		}

		if err, bad := failed[job.person.ID]; bad {
			msg := err.Error()
			_ = s.saveSyncState(ctx, deviceID, job.person.ID, "failed", recArg, false,
				job.person.DesiredUserHash, &msg)
			s.update(deviceID, func(p *Progress) { p.Done++; p.Failed++ })
			continue
		}

		if err := s.saveSyncState(ctx, deviceID, job.person.ID, "synced", recArg, true,
			job.person.DesiredUserHash, nil); err != nil {
			msg := "yuz yozildi, holat bazaga yozilmadi: " + err.Error()
			s.update(deviceID, func(p *Progress) { p.Done++; p.Failed++; p.Error = msg })
			continue
		}
		s.update(deviceID, func(p *Progress) { p.Done++; p.Faces++ })
	}
	return nil
}

// writeFaceGroup — bir xil turdagi yuzlarni bitta so'rovda yozadi.
//
// ⚠️ Qurilma yozish SUR'ATINI cheklaydi ("MAX INSERT RATE EXCEEDED").
// Bu rasmning aybi emas — biroz kutib qayta urinilsa o'tadi.
func (s *Service) writeFaceGroup(ctx context.Context, client *dahua.Client,
	jobs []faceJob, update bool) error {

	uploads := make([]dahua.FaceUpload, 0, len(jobs))
	for _, job := range jobs {
		uploads = append(uploads, dahua.FaceUpload{UserID: job.person.UserID, JPEG: job.jpeg})
	}

	write := client.InsertFaces
	if update {
		write = client.UpdateFaces
	}

	var err error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-time.After(rateLimitPause):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err = write(ctx, uploads)
		if err == nil || !dahua.RateLimited(err) {
			break
		}
	}

	// Bitta yuz bo'lsa, ikkinchi yo'lni ham sinab ko'ramiz: bazadagi
	// "qurilmada yuzi bor/yo'q" holati eskirgan bo'lishi mumkin.
	if err != nil && len(uploads) == 1 && !dahua.Unreachable(err) && !dahua.RateLimited(err) {
		other := client.UpdateFaces
		if update {
			other = client.InsertFaces
		}
		if other(ctx, uploads) == nil {
			return nil
		}
	}
	return err
}

func splitFaces(jobs []faceJob) (fresh, replace []faceJob) {
	for _, job := range jobs {
		if job.replace {
			replace = append(replace, job)
			continue
		}
		fresh = append(fresh, job)
	}
	return fresh, replace
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
	release, err := s.BeginDeviceOperation(deviceID, "clean_faces")
	if err != nil {
		return err
	}
	s.mu.Lock()
	if p, ok := s.progress[deviceID]; ok && p.Running {
		s.mu.Unlock()
		release()
		return fmt.Errorf("bu qurilmada amal allaqachon ketyapti")
	}
	s.progress[deviceID] = &Progress{DeviceID: deviceID, Operation: "clean_faces", Running: true, StartedAt: time.Now()}
	s.mu.Unlock()

	go s.cleanFaces(deviceID, release)
	return nil
}

func (s *Service) cleanFaces(deviceID int64, release func()) {
	defer release()
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

	// ⚠️ Yuz RPC2 orqali o'chiriladi (70 ms), CGI orqali emas (~700 ms va
	// uchta yangi TCP ulanish). Minglab yetim yuz uchun bu farq qurilmani
	// tirik qoldiradi.
	for _, orphan := range orphans {
		if ctx.Err() != nil {
			finish(ctx.Err())
			return
		}

		err := client.RemoveFaces(ctx, []string{orphan.UserID})

		if err == nil {
			consecutiveErrors = 0
			_ = s.store.MarkFaceRemoved(ctx, orphan.SyncID)
			s.update(deviceID, func(p *Progress) { p.Done++; p.Faces++ })
			continue
		}

		consecutiveErrors++
		s.update(deviceID, func(p *Progress) { p.Done++; p.Failed++ })

		// Qurilma bo'g'ilib qolgan yoki sur'at chegarasiga urilgan
		// bo'lishi mumkin — kutamiz.
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
	release, err := s.BeginDeviceOperation(deviceID, "wipe")
	if err != nil {
		return err
	}
	defer release()
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
