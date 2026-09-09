# Mini-DSS — arxitektura qarorlari

**Sana:** 2026-08-11
**Asos:** `device-probe/DEVICE-REPORT.md` (TASK-01 discovery natijalari)
**Maqsad:** Dahua DSS qila oladigan ishlarni o'zimiz qilish + HEMIS integratsiya
+ to'liq UI. Litsenziya cheklovisiz, 9801+ foydalanuvchi.

> ## 🎯 Hozirgi scope — FAQAT FOYDALANUVCHI MA'LUMOTLARI
>
> Kirish/chiqish loglari, real-time hodisalar va davomat **hozircha kerak emas**.
>
> **Kiradi:** manba → baza → UI (rasm tahrirlash) → terminalga sync,
> va teskari yo'nalishda terminaldagi userlarni bazaga tortish.
> **Chiqadi (keyinroq):** event listener, `access_events`, davomat hisobi,
> log poll, `RecNo` kursori.
>
> Discovery natijalari (`DEVICE-REPORT.md`) baribir to'liq saqlanadi — hodisa
> qismi kerak bo'lganda tayyor turadi, qayta discovery kerak emas.

---

## 1. Qabul qilingan qarorlar

| # | Qaror | Sabab |
|---|---|---|
| 1 | **Hamma narsa alohida.** O'z PostgreSQL, o'z Redis, o'z docker-compose, o'z tarmog'i, o'z servislari. `elms.ttysi.uz` bilan hech qanday umumiy konteyner yo'q. | Kirish nazorati LMS relizlariga bog'lanib qolmasin. |
| 2 | **Ma'lumot manbasi adapter orqasida.** Hozir `ElmsDbSource` (elms PostgreSQL'dan read-only), keyinchalik `HemisApiSource`. | Manba o'zgarishi config o'zgarishi bo'lsin, qayta yozish emas. |
| 3 | **Foto validatsiya — alohida Python servis** (InsightFace). `proctor-face-service` qayta ishlatilmaydi. | Mustaqillik (1-qarorga mos). |
| 4 | **Backend: Laravel**, UI `localhost:6000`. | TASK-01 da kelishilgan. |
| 5 | **Event listener: alohida Python asyncio servisi.** | Uzoq muddatli multipart stream — PHP uchun noqulay. |
| 6 | **Terminal API: legacy CGI** (`recordFinder.cgi` / `recordUpdater.cgi`). | Discovery bilan tasdiqlangan. |
| 7 | **Yo'nalish (kirish/chiqish) qurilmadan aniqlanadi**, yozuvning `Type` maydonidan emas. | Barcha yozuvlarda `Type=Entry` chiqdi, `bidirect` sozlangan bo'lsa ham. |
| 8 | Hozircha **faqat `192.168.30.222`** ishlaydi. Qolgan 23 tasi hali yo'q. | Kod boshidanoq N-qurilmali bo'lsin, lekin test bittada. |

---

## 2. Konteynerlar (`docker-compose.yml`)

Barchasi `mini-dss` tarmog'ida, o'z port blokida (6xxx) — elms bilan to'qnashmaydi.

| Servis | Nima | Port |
|---|---|---|
| `app` | Laravel + UI (admin panel) | **6000** |
| `postgres` | PostgreSQL 16 — mini-DSS o'z bazasi | 6432 |
| `redis` | Redis — queue + cache + dedup | 6379 (ichki) |
| `horizon` | Laravel queue worker (sync ishlari) | — |
| `scheduler` | Laravel scheduler (davriy sync) | — |
| `face-api` | **Python + InsightFace** — foto validatsiya | 6001 |

Hozirgi scope'da `listener` konteyneri **yo'q** — hodisa oqimi kerak emas.

> ℹ️ Keyinroq hodisa oqimi qo'shilganda esda tutilsin: stream ochiq turganda
> terminal port 80'ga **yangi HTTP ulanish qabul qilmaydi** (discovery'da
> tasdiqlangan). Ya'ni listener va sync bir qurilmada bir vaqtda ishlay olmaydi.

---

## 3. Baza sxemasi (asosiy jadvallar)

