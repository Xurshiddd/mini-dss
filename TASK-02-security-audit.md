# TASK-02 — Xavfsizlik auditi

**Sana:** 2026-08-12
**Qamrov:** Docker konfiguratsiyasi, Go API, Nuxt web, Postgres, face-api, tarmoq
**Usul:** kod tahlili + jonli sinov (o'z tizimimizga qarshi)
**Asos:** OWASP Top 10 (2021), OWASP API Top 10, CIS Docker Benchmark

Har band OWASP kategoriyasiga bog'langan. `[x]` — tuzatilgan va jonli
tekshirilgan; `[ ]` — hali ochiq.

## ✅ 2026-08-12 da tuzatildi (kritik + shoshilinch)

| # | Nima | Tasdiq |
|---|---|---|
| 1 | Postgres `127.0.0.1` ga bog'landi + kuchli parol | tashqi IP dan `Connection refused` |
| 1b | Ilova NOSUPERUSER `minidss_app` roli bilan ulanadi | `COPY FROM PROGRAM` va `COPY FROM file` → permission denied |
| 2 | Kuchli admin parol | `admin123` → 401 |
| 2 | Login rate-limit (5/15daq, IP bo'yicha) | 5 urinishdan keyin 429; to'g'ri parol blokni tozalaydi |
| 2b | SSRF himoyasi — dial darajasida ichki IP bloklanadi | `169.254.169.254`, `127.0.0.1`, `10.x` → rejected |
| 2c | Body hajmi cheklovi (login 4KB, umumiy 16MB) | 100KB login body → rad etildi |

**Eslatma 2b bo'yicha:** dastlab "createPerson orqali SSRF" deb belgilangan
edi, lekin tekshiruvda `photo_url` createPerson'da SAQLANMAS ekan — u faqat
HEMIS sync orqali keladi. Ya'ni to'g'ridan-to'g'ri foydalanuvchi yuzasi yo'q,
lekin HEMIS ma'lumoti ishonchsiz bo'lishi mumkin, shuning uchun himoya
baribir qo'shildi (barcha rasm yuklash `safeHTTPClient` orqali).

**⚠️ Yangi parollar `.env` da.** Admin parol o'zgardi — eski `admin123`
endi ishlamaydi. Yangi qiymatlar `.env` faylida.

## ✅ 2026-08-12 (ikkinchi to'plam — yuqori/o'rta)

| # | Nima | Tasdiq |
|---|---|---|
| 3, 4 | face-api porti `127.0.0.1` ga yopildi | tashqi IP → ulanmadi; ichki validatsiya ishlaydi |
| 6 | Rasm tokeni URL'dan olib tashlandi, blob orqali (header) | `?token=` → 401; header → 200; `AuthImage.vue` |
| 17 | Xavfsizlik sarlavhalari (CSP, X-Frame, nosniff, Referrer, Permissions) | `curl -I` da hammasi bor |
| — | DB rol + GRANT init-skript (yangi muhitda avtomatik) | `api/initdb/01-app-role.sh` |

## ✅ 2026-08-12 (uchinchi to'plam — HTTPS)

| # | Nima | Tasdiq |
|---|---|---|
| 5 | Caddy reverse proxy + ichki TLS (self-signed CA) | `https://localhost` → 200, sertifikat "Caddy Local Authority" |
| 5 | HTTP → HTTPS avtomatik redirect | `http://localhost` → 308 |
| 3 | Web endi faqat Caddy ortida (host porti yo'q) | eski `:6080` → yopiq |
| 9 | Same-origin — CORS `*` endi amalda ishlatilmaydi | brauzer `/api/*` ni joriy origindan oladi |

**Yangi arxitektura:** yagona tashqi kirish — Caddy (443/80). Web va API
bitta origin ortida (`/api/*` → API, qolgani → web). Brauzer so'rovlari
nisbiy (`NUXT_PUBLIC_API_BASE=""`), shu sababli same-origin.

**Kirish:** `https://<server-ip>` yoki `https://localhost` (serverda).
Sertifikat ichki CA'dan — brauzer birinchi kirishda ogohlantiradi (ichki
tizim uchun kutilgan). Debug uchun API hali `127.0.0.1:6081` da.

**Portlar yakuniy holati:**
| Xizmat | Bind | Kirish |
|---|---|---|
| Caddy | `0.0.0.0:443`, `:80` | tashqi (yagona) |
| web | faqat compose tarmog'i | Caddy orqali |
| API | `127.0.0.1:6081` | Caddy `/api/*` + lokal debug |
| face-api | `127.0.0.1:6001` | ichki + lokal debug |
| Postgres | `127.0.0.1:6432` | ichki + lokal admin |
| NTP | `0.0.0.0:123/udp` | terminallar (chrony, amplification'ga chidamli) |

## ✅ 2026-08-12 (to'rtinchi to'plam — o'rta/past, "hammasi")

| # | Nima | Tasdiq |
|---|---|---|
| 7 | Konteynerlar non-root (api/face-api uid 10001, web node) | `id -u` → 10001/1000 |
| 8 | Resurs chegaralari (mem_limit har xizmat, face-api cpus 2) | compose'da |
| 10 | NTP image digest bilan qadaldi (`latest` emas) | `@sha256:...` |
| 11 | face-api bind mount olib tashlandi (kod image ichida) | `./face-api:/app` yo'q |
| 12 | Loglardan odam ismi olib tashlandi (UserID qoldi) | `listener.go` |
| 13 | Config `_FILE` qo'llab-quvvatlaydi (Docker/K8s secrets) | `envOrFile` |
| 18 | JWT server-side bekor qilish (logout) | logout → keyingi so'rov 401 |
| 19 | Caddy XFF hard-set → RealIP/rate-limit real client IP | rate-limit Caddy orqali ishlaydi |
| 15 | CVE skan (Trivy) o'tkazildi | quyida |

**Non-root nuance:** `storage-photos` volume eski root egaligida edi — API
(uid 10001) yoza olmadi (`permission denied`). Volume egaligi 10001 ga
o'zgartirildi (bir martalik; yangi o'rnatishda bo'sh volume Dockerfile
egaligini avtomatik oladi).

**13-band (secrets):** to'liq compose secrets `.env` UX'ini buzardi. Amaliy
yechim — config `KEY_FILE` variantini o'qiydi (fayldan), shu bilan Docker
secrets / K8s bilan ishlaydi va kalit `docker inspect`da ko'rinmaydi. `.env`
default sifatida qoladi (u allaqachon `.gitignore`da).

### CVE skani (Trivy, CRITICAL+HIGH)

| Image | Natija | Holat |
|---|---|---|
| api (gobinary) | 1 HIGH → **0** | `golang.org/x/text` 0.38→0.39 yangilandi ✅ |
| alpine base (api) | 0 | toza |
| web | 8 (7H, 1C) | node base + npm — `build --pull` bilan yangilanadi |
| face-api | 109+18 | python:3.12-slim (debian) OS paketlar |
| caddy | 5 HIGH | rasmiy image — yangilanish bilan |

**Base image CVE'lari** ilova kodida emas, asosiy image OS paketlarida.
Yopish yo'li — davriy `docker compose build --pull` (base'ning eng yangi
patch'i). face-api'ning ko'p CVE'si debian slim'da, LEKIN uning ekspluatatsiya
yuzasi tor: port `127.0.0.1` da, tashqi kirish yo'q, faqat ichki API foydalanadi.

⚠️ `build --pull` bu audit paytida registry TLS timeout berdi (vaqtinchalik
tarmoq) — base yangilash keyingi barqaror ulanishda takrorlansin.

---

## Asl topilma tavsiflari

> ⚠️ Quyidagilar auditning DASTLABKI ro'yxati — batafsil dalil va tahlil
> bilan. Aksari yuqoridagi to'plamlarda ✅ tuzatildi (7, 8, 10–13, 15, 17–19
> va kritiklar). Bu bo'lim tarixiy ma'lumot uchun saqlanadi.
>
> **Haqiqatan ochiq qolganlar:** base image CVE'lari (davriy `build --pull`
> bilan) va JWT TTL 12 soat (logout bekor qilish qo'shildi, TTL o'zi maqbul).

