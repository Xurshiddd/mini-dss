package syncsvc

import (
	"context"
	"log"
	"sync"
	"time"

	"minidss/api/internal/dahua"
	"minidss/api/internal/store"
)

// listener — bitta qurilmaning jonli hodisa oqimi.
type listener struct {
	cancel context.CancelFunc
	done   chan struct{}

	// Bir odam bir necha soniya ichida bir necha marta hodisa hosil qiladi
	// (o'lchangan: 21 soniyada 4 marta). Oxirgi ko'rilgan vaqt shu yerda.
	mu   sync.Mutex
	seen map[string]time.Time
}

// debounce — shu muddat ichidagi takroriy hodisalar tashlab yuboriladi.
const debounce = 15 * time.Second

// StartListeners — barcha faol qurilmalar uchun jonli oqimni ochadi.
// Bloklaydi, `go` bilan chaqiriladi.
func (s *Service) StartListeners(ctx context.Context) {
	// Qurilmalar ro'yxati o'zgarishi mumkin (yangisi qo'shiladi, eskisi
	// o'chiriladi), shuning uchun davriy solishtirib turamiz.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	s.syncListeners(ctx)

	for {
		select {
		case <-ctx.Done():
			s.stopAll()
			return
		case <-ticker.C:
			s.syncListeners(ctx)
		}
	}
}

// syncListeners — ro'yxatni bazadagi faol qurilmalarga moslaydi.
func (s *Service) syncListeners(ctx context.Context) {
	devices, err := s.store.ActiveDevices(ctx)
	if err != nil {
		log.Printf("jonli oqim: qurilmalar ro'yxati o'qilmadi: %v", err)
		return
	}

	active := map[int64]bool{}
	for _, dev := range devices {
		active[dev.ID] = true

		s.listenersMu.Lock()
		_, running := s.listeners[dev.ID]
		s.listenersMu.Unlock()

		if !running {
			s.startListener(ctx, dev.ID)
		}
	}

	// Endi faol bo'lmagan qurilmalarning oqimini yopamiz.
	s.listenersMu.Lock()
	var stale []int64
	for id := range s.listeners {
		if !active[id] {
			stale = append(stale, id)
		}
	}
	s.listenersMu.Unlock()

	for _, id := range stale {
		s.stopListener(id)
	}
}

func (s *Service) startListener(parent context.Context, deviceID int64) {
	ctx, cancel := context.WithCancel(parent)
	l := &listener{cancel: cancel, done: make(chan struct{}), seen: map[string]time.Time{}}

	s.listenersMu.Lock()
	s.listeners[deviceID] = l
	s.listenersMu.Unlock()

	go func() {
		defer close(l.done)
		s.runListener(ctx, deviceID, l)
	}()
}

// runListener — oqimni ochadi va uzilganda qayta urinadi.
func (s *Service) runListener(ctx context.Context, deviceID int64, l *listener) {
	backoff := 5 * time.Second

	for ctx.Err() == nil {
		err := s.listenOnce(ctx, deviceID, l)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("jonli oqim (qurilma %d): %v — %s dan keyin qayta urinamiz",
				deviceID, err, backoff)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Qurilma o'chiq bo'lsa har 5 soniyada urinaverish foydasiz.
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

func (s *Service) listenOnce(ctx context.Context, deviceID int64, l *listener) error {
	dev, err := s.store.Device(ctx, deviceID)
	if err != nil {
		return err
	}

	password, err := s.decrypt(dev.Password)
	if err != nil {
		return err
	}

	// ⚠️ Timeout YO'Q: oqim soatlab ochiq turadi. Uzilishni `heartbeat=5`
	// va ctx boshqaradi.
	client := dahua.New(dahua.Device{
		IP: dev.IP, Port: dev.Port, Username: dev.Username, Password: password,
	}, 0)
	defer client.Close()

	log.Printf("jonli oqim ochildi: %s (%s)", dev.Name, dev.IP)

	return client.AttachEvents(ctx, func(ev dahua.StreamEvent) {
		s.handleEvent(ctx, dev, l, ev)
	})
}

// handleEvent — hodisani takrorlardan tozalab bazaga yozadi.
func (s *Service) handleEvent(
	ctx context.Context, dev store.Device, l *listener, ev dahua.StreamEvent,
) {
	// Tanilmagan urinishlar (`Status=0`, `UserID` bo'sh) davomat emas —
	// ularni jonli oqimdan saqlamaymiz. Log yig'ishda baribir tushadi.
	if ev.Status != 1 || ev.UserID == "" {
		return
	}

	l.mu.Lock()
	last, ok := l.seen[ev.UserID]
	if ok && ev.HappenedAt.Sub(last) < debounce {
		l.mu.Unlock()
		return
	}
	l.seen[ev.UserID] = ev.HappenedAt
	l.mu.Unlock()

	// ⚠️ Yo'nalish QURILMADAN.
	saved, err := s.store.SaveLiveEvent(ctx, dev.ID, dev.Direction, store.AccessEvent{
		UserID:     ev.UserID,
		HappenedAt: ev.HappenedAt,
		Status:     ev.Status,
		ErrorCode:  ev.ErrorCode,
		Method:     ev.Method,
	})
	if err != nil {
		log.Printf("jonli hodisa saqlanmadi (%s): %v", ev.UserID, err)
		return
	}
	if saved {
		// ⚠️ Ism jurnalga YOZILMAYDI — Docker loglari aylanma saqlanadi va
		// ularni ko'rish uchun alohida huquq yo'q. UserID yetarli.
		log.Printf("jonli hodisa: %s %s", dev.Direction, ev.UserID)
	}
}

// PauseDevice — qurilmaning oqimini yopadi va qayta ochuvchi funksiya beradi.
//
// ⛔ Qurilmaga HAR QANDAY murojaatdan oldin chaqirilishi SHART: oqim ochiq
// turganda qurilma port 80'ga yangi ulanish qabul qilmaydi va so'rov
// "connection refused" bilan yiqiladi.
//
//	resume := svc.PauseDevice(id)
//	defer resume()
func (s *Service) PauseDevice(deviceID int64) func() {
	s.listenersMu.Lock()
	_, running := s.listeners[deviceID]
	s.listenersMu.Unlock()

	if !running {
		return func() {}
	}

	s.stopListener(deviceID)

	return func() {
		// Fonda qayta ochiladi: chaqiruvchi kutib turmasin.
		go s.startListener(context.Background(), deviceID)
	}
}

func (s *Service) stopListener(deviceID int64) {
	s.listenersMu.Lock()
	l, ok := s.listeners[deviceID]
	delete(s.listeners, deviceID)
	s.listenersMu.Unlock()

	if !ok {
		return
	}

	l.cancel()

	// Oqim yopilishini kutamiz — aks holda keyingi so'rov hali ham
	// "connection refused" olishi mumkin.
	select {
	case <-l.done:
	case <-time.After(10 * time.Second):
		log.Printf("jonli oqim (qurilma %d) o'z vaqtida yopilmadi", deviceID)
	}
}

func (s *Service) stopAll() {
	s.listenersMu.Lock()
	ids := make([]int64, 0, len(s.listeners))
	for id := range s.listeners {
		ids = append(ids, id)
	}
	s.listenersMu.Unlock()

	for _, id := range ids {
		s.stopListener(id)
	}
}