```sql
-- Qurilmalar. HAMMASI UI ORQALI QO'SHILADI — .env da emas.
devices (
  id, name,
  ip, port,             -- UI da kiritiladi
  username,             -- UI da kiritiladi
  password,             -- UI da kiritiladi, `encrypted` cast bilan saqlanadi
  direction,            -- 'entry' | 'exit'  (yo'nalish shu yerda, yozuvda emas)
  location,
  model, firmware, serial,   -- "Ulanishni tekshirish" tugmasi avtomatik to'ldiradi
  is_active,
  last_seen_at,
  last_error,
  created_at, updated_at
)

-- Odamlar. Manbadan qat'i nazar bitta jadval (2-qaror).
people (
  id,
  external_id,          -- manbadagi ID (elms students.id / employees.id)
  source,               -- 'elms_student' | 'elms_employee' | 'hemis_*' | 'manual'
  user_id,              -- TERMINALGA KETADIGAN ID. VARCHAR(32), BIGINT EMAS!
  full_name,
  photo_path,           -- bizdagi asl rasm
  photo_hash,           -- o'zgarganini bilish uchun
  photo_status,         -- 'missing'|'pending'|'valid'|'rejected'
  photo_reject_reason,
  valid_from, valid_to,
  is_active,
  synced_at
)

-- Har bir odam × har bir qurilma uchun sync holati.
-- 9801 odam × 24 qurilma ≈ 235k qator.
device_person_sync (
  id, device_id, person_id,
  state,                -- 'pending'|'synced'|'failed'|'removed'
  device_recno,         -- terminaldagi RecNo (update/delete uchun kerak)
  face_synced,          -- yuz shabloni yuklandimi
  last_error,
  attempts,
  synced_at,
  UNIQUE (device_id, person_id)
)

-- Odamning rasmlari. `people.photo_path` — faqat HEMIS oqimidagi rasm;
-- qo'lda qo'yilgan va terminaldan olingani SHU YERDA yotadi.
--
-- ⚠️ Ilgari ikkalasi bitta `photo_path` / bitta `<user_id>.jpg` faylini
-- bo'lishardi va "Rasmlarni yuklash" bosqichi qo'lda qo'yilgan rasmni
-- jimgina bosib ketardi.
person_photos (
  id, person_id,
  source,               -- 'manual' | 'terminal' | 'hemis'
  file_name,            -- PHOTO_DIR ichidagi fayl (yo'lsiz)
  source_url,           -- HEMIS URL yoki `device://<id>/<UserID>`
  status,               -- 'pending'|'valid'|'rejected'
  reject_reason, metrics,
  device_id,            -- terminaldan olingan bo'lsa
  created_at
)
-- people.photo_override_id → person_photos.id
--   NULL   : terminalga HEMIS rasmi ketadi
--   to'la  : terminalga SHU rasm ketadi va HEMIS oqimi odamga tegmaydi

-- Terminaldan olingan yuzlar — QORALAMA. Hech kimga tegishli emas.
photo_drafts (
  id,
  user_id,              -- terminaldagi UserID (unikal kalit)
  full_name,            -- terminaldagi CardName
  device_id, file_name, -- fayl PHOTO_DIR/drafts/ ichida
  status, reject_reason, metrics,
  person_id, attached_at,   -- biriktirilgan bo'lsa
  created_at, updated_at
)

-- Sync tarixi / audit (kim nimani qachon yubordi, xato nima edi).
sync_runs (
  id, device_id, kind,  -- 'push_users' | 'pull_users' | 'push_faces'
                        -- | 'photo_fetch' | 'draft_pull'
  started_at, finished_at,
  total, ok_count, fail_count,
  status, error
)
```

> `access_events` va `attendance_days` — **hozirgi scope'da yo'q**. Hodisa/davomat
> kerak bo'lganda qo'shiladi; `DEVICE-REPORT.md` 7-bandida maydonlar tayyor.

### `user_id` — kritik detal

Terminalda **4 xil format** aralash (`DEVICE-REPORT.md` 5-band):
12 raqam (76%), `AB5019365` pasport (14%), 4 raqam (8%), 11 raqam (2%, xato).

**`VARCHAR(32)`, `BIGINT` EMAS.** `AB5019365` ni `BIGINT` ga solib bo'lmaydi —
sync butunlay yiqiladi.

elms'dagi `students.student_id_number` 12 raqamli — terminaldagi asosiy format
bilan **aynan mos**, transformatsiya kerak emas.

---

## 4. Ma'lumot oqimlari

### 4.1 Pastga: manba → mini-DSS bazasi

```
ElmsDbSource (hozir)  ─┐
                       ├─► SourceAdapter ─► people jadvali