---

## ⛔ KRITIK

### 1. Postgres butun tarmoqqa ochiq va paroli standart

**Dalil** — compose tarmog'idan TASHQARIDA, host IP orqali ulandim:

```
$ psql "postgres://minidss:minidss@172.16.240.70:6432/minidss"
  minidss|8284          ← 8 284 odam yozuvi o'qildi
```

`docker-compose.yml`: `"${DB_PORT_HOST:-6432}:5432"` — port `0.0.0.0` ga
chiqarilgan. `.env`: `DB_PASSWORD=minidss` — bu `.env.example` dagi standart
qiymat, 7 belgi.

**Ta'sir:** 172.16.x tarmog'idagi istalgan kishi 8 284 odamning shaxsiy
ma'lumotini (F.I.Sh, HEMIS ID, bo'lim, lavozim, davomat tarixi) o'qiy oladi
va o'zgartira oladi. Terminal parollari shifrlangan, lekin qolgan hamma
narsa ochiq.

**OWASP:** A05 Security Misconfiguration + A02 Cryptographic Failures

**Yechim:**
- [ ] Portni faqat lokalga bog'lash: `127.0.0.1:6432:5432` (yoki umuman
      olib tashlash — API compose tarmog'i orqali boradi)
- [ ] Kuchli parol yaratish: `openssl rand -base64 32`
- [ ] Parol o'zgargach `docker compose up -d` bilan qayta ishga tushirish

