# Mini-DSS

Dahua yuz tanish terminallari uchun o'z boshqaruv tizimimiz. Dahua'ning rasmiy
DSS dasturi litsenziyasiz cheklangan foydalanuvchi bilan ishlaydi, terminalning
o'zi esa 50 000+ foydalanuvchini saqlaydi va CGI/RPC2 API'si mavjud.

**Hozirgi scope:** faqat foydalanuvchi ma'lumotlari — manba → baza → UI (rasm
tahrirlash) → terminallarga sync. Kirish/chiqish loglari va davomat keyinroq.

## Ishga tushirish

```bash
cp .env.example .env
# ENCRYPTION_KEY va JWT_SECRET ni to'ldiring:  openssl rand -hex 32
docker compose up -d --build
```

⚠️ **Bo'sh bazada migratsiyalar avtomatik qo'llanmaydi** — `api` birinchi
marta "relation ... does not exist" xatosi bilan ishga tushadi. Faqat yangi
(bo'sh) `postgres` volume'da, bir marta kerak:

```bash
for f in api/migrations/*.sql; do
  docker compose exec -T postgres psql -U minidss -d minidss -v ON_ERROR_STOP=1 < "$f"
done
docker compose restart api
```

→ **https://localhost:6443** · `admin` / `admin123`

⚠️ **Faqat HTTPS.** `http://localhost:6080` ochilsa Caddy `https://localhost/`
ga (portsiz) qayta yo'naltiradi — bu docker-compose'da chiqarilmagan (faqat
`6443:443` bor), shuning uchun brauzer ulana olmaydi. To'g'ridan-to'g'ri
`https://localhost:6443` oching. Sertifikat ichki CA'dan (Let's Encrypt
emas) — brauzer ogohlantiradi, "Advanced → Proceed" bosing.

## Tuzilma

```
mini dss/
├── api/          Go — REST API, Dahua klienti, sync engine
├── web/          Nuxt 4 — admin panel (SPA)
├── face-api/     Python + InsightFace — foto validatsiya
├── device-probe/ TASK-01 discovery natijalari va skriptlari
└── ARCHITECTURE.md
```

| Servis | Port |
|---|---|
| Nuxt admin panel | **6080** |
| Go API | 6081 |
| face-api | 6001 |
| PostgreSQL | 6432 |

⚠️ **Port 6000 ishlatilmaydi** — Chrome uni bloklaydi (`ERR_UNSAFE_PORT`,
X11 uchun ajratilgan). Boshqa bloklangan portlar: 6665–6669, 6697, 10080.

## Terminal qo'shish

IP va parol **UI orqali** kiritiladi (`.env` da emas) va bazada AES-256-GCM
bilan shifrlangan holda saqlanadi.

1. Qurilmalar → Qurilma qo'shish
2. IP, foydalanuvchi, parol, yo'nalish (kirish/chiqish)
3. «Tekshirish» — model/firmware/seriya avtomatik to'ladi
4. «Sync» — rasmi tekshiruvdan o'tgan odamlar yoziladi

⚠️ **Parol bir marta sinaladi.** Dahua bir necha muvaffaqiyatsiz urinishdan
keyin admin akkauntni vaqtincha bloklaydi. `401` kelsa kod qayta urinmaydi.

## Terminal bilan ishlashda bilib qo'yish kerak

Discovery va amaliy sinovlarda topilgan, hujjatda yozilmagan narsalar.
To'liqroq: [`device-probe/API-MATRIX.md`](device-probe/API-MATRIX.md)

| Narsa | Tafsilot |
|---|---|
| **`CardNo` majburiy** | Usiz `insert` → `400`, garchi mavjud yozuvlarda qiymati bo'sh bo'lsa ham |
| **RPC2 13× tez** | 30 ms/yozuv vs CGI 565 ms. Sabab: Digest har so'rovda 2 marta boradi, RPC2 sessiya bilan bir marta |
| **Ommaviy yozish YO'Q** | `RecordUpdater.import` ham, CGI `insertMulti` ham qo'llab-quvvatlanmaydi |
| **Ulanish qayta ishlatilsin** | Har so'rovga yangi TCP ochilsa qurilma ~100 so'rovdan keyin javob bermay qo'yadi |
| **Bir vaqtda bitta jarayon** | Qurilma juda kam ulanish qabul qiladi — parallel sikllar "connection refused" oladi |
| **RPC2 sessiyasi tugaydi** | "Invalid session" — avtomatik qayta login kerak. Obyekt handle'lari ham sessiyaga bog'liq |
| **Yuz alohida o'chiriladi** | `recordUpdater` yuzga tegmaydi; `FaceInfoManager.cgi?action=clear` esa umuman yo'q |
| **`user_id` — satr** | Qurilmada `AB5019365` kabi pasport formatidagi ID'lar bor. `BIGINT` qilinmaydi |
| **Vaqt filtri ishlamaydi** | `condition.StartTime` jimgina e'tiborga olinmaydi — xato ham, ogohlantirish ham yo'q |
| **Yo'nalish qurilmadan** | Yozuvdagi `Type` har doim `Entry` qaytaradi, unga tayanib bo'lmaydi |
| **Vaqt zonasi** | Qurilma vaqti local (UTC+5), loglardagi `CreateTime` esa epoch UTC |

## Ma'lumot manbasi

Hozir `elms.ttysi.uz` PostgreSQL'idan read-only o'qiladi (HEMIS sync u yerda
allaqachon ishlaydi). Keyinchalik to'g'ridan-to'g'ri HEMIS'ga o'tkaziladi.

⛔ **Hal qilinmagan:** elms xodimlarining `employee_id_number` (10 raqam)
terminaldagi ID'lar bilan **0% mos keladi** — ular terminalga boshqa
identifikator bilan kiritilgan. 460 tasi ism bo'yicha birlashtirildi
(`legacy_user_id` da eski ID saqlanadi), qolgani ochiq masala.

## Hali qilinmagan

- Yetim yuz shablonlari — foydalanuvchilar o'chirilgan, yuzlari qurilmada qolgan
- Qolgan 23 terminal (jami 24: 12 kirish + 12 chiqish)
- Kirish/chiqish loglari va davomat hisobi