HemisApiSource (keyin) ─┘
```

`PersonSource` interfeysi: `people(): Generator<SourcePerson>`.
`ElmsDbSource` — elms PostgreSQL'ga **read-only** ulanish (`students`, `employees`).
Almashtirish `config/minidss.php` dagi `source` qiymatida.

#### HEMIS talaba filtrlari (2026-09-04 da o'lchandi)

`GET /rest/v1/data/student-list` qabul qiladigan parametrlar (hammasi VA
bilan birlashadi): `_department`, `_specialty`, `_group`, `_curriculum`,
`_level`, `_semester`, `_education_form`, `_education_type`,
`_student_status`, `_gender`, `_citizenship`, `_province`, `_district`,
`search`, `passport_pin` + `passport_number`, `tutor_pin`,
`updated_at_from`, `updated_at_to`.

| Bilim | Tafsilot |
|---|---|
| ⚠️ `_payment_form` ishlamaydi | `11` ham, `12` ham 7120 ta (= jami) qaytardi — shuning uchun u so'rovga yuborilmaydi, javob ustida saralanadi |
| ⚠️ Turar joy filtri YO'Q | `_accommodation` (va `accommodation`, `_living_status`) jimgina e'tiborsiz qoladi. Maydonning o'zi javobda bor (`accommodation.code`), shuning uchun yotoqxona (`15`) mahalliy saralanadi: 1985 / 7120. Alohida `dormitory-list` endpointi ham yo'q (404) |
| ⚠️ Vergulli ro'yxat yo'q | `_education_form=11,13` → 0 ta. Xato emas, jimgina bo'sh javob |
| ⚠️ Standart holat = `11` | filtrsiz faqat "O'qimoqda" keladi (7120); `_student_status=-1` bersa hammasi (17120) |
| ⚠️ Harfli kod → HTTP 500 | `_group=abc` "Ichki server xatosi" beradi, 400 emas. Tekshiruv bizda: `hemis.StudentFilter.Validate` |
| ⚠️ Kurslar `h_course` da | `h_level` degan klassifikator YO'Q; `_level` esa `h_course` kodlarini (11…16) oladi |
| ⚠️ Viloyat va tuman bitta ro'yxatda | `h_soato`: viloyatda `_parent` o'z kodiga teng, tumanda viloyat kodi. Ajratishning boshqa belgisi yo'q |
| `pagination.totalCount` | filtr natijasi oldindan ma'lum — progress bar va "topiladi: N ta" shunga tayanadi |

Filtr ro'yxatlari (fakultet, yo'nalish, guruh, o'quv reja va klassifikatorlar)
`GET /api/hemis/filters` orqali beriladi (30 daqiqa keshlanadi),
`POST /api/hemis/student-count` esa tanlangan filtr bo'yicha sonni qaytaradi.
Sync `POST /api/sync/hemis` tanasida `{employees, students, filter}` oladi;
tana bo'sh bo'lsa — eskisidek, hammasi filtrsiz.

#### ⛔ Xodimlarni bog'lab bo'lmaydi — hal qilinishi kerak

2026-08-11 da o'lchandi (elms ↔ terminaldagi 9801 odam):

| Manba | Kalit | Format | Terminalda topildi |
|---|---|---|---|
| Talaba | `student_id_number` | 12 raqam | **205 / 264 = 78%** ✅ |
| Xodim | `employee_id_number` | **10 raqam** | **0 / 1109 = 0%** ⛔ |

Xodimlar terminalga **butunlay boshqa identifikator** bilan kiritilgan.
Terminalda `AB5019365` shaklidagi 620 ta pasport formatidagi yozuv bor —
ehtimol xodimlar shular. Lekin elms'da pasport maydoni **umuman yo'q**
(`employees` ustunlari: `external_id, full_name, short_name,
employee_id_number, image, staff_position, employee_type_name, active`).

Ism bo'yicha bog'lash ham **ishlamaydi**:

| | Ism bo'yicha moslik |
|---|---|
| Xodim | 459 / 1109 = 41% |
| Talaba | 72 / 264 = 27% |

Talaba ID bo'yicha 78%, ism bo'yicha esa atigi 27% — ya'ni terminaldagi
ismlar transliteratsiya, tartib va imlo jihatidan sezilarli farq qiladi.
**Ism sync kaliti bo'la olmaydi.**

Hozirgi holat: xodimlar `people` ga *yangi* yozuv sifatida tushdi, terminaldagi
mavjud yozuvlariga bog'lanmadi. Ya'ni sync qilinsa, xodimlar terminalda
**ikki marta** paydo bo'ladi.

**Variantlar (odam qarori kerak):**
1. HEMIS API'sida pasport/JSHSHIR maydoni bormi — bo'lsa, elms'ga qo'shib
   shu orqali bog'lanadi (eng to'g'ri yo'l)
2. Terminaldagi xodimlarni o'chirib, elms ID'lari bilan qaytadan yozish
3. UI'da qo'lda bog'lash oynasi (1109 ta yozuv — real emas)

### 4.2 Yuqoriga: mini-DSS → terminallar (asosiy ish)

```
people (photo_status=valid)
   │
   ├─► foto validatsiya (face-api:6001) ─► valid / rejected
   │
   └─► har bir active device uchun queue job:
         1. recordUpdater.cgi?action=insert&name=AccessControlCard   (user)
         2. FaceInfoManager.cgi?action=add                            (yuz, base64)
         3. device_person_sync.state = synced, device_recno saqlanadi