---

### 1b. DB foydalanuvchisi SUPERUSER — Postgres orqali RCE va fayl o'qish

**Dalil** — 1-band bilan ZANJIR hosil qiladi (tashqi port + superuser):

```
SELECT current_user, usesuper  →  minidss | t     ← superuser

-- Konteynerda ixtiyoriy buyruq:
COPY t FROM PROGRAM 'id'  →  uid=70(postgres) gid=70(postgres)

-- Host fayl tizimini o'qish:
COPY f FROM '/etc/passwd'  →  root:x:0:0:root:/root:/bin/sh ...
```

**Ta'sir:** 1-band bilan birgalikda — tarmoqdagi istalgan kishi standart
parol bilan ulanib, `COPY FROM PROGRAM` orqali Postgres konteynerida
ixtiyoriy buyruq bajaradi (RCE). Bu shunchaki "ma'lumot o'qish" emas, to'liq
konteyner egallash.

**OWASP:** A04 Insecure Design + A01 Broken Access Control

**Yechim:**
- [ ] Ilova uchun ALOHIDA, superuser BO'LMAGAN rol yaratish:
      `CREATE ROLE minidss_app LOGIN PASSWORD '...' NOSUPERUSER;`
      va faqat kerakli jadvallarga `GRANT`
- [ ] `POSTGRES_USER` ni migratsiya/admin uchun qoldirish, ilova esa cheklangan
      rol bilan ulanishi
- [ ] (1-band bilan birga) tashqi portni yopish

---

### 2. Admin paroli standart va login urinishlari cheklanmagan

**Dalil:**

```
.env: ADMIN_PASSWORD=admin123        ← .env.example dagi standart, 8 belgi
10 ta noto'g'ri login urinishi → 401 401 401 ... (0 soniyada, bloklanmadi)
```

**Ta'sir:** `admin`/`admin123` — birinchi urinishdayoq topiladi. Admin
huquqi bilan: terminal parollarini o'zgartirish, hamma odamni o'chirish,
terminallarni tozalash (`wipe`), davomat ma'lumotini buzish.

**OWASP:** A07 Identification & Authentication Failures

**Yechim:**
- [ ] Kuchli admin paroli
- [ ] Login'ga urinish chegarasi (masalan IP bo'yicha 5 urinish / 15 daqiqa)
- [ ] Muvaffaqiyatsiz urinishlarni jurnalga yozish

---

### 2b. SSRF — foydalanuvchi ixtiyoriy URL berib, ichki tarmoqni skanerlaydi

**Dalil** — `photo_url` hech qanday tekshiruvsiz `http.Get` bilan olinadi:

```
POST /api/people {"photo_url":"http://169.254.169.254/latest/meta-data/"}
  → 201 Created

fetchOne(): http.NewRequestWithContext(ctx, GET, job.url, nil) — sxema
va manzil tekshirilmaydi (photos.go:162)
```

