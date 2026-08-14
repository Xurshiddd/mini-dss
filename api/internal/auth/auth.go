// Package auth — login va JWT.
//
// Hozircha bitta admin foydalanuvchi (env orqali: ADMIN_USER/ADMIN_PASSWORD).
// Ko'p foydalanuvchi kerak bo'lganda `users` jadvali qo'shiladi — token
// tekshiruvi o'zgarmaydi.
package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrUnauthorized = errors.New("autentifikatsiya talab qilinadi")

type Service struct {
	secret   []byte
	ttl      time.Duration
	user     string
	password string
	aead     cipher.AEAD

	// Bekor qilingan tokenlar (logout). Token'ning SHA-256 hash'i saqlanadi
	// (ochiq token emas), qiymat — muddati (o'shandan keyin tozalanadi).
	//
	// ⚠️ Xotirada: server qayta ishga tushsa tozalanadi. Bitta admin uchun
	// bu yetarli — logout darhol ishlaydi, restart esa baribir sessiyani
	// uzadi. Ko'p tugunli o'rnatishda Redis kerak bo'lardi.
	revoked   map[string]time.Time
	revokedMu sync.Mutex
}

func New(secret []byte, ttl time.Duration, user, password, encryptionKey string) (*Service, error) {
	// Kalitni SHA-256 orqali 32 baytga keltiramiz — istalgan uzunlikdagi
	// satr berilishi mumkin.
	sum := sha256.Sum256([]byte(encryptionKey))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	s := &Service{
		secret: secret, ttl: ttl, user: user, password: password, aead: aead,
		revoked: map[string]time.Time{},
	}
	go s.cleanupRevoked()
	return s, nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Revoke — token'ni bekor qiladi (logout). Muddati o'tguncha qora ro'yxatda.
func (s *Service) Revoke(token string) {
	s.revokedMu.Lock()
	s.revoked[tokenHash(token)] = time.Now().Add(s.ttl)
	s.revokedMu.Unlock()
}

func (s *Service) isRevoked(token string) bool {
	s.revokedMu.Lock()
	defer s.revokedMu.Unlock()
	_, ok := s.revoked[tokenHash(token)]
	return ok
}

// cleanupRevoked — muddati o'tgan yozuvlarni davriy tozalaydi.
func (s *Service) cleanupRevoked() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.revokedMu.Lock()
		for h, exp := range s.revoked {
			if now.After(exp) {
				delete(s.revoked, h)
			}
		}
		s.revokedMu.Unlock()
	}
}

// Login — foydalanuvchi nomi va parolni tekshiradi, token qaytaradi.
func (s *Service) Login(user, password string) (string, time.Time, error) {
	// ⚠️ Doimiy vaqtli solishtirish — parolni belgima-belgi taxmin qilishning
	// oldini oladi.
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(s.user)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(s.password)) == 1

	if !userOK || !passOK {
		return "", time.Time{}, ErrUnauthorized
	}

	expires := time.Now().Add(s.ttl)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user,
		"exp": expires.Unix(),
		"iat": time.Now().Unix(),
	})

	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expires, nil
}

func (s *Service) parse(token string) error {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("kutilmagan imzo usuli: %v", t.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil || !parsed.Valid {
		return ErrUnauthorized
	}
	return nil
}

// Middleware — himoyalangan marshrutlar uchun.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ⚠️ Token FAQAT Authorization sarlavhasidan olinadi. Ilgari rasm
		// endpointi uchun `?token=` query'dan ham qabul qilinardi, lekin
		// token URL'da jurnal, brauzer tarixi va Referer'ga tushardi. Endi
		// web rasmni blob sifatida sarlavha bilan oladi (AuthImage.vue).
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		token = strings.TrimSpace(token)

		if !ok || s.parse(token) != nil || s.isRevoked(token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"autentifikatsiya talab qilinadi"}`))
			return
		}

		// Token'ni kontekstga qo'yamiz — logout handler'i uni bekor qiladi.
		next.ServeHTTP(w, r.WithContext(withToken(r.Context(), token)))
	})
}

type ctxKey int

const tokenKey ctxKey = 0

func withToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey, token)
}

// TokenFromContext — joriy so'rov token'i (Middleware qo'ygan).
func TokenFromContext(ctx context.Context) string {
	t, _ := ctx.Value(tokenKey).(string)
	return t
}

// ------------------------------------------------- qurilma parolini shifrlash

// Encrypt — qurilma parolini bazaga yozishdan oldin shifrlaydi (AES-256-GCM).
func (s *Service) Encrypt(plain string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *Service) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(raw) < s.aead.NonceSize() {
		return "", errors.New("shifrlangan qiymat juda qisqa")
	}

	nonce, body := raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():]
	plain, err := s.aead.Open(nil, nonce, body, nil)
	if err != nil {
		// ⚠️ Laravel bilan yozilgan eski parollar bu formatda EMAS —
		// ular qayta kiritilishi kerak.
		return "", fmt.Errorf("parol ochilmadi (ENCRYPTION_KEY o'zgarganmi yoki eski Laravel formatimi?): %w", err)
	}
	return string(plain), nil
}
