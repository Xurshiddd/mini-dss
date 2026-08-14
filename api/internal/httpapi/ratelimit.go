package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// clientIP — so'rov manbasining IP'si (portsiz).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimiter — kalit (masalan IP) bo'yicha urinishlarni cheklaydi.
//
// Oddiy sirpanuvchi oyna: har kalit uchun oxirgi urinishlar vaqti saqlanadi,
// `window` dan eskisi tashlanadi. Login brute-force'iga qarshi yetarli —
// katta hajm uchun Redis kerak bo'lardi, lekin bu bitta admin hisobi.
type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		attempts: map[string][]time.Time{},
		limit:    limit,
		window:   window,
	}
	// Eski yozuvlarni davriy tozalaymiz — aks holda map cheksiz o'sadi.
	go rl.cleanupLoop()
	return rl
}

// allow — urinishni qayd etadi va ruxsat berilganini qaytaradi.
//
// Chegaradan oshsa `false` — urinish baribir qayd etilmaydi, aks holda
// hujumchi doimiy so'rov bilan oynani cheksiz uzaytirardi.
func (rl *rateLimiter) allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	recent := rl.attempts[key][:0]
	for _, t := range rl.attempts[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= rl.limit {
		rl.attempts[key] = recent
		return false
	}

	rl.attempts[key] = append(recent, now)
	return true
}

// reset — kalit bo'yicha hisobni tozalaydi (muvaffaqiyatli login'dan keyin).
//
// ⚠️ Muvaffaqiyatli urinish oynani tozalaydi, aks holda to'g'ri parol
// kiritgan foydalanuvchi ham 5 tadan keyin bloklanardi. Brute-force esa
// baribir muvaffaqiyatsiz bo'lgani uchun tozalanmaydi.
func (rl *rateLimiter) reset(key string) {
	rl.mu.Lock()
	delete(rl.attempts, key)
	rl.mu.Unlock()
}

func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-rl.window)
		rl.mu.Lock()
		for key, times := range rl.attempts {
			kept := times[:0]
			for _, t := range times {
				if t.After(cutoff) {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(rl.attempts, key)
			} else {
				rl.attempts[key] = kept
			}
		}
		rl.mu.Unlock()
	}
}