**Ta'sir:** Admin (yoki 2-band orqali kirgani) ixtiyoriy `photo_url` beradi,
sync uni server nomidan ochadi. Bu bilan: ichki xizmatlarni skanerlash
(`http://172.16.x`), bulut metama'lumot xizmati (`169.254.169.254`), yoki
`file://`-turdagi manbalarni sinash. Server ichki tarmoqqa "ko'prik" bo'ladi.

**OWASP:** A10 Server-Side Request Forgery

**Yechim:**
- [ ] `photo_url` faqat `https://` va faqat HEMIS domeniga ruxsat etilsin
      (allowlist)
- [ ] Lokal/xususiy IP oralig'iga (`127.0.0.0/8`, `169.254.0.0/16`,
      `10/8`, `172.16/12`, `192.168/16`) so'rovni bloklash
- [ ] Redirect'larni cheklash

---

### 2c. So'rov tanasi hajmi cheklanmagan (DoS)

**Dalil:** `login`, `createPerson` va boshqa JSON endpointlarda
`http.MaxBytesReader` yo'q. 5 MB body qabul qilindi:

```
POST /api/login  (5 MB body)  →  ulanish ochildi, o'qiy boshladi
grep MaxBytesReader → topilmadi (faqat photo va sync'da LimitReader bor)
```

**Ta'sir:** Ko'p parallel katta so'rov bilan API xotirasini to'ldirish.
Rasm yuklash (`ParseMultipartForm 12MB`) va sync (`LimitReader 12MB`)
himoyalangan, lekin JSON endpointlar emas.

**OWASP:** A05 Security Misconfiguration (resurs cheklovi)

**Yechim:**
- [ ] Har bir JSON handler'da `r.Body = http.MaxBytesReader(w, r.Body, 1<<20)`
- [ ] Yoki umumiy middleware sifatida qo'shish

---

## 🔴 YUQORI

### 3. Hamma xizmat tarmoqqa ochiq

**Dalil:**

```
172.16.240.70:6080 → 200   web (admin panel)
172.16.240.70:6081 → 404   API (javob beryapti)
172.16.240.70:6001 → 200   face-api /docs
172.16.240.70:6432 → psql  Postgres
```

**Yechim:**
- [ ] Faqat web (6080) tashqarida qolsin; API, face-api, Postgres
      `127.0.0.1` ga bog'lansin
- [ ] Yoki Windows firewall'da 6001/6432 ni yopish

---

### 4. face-api autentifikatsiyasiz

**Dalil:** `POST /validate` va `/docs` tokensiz ochiq, tarmoqdan ham.

**Ta'sir:** Har bir so'rov InsightFace modelini ishlatadi — CPU og'ir.
Bir necha parallel so'rov bilan xizmatni (va butun hostni) sekinlashtirish
mumkin. Bundan tashqari rasm yuklash — hujum yuzasi.