```

> ⛔ **2-qadam hali tasdiqlanmagan.** `FaceInfoManager.cgi?action=add` jonli
> qurilmada sinalmadi (yozuv taqiqlangan edi). RPC2 da `FaceInfoManager` bor,
> lekin CGI `add` ishlashi tekshirilishi kerak. **Bu birinchi bajariladigan ish.**

Hajm: 9801 × 24 ≈ 235k operatsiya. Queue majburiy (Horizon), qurilma bo'yicha
rate-limit bilan. RPC2 `RecordUpdater.import` (ommaviy import) topilgan — agar
ishlasa, bu yo'l ancha tez bo'ladi, Faza 1 da sinaladi.

### 4.3 Teskari: terminal → mini-DSS bazasi

Siz so'ragan "face id dagi ma'lumotni bazaga tortib olish":

| Nima | Holat |
|---|---|
| **Foydalanuvchilar** (9801 ta: `UserID`, ism, `RecNo`, amal muddati) | ✅ **Ishlaydi** — `doSeekFind` + `offset`. Hozirgi scope'ning asosiy qismi. |
| **Ro'yxatga olingan yuz shablonlari/fotolari** | ❓ **Noaniq** — pastdagi "Qoralama yig'ish" ga qarang. |
| Kirish/chiqish loglari, hodisa fotolari | ✅ ishlaydi, lekin **hozirgi scope'da kerak emas** |

Terminaldagi 9801 odamni bazaga tortib olish **hozir ham mumkin va birinchi
qadam bo'lishi kerak**: kim bor, kim manbada yo'q, qaysi ID formati xato.
Bu sync yo'nalishini tanlashdan oldin real holatni ko'rsatadi.

#### Qoralama yig'ish (`POST /api/sync/drafts`)

Eski DSS orqali kiritilgan odamlarning rasmi faqat terminalda qolgan —
HEMIS ular uchun yuz bermaydi. Yig'ish ularni `photo_drafts` ga oladi:

1. Terminal tanlanadi — berilmasa har bir faol qurilmada `CountUsers`
   sanaladi va **eng ko'p yozuvlisi** olinadi (eski DSS hamma terminalga
   bir xil yozmagan).
2. `doSeekFind` bilan `UserID` + `CardName` o'qiladi.
3. Har biriga `FaceInfoManager.cgi?action=find&UserID=…` yuboriladi;
   javobda base64 kelmasa, ichidagi fayl yo'li `RPC_Loadfile` orqali
   olinadi (u Digest'ni qabul qilmaydi — RPC2 sessiya cookie'si bilan).
4. Rasm face-api'da tekshiriladi va qoralama sifatida saqlanadi.

> ⚠️ **3-qadam bu firmware'da HALI TASDIQLANMAGAN** (`API-MATRIX.md` 8.2).
> `action=find` probe paytida `UserID`siz yuborilgani uchun `400` bergan
> bo'lishi mumkin. Jonli terminalda avval `GET /api/devices/{id}/face-probe?user_id=X`
> chaqirilsin — u qaysi action rasm qaytarganini ko'rsatadi.
>
> Yig'ish **15 ta ketma-ket muvaffaqiyatsiz urinishdan keyin to'xtaydi**:
> firmware yuzni bermayotgan bo'lsa 9 800 ta so'rov yuborishning ma'nosi
> yo'q va u qurilmani bo'g'adi.

Qoralama biriktirilgunicha **hech kimga ta'sir qilmaydi**. Biriktirish
`person_photos` ga NUSXA yozadi (qoralama o'chsa ham rasm joyida qoladi) va
`photo_override_id` ni o'sha nusxaga qaratadi.

Avtomatik biriktirish faqat **bir ma'noli** mosliklarda ishlaydi: `UserID`
(yoki `legacy_user_id`) bo'yicha bitta nomzod, yoki faqat ism bo'yicha
topilganda butun bazada bitta nomzod. Bunsiz bir xil ismli ikki odamdan
biriga begona yuz biriktirilib, u boshqa odamning ismi bilan eshikdan o'tib
ketardi.

---

## 5. Vaqt masalasi

- `CreateTime`/`UTC`/`RealUTC` — **Unix epoch, UTC**. Bazaga `timestamptz`.
- Qurilma **NTP o'chiq**, server vaqtidan **~48 sekund oldinda**, farq o'sib boradi.
- `devices.time_drift_seconds` da kuzatiladi, `scheduler` davriy tekshiradi.
- **Faza 1 da NTP yoqilishi kerak** (`setConfig` — alohida ruxsat bilan).
- 24 qurilma har biri o'z tezligida siljiydi → kirish/chiqish juftlashtirish buziladi.

---

## 6. Keyingi tasklar (yangilangan)

| Task | Nima | Holat |
|---|---|---|
| TASK-01 | Qurilma discovery | ✅ bajarildi |
| **TASK-02** | Laravel skeleton + docker-compose + PostgreSQL + UI `:6000` ko'tarilsin | keyingi |
| TASK-03 | `devices` CRUD UI: IP/user/parol qo'lda kiritiladi, parol shifrlangan, "Ulanishni tekshirish" tugmasi model/firmware'ni to'ldiradi | keyingi |
| TASK-04 | Teskari import: terminaldagi 9801 userni `people` ga tortish | TASK-03 dan keyin |
| TASK-05 | `ElmsDbSource` adapteri → `people` (keyin `HemisApiSource`) | mustaqil |
| TASK-06 | `people` UI: ro'yxat, qidiruv, **rasm tahrirlash** (asosiy ish) | TASK-02 dan keyin |
| TASK-07 | Foto validatsiya servisi (InsightFace, `:6001`) | mustaqil |
| **TASK-08** | **Yozuv yo'lini tasdiqlash** | ✅ **bajarildi 2026-08-11** |
| TASK-09 | Sync engine: `device_person_sync`, Horizon, qurilma bo'yicha rate-limit | 🟢 endi boshlanishi mumkin |

### TASK-08 natijasi: legacy CGI yozadi, RPC2 KERAK EMAS

`TEST0001` bilan jonli qurilmada sinaldi va darhol tozalandi.

| Amal | Endpoint | Natija |
|---|---|---|
| User qo'shish | `recordUpdater.cgi?action=insert&name=AccessControlCard` | ✅ `RecNo` qaytaradi |
| Yuz yuklash | `POST FaceInfoManager.cgi?action=add` (JSON) | ✅ |
| User o'chirish | `recordUpdater.cgi?action=remove&recno=N` | ✅ |
| Yuz o'chirish | `FaceInfoManager.cgi?action=remove&UserID=X` | ✅ |

**Uchta detal sync engine uchun kritik:**

1. **`CardNo` majburiy** — usiz `400`. Mavjud yozuvlarda qiymati bo'sh bo'lsa
   ham, parametr so'rovda bo'lishi shart.
2. **User va yuz alohida o'chiriladi** — `recordUpdater` yuzga tegmaydi.
   Odam o'chirilganda ikkala chaqiruv ham qilinsin, aks holda yetim yuz qoladi.
3. **base64 buyruq qatoriga sig'maydi** — HTTP tanasi orqali yuborilsin.

To'liq so'rov shablonlari: `device-probe/API-MATRIX.md` 4-band.

---

## 7. Hal qilinmagan savollar

1. **Test qurilma qachon bo'ladi?** TASK-02 shunsiz bajarilmaydi.
2. Qolgan 23 IP ro'yxati va yo'nalishi (entry/exit).
3. Terminaldagi 9801 user bilan nima qilamiz — solishtirib tozalanadimi yoki
   hammasi o'chirilib qaytadan sync qilinadimi?
4. NTP yoqishga ruxsat (davomat aniqligi uchun kritik).
5. UI autentifikatsiyasi: mini-DSS o'z login tizimimi yoki elms SSO?
   (1-qarorga ko'ra — o'zinikini kutamiz, tasdiqlansin.)
