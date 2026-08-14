package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port        string
	DatabaseURL string

	// Qurilma parollarini shifrlash kaliti (32 bayt, hex yoki base64).
	// ⚠️ O'zgarsa saqlangan parollar o'qilmay qoladi.
	EncryptionKey string

	JWTSecret     []byte
	JWTTTL        time.Duration
	AdminUser     string
	AdminPassword string

	PhotoDir   string
	FaceAPI    string
	HemisBase  string
	HemisToken string
	Timezone   *time.Location

	// Rasm manbasi uchun ishonchli hostlar (vergul bilan).
	//
	// ⚠️ HEMIS rasm serveri ICHKI tarmoqda (hemis.ttyesi.uz -> 172.16.0.253).
	// SSRF himoyasi xususiy IP'larni bloklaydi, shuning uchun bu host aniq
	// ko'rsatilmasa HAMMA rasm "manba URL ochilmadi" bilan rad etiladi.
	// Bo'sh qoldirilsa — faqat ochiq IP'lardan rasm olinadi.
	PhotoAllowedHosts []string

	// Qurilmalar uchun NTP serveri. ⚠️ NOM emas, IP — ichki DNS tashqi
	// nomlarni o'ziga qaytaradi va sinxronizatsiya jimgina ishlamay qoladi.
	// Standart: 216.239.35.0 (time.google.com, stratum-1).
	NTPAddress string

	// Terminallardan hodisalarni JONLI oqim orqali olish.
	//
	// ⚠️ Oqim ochiq turganda qurilma port 80'ga yangi ulanish qabul
	// qilmaydi — shuning uchun har qanday yozish amalidan oldin oqim
	// yopiladi va keyin qayta ochiladi.
	LiveEvents bool
}

// env — muhit o'zgaruvchisi, `_FILE` variantini qo'llab-quvvatlaydi.
//
// ⚠️ Agar `KEY_FILE` berilgan bo'lsa, qiymat SHU FAYLDAN o'qiladi (Docker
// secrets / Kubernetes bilan mos). Bunda maxfiy kalit muhitda turmaydi va
// `docker inspect` bilan ko'rinmaydi — bu 13-band (secretlar) uchun.
func env(key, fallback string) string {
	if path := os.Getenv(key + "_FILE"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if v := strings.TrimSpace(string(data)); v != "" {
				return v
			}
		}
	}
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// splitHosts — vergul bilan ajratilgan host ro'yxatini bo'ladi.
// Bo'sh qismlar tashlanadi, host'lar kichik harfga keltiriladi.
func splitHosts(raw string) []string {
	var out []string

	for _, part := range strings.Split(raw, ",") {
		if h := strings.ToLower(strings.TrimSpace(part)); h != "" {
			out = append(out, h)
		}
	}

	return out
}

func Load() (Config, error) {
	tzName := env("MINIDSS_TIMEZONE", "Asia/Tashkent")
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		return Config{}, fmt.Errorf("vaqt zonasi yaroqsiz (%s): %w", tzName, err)
	}

	ttlMin, _ := strconv.Atoi(env("JWT_TTL_MINUTES", "720"))

	cfg := Config{
		Port: env("PORT", "8000"),
		DatabaseURL: env("DATABASE_URL", fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=disable",
			env("DB_USERNAME", "minidss"), env("DB_PASSWORD", "minidss"),
			env("DB_HOST", "postgres"), env("DB_PORT", "5432"),
			env("DB_DATABASE", "minidss"))),
		EncryptionKey: env("ENCRYPTION_KEY", ""),
		JWTSecret:     []byte(env("JWT_SECRET", "")),
		JWTTTL:        time.Duration(ttlMin) * time.Minute,
		AdminUser:     env("ADMIN_USER", "admin"),
		AdminPassword: env("ADMIN_PASSWORD", "admin123"),
		PhotoDir:      env("PHOTO_DIR", "/data/photos"),
		FaceAPI:       env("FACE_API_URL", "http://face-api:8000"),
		HemisBase:     env("HEMIS_BASE_URL", ""),
		HemisToken:    env("HEMIS_TOKEN", ""),

		PhotoAllowedHosts: splitHosts(env("PHOTO_ALLOWED_HOSTS", "")),
		Timezone:          tz,
		NTPAddress:        env("MINIDSS_NTP_ADDR", "216.239.35.0"),
		LiveEvents:        env("MINIDSS_LIVE_EVENTS", "true") != "false",
	}

	if len(cfg.JWTSecret) < 16 {
		return cfg, fmt.Errorf("JWT_SECRET kamida 16 belgi bo'lishi kerak")
	}
	if cfg.EncryptionKey == "" {
		return cfg, fmt.Errorf("ENCRYPTION_KEY berilmagan — qurilma parollari shu bilan shifrlanadi")
	}

	return cfg, nil
}