**Yechim:**
- [ ] Portni tashqariga chiqarmaslik (API compose tarmog'i orqali boradi)
- [ ] Ichki token yoki so'rov chegarasi qo'shish

---

### 5. HTTPS yo'q

Token, admin paroli, shaxsiy ma'lumot va yuz rasmlari tarmoq bo'ylab
**ochiq matnda** yuriladi. Bir tarmoqdagi kishi ularni tinglashi mumkin.

**Yechim:**
- [ ] Web va API oldiga TLS bilan reverse proxy (Caddy yoki nginx)
- [ ] Ichki sertifikat ham bo'ladi — asosiysi tarmoqda ochiq matn qolmasin

---

### 6. Rasm tokeni URL ichida

`useApi.photoUrl()` tokenni `?token=...` qilib yuboradi (`<img src>` sarlavha
qabul qilmagani uchun). Token server loglariga, brauzer tarixiga va
`Referer` sarlavhasiga tushadi.

**Yechim:**
- [ ] Rasm uchun qisqa muddatli alohida token (masalan 5 daqiqa, faqat shu
      odam rasmiga)
- [ ] Yoki rasmni blob orqali olish (`useApi.download` dagi kabi)

---

## 🟡 O'RTA

### 7. Hamma konteyner root bilan ishlaydi

**Dalil:** `docker inspect --format='{{.Config.User}}'` — hammasida bo'sh.

**Yechim:**
- [ ] `api` va `web` Dockerfile'iga `USER` qo'shish (nginx'siz ham bo'ladi,
      portlar 1024 dan katta)
- [ ] `read_only: true` va `cap_drop: [ALL]` ni sinab ko'rish

### 8. Resurs chegaralari yo'q

Bitta konteyner (masalan face-api) butun hostning xotirasini yeb qo'yishi
mumkin.

- [ ] `deploy.resources.limits` yoki `mem_limit` / `cpus` qo'shish

### 9. CORS hamma manbaga ochiq

`AllowedOrigins: ["*"]`. Token sarlavhada bo'lgani uchun to'g'ridan-to'g'ri
o'g'irlash bo'lmaydi, lekin har qanday sayt API bilan gaplasha oladi.

- [ ] Faqat o'z domenimizga cheklash

### 10. `cturra/ntp:latest` — versiya qadalmagan

`latest` istalgan paytda o'zgaradi. Bundan tashqari 123/udp `0.0.0.0` ga
ochiq — NTP kuchaytirish hujumida ishlatilishi mumkin.

- [ ] Aniq versiyaga qadash
- [ ] Portni faqat terminallar tarmog'iga cheklash

### 11. face-api'da host kodi bind mount qilingan

`./face-api:/app` — konteyner buzilsa host fayllariga yozadi.

- [ ] Ishlab chiqarishda bind mount o'rniga image ichiga COPY

### 12. Loglarda shaxsiy ma'lumot

```
jonli hodisa: exit 3312411057 MARIFOV XURSHID SOBIR O'G'LI
```

Docker loglari aylanma saqlanadi va ularni ko'rish uchun alohida huquq yo'q.

- [ ] Logda faqat ID qolsin, ism olib tashlansin

---

## 🟢 PAST

- [ ] **13.** Maxfiy kalitlar env orqali — `docker inspect` bilan ko'rinadi.
      Docker secrets yoki fayldan o'qish yaxshiroq.
- [ ] **14.** JWT muddati 12 soat va bekor qilish yo'li yo'q. Chiqish faqat
      localStorage'ni tozalaydi, token amal qilaveradi.
- [ ] **15.** Image'lar uchun CVE skaner yo'q (`trivy` yoki `docker scout`).
- [ ] **16.** Web'da xavfsizlik sarlavhalari yo'q: CSP, X-Frame-Options,
      X-Content-Type-Options.

---

## 🟡 O'RTA (davomi)

### 17. Xavfsizlik sarlavhalari umuman yo'q (jonli tasdiqlandi)

**Dalil:** `curl -I http://localhost:6080/` — CSP, X-Frame-Options,
X-Content-Type-Options, HSTS — hech biri yo'q.

**Ta'sir:** Clickjacking (iframe), MIME-sniffing, XSS himoyasi kuchsiz.

**OWASP:** A05 Security Misconfiguration

- [ ] Nuxt route rules yoki reverse proxy'da sarlavhalar qo'shish

### 18. Chiqish (logout) tokenni bekor qilmaydi

**Dalil:** JWT claim — `{"exp":..., "iat":..., "sub":"admin"}`, `jti` yo'q.
Chiqish faqat `localStorage.clear()` — token 12 soat amal qilaveradi.

**OWASP:** A07 Authentication Failures

- [ ] Token qora ro'yxati yoki qisqa TTL + refresh token

### 19. `RealIP` middleware proxy'siz — IP soxtalashtiriladi

**Dalil:** `middleware.RealIP` yoqilgan, lekin API to'g'ridan-to'g'ri ochiq
(reverse proxy yo'q). Hujumchi `X-Forwarded-For` bilan istalgan IP ko'rsatadi.

**Ta'sir:** Kelajakda IP bo'yicha rate-limit yoki audit qo'yilsa, u aldanadi.

**OWASP:** A05 Security Misconfiguration

- [ ] Proxy ortiga qo'yilmaguncha `RealIP` ni olib tashlash yoki ishonchli
      proxy ro'yxatini belgilash

---

## ✅ Tekshirildi — muammo topilmadi

| Nima sinaldi | OWASP | Natija |
|---|---|---|
| SQL in'ektsiya (`' OR 1=1--`, `DROP TABLE`, `UNION SELECT`) | A03 | Ta'sir yo'q — hamma so'rov parametrli (`$1`), `access_events` joyida (5 003 yozuv) |
| Yo'l bo'ylab chiqish (`../../etc/passwd/photo`) | A01 | 404 — ID butun songa parse qilinadi |
| IDOR (mavjud bo'lmagan person ID) | A01 | 404 |
| Buyruq in'ektsiyasi (`os/exec` ishlatilishi) | A03 | Kodda `os/exec` umuman yo'q |
| XSS (`v-html` ishlatilishi) | A03 | Kodda `v-html` umuman yo'q — Vue avto-escape qiladi |
| Tokensiz kirish (`/people`, `/devices`, `/summary`, `/reports`) | A01 | Hammasi 401 |
| Yolg'on token bilan rasm olish | A01 | 401 |
| JWT algoritm almashtirish (`alg=none`) | A02 | Himoyalangan — parserda HMAC tekshiriladi |
| JWT muddati (`exp`) tekshiriladimi | A07 | Ha — claim'da `exp` bor va tekshiriladi |
| Parol solishtirish | A02 | `subtle.ConstantTimeCompare` — vaqt hujumidan himoyalangan |
| Terminal parollarini saqlash | A02 | AES-256-GCM (AEAD) — to'g'ri |
| Rasm yuklashda hajm cheki | A05 | Bor — `ParseMultipartForm 12MB` + `header.Size > 10MB` rad |
| Sync rasm yuklashda hajm cheki | A05 | Bor — `LimitReader 12MB` |
| TRACE metodi | A05 | 401 (auth ortida) |
| `.env` git'ga tushganmi | A05 | Yo'q — `.gitignore` da |
| Docker soketi konteynerga ulanganmi | A05 | Yo'q — hech qaysi konteynerda `/var/run/docker.sock` yo'q |
| Panic stack tashqariga chiqadimi | A05 | Yo'q — `Recoverer` middleware ushlaydi |

---

## OWASP Top 10 (2021) qamrovi

| Kategoriya | Topilma | Holat |
|---|---|---|
| A01 Broken Access Control | 1b, RBAC yo'q (bitta rol) | ⚠️ qisman |
| A02 Cryptographic Failures | 1, 5 (HTTPS yo'q), 6 (URL token) | 🔴 ochiq |
| A03 Injection | — | ✅ toza (SQLi, cmd, XSS himoyalangan) |
| A04 Insecure Design | 1b (superuser), rate-limit yo'q | 🔴 ochiq |
| A05 Security Misconfiguration | 3, 7, 8, 10, 17, 19, 2c | 🔴 ochiq (ko'p) |
| A06 Vulnerable Components | 15 (skaner yo'q) | 🟡 tekshirilmagan |
| A07 Auth Failures | 2, 18 | 🔴 ochiq |
| A08 Data Integrity Failures | 13 (secret env'da) | 🟢 past |
| A09 Logging Failures | 12 (logda ism), audit yo'q | 🟡 o'rta |
| A10 SSRF | 2b | 🔴 ochiq |

**Xulosa:** Injection sinfi (A03) toza — bu ilova kodining eng kuchli tomoni.
Asosiy xavf A02/A04/A05/A07/A10 — deyarli hammasi **konfiguratsiya va
joylashtirishda**, ilova mantig'ida emas.

---

## Tavsiya etilgan tartib

1. **Bugun:** 1, 1b, 2 — bularsiz tizim tarmoqdan to'liq ochiq (RCE'gacha).
2. **Shu hafta:** 2b (SSRF), 2c (body DoS), 3-6 (portlar, face-api, HTTPS,
   rasm tokeni).
3. **Keyin:** o'rta va past bandlar.

> ⚠️ **Muhim nuans:** bu tizim **ichki tarmoqda** (172.16.x), internetdan
> to'g'ridan-to'g'ri ochiq emas. Shuning uchun real xavf — ichki tarmoqqa
> kira olgan kishi (xodim, buzilgan qurilma, mehmon Wi-Fi). Bu xavfni
> kamaytirmaydi, faqat internetga ochiq tizimga qaraganda "hujum yuzasi"
> torroq ekanini bildiradi.
