# DEVICE-REPORT — Dahua ASI7213Y-V3 discovery hisoboti

**Task:** TASK-01 (Faza 0, read-only discovery)
**Sana:** 2026-08-11
**Qurilma:** `192.168.30.222`
**Rejim:** faqat o'qish — hech qanday yozuv/o'zgartirish operatsiyasi bajarilmadi

---

## 1. Xulosa

Qurilma **DHI-ASI7213Y-V3**, firmware `1.000.0000014.0.R (2021-08-23)`.
API avlodi — **Legacy (Web 3.0)**: `recordFinder.cgi` / `recordUpdater.cgi` /
`FaceInfoManager.cgi` semantikasi. Yangi `AccessUser.cgi` / `AccessFace.cgi`
to'plami **yo'q**. Hodisa oqimi (`snapManager.cgi`) to'liq ishlaydi va har bir
hodisa bilan **JPEG foto** ham keladi — bu davomat jurnalida foto saqlash
imkonini beradi.

**Keyingi qadam:** adapterni legacy CGI ustiga qurish. Ammo yozuv yo'li
(user qo'shish, yuz yuklash) jonli qurilmada sinalmadi — **Faza 1 uchun test
qurilma majburiy**. Eng katta ochiq savol: yuz yuklash CGI orqali ishlaydimi
yoki RPC2 talab qiladimi (2-bandga qarang).

---

## 2. Qurilma pasporti

| Xususiyat | Qiymat |
|---|---|
| Model | `DHI-ASI7213Y-V3` |
| Firmware | `1.000.0000014.0.R`, build `2021-08-23` |
| Hardware | `1.00` |
| Protsessor | `HI3516DV300` |
| Seriya raqami | `7J0A8D6PAJ7315A` |
| Ishlab chiqaruvchi | Dahua |
| Web UI | Vue SPA ("WEB SERVICE"), RPC2 protokoli ustida |
| **Ro'yxatdagi userlar** | **9801** |
| **Log yozuvlari** | **100 000 — buffer TO'LA** (halqa/ring buffer) |
| Eshiklar | **1 ta** — `AccessControl[0].Name=Door1`, indeks `0`, `ReaderID=1` |
| Vaqt jadvallari | 128 slot, faqat **indeks 0** yoqilgan (`time.template.name.all`, 24/7) |
| Kirish rejimi | `AccessProperty=bidirect` |
| Ochiq portlar | 80 (HTTP), 554 (RTSP) |

### Log bufferi — muhim
`RecNo` oralig'i `546823 … 646822` (aynan 100 000 ta).
Eng eski saqlangan yozuv: **2026-02-20**, eng yangisi — jonli.
Ya'ni buffer ~6 oylik tarixni saqlaydi va **eskisi o'chib boradi**.
Kuniga ~580 yozuv. Yo'qotmaslik uchun muntazam poll qilish shart.

---

## 3. API avlodi qarori: **Legacy (Web 3.0)**

**Nima asosida:**

1. **Ijobiy dalil (hal qiluvchi):** `recordFinder.cgi?action=getQuerySize&name=AccessControlCard`
   → `200`, `Size=9801` — real ma'lumot bilan ishlaydi.
2. **Salbiy dalil:** `AccessUser.cgi`, `AccessFace.cgi`, `AccessCard.cgi` —
   barchasi `400`.
3. **Kontrol test:** mavjud bo'lmagan CGI ham `400` qaytaradi, ya'ni `400`
   o'zi "yo'q" degani emas. Shu sababli qaror **ijobiy dalilga** asoslangan.
4. Firmware 2021-yil — Web 5.0 avlodidan oldingi liniya.

**Muhim nuans — RPC2 qatlami.**
Qurilmaning o'z web UI'si CGI emas, **RPC2 (JSON-RPC)** ishlatadi. Bundle'dan
statik o'qish orqali `RecordUpdater.insert/update/remove/import`,
`FaceInfoManager.getCaps`, `AccessUser.list`, hatto `Attendance.*` metodlari
topildi. Ya'ni "zamonaviy" API bu qurilmada **RPC2 shaklida** mavjud, Web 5.0
CGI shaklida emas. Bu bizga ikkinchi variant beradi (8-bandga qarang).

---

## 4. Adapter uchun aniq endpointlar

Auth: **HTTP Digest** hamma joyda. Batafsil jadval → `API-MATRIX.md`.

### O'qish — tasdiqlangan, ishlaydi

```
# User soni
GET /cgi-bin/recordFinder.cgi?action=getQuerySize&name=AccessControlCard

# Userlarni sahifalab o'qish
GET /cgi-bin/recordFinder.cgi?action=doSeekFind&name=AccessControlCard&count=100&offset=0

# Loglarni sahifalab o'qish
GET /cgi-bin/recordFinder.cgi?action=doSeekFind&name=AccessControlCardRec&count=100&offset=<N>

# Real-time hodisa oqimi
GET /cgi-bin/snapManager.cgi?action=attachFileProc&Flags[0]=Event&Events=[AccessControl]&heartbeat=5
```

> ⚠️ **`condition.StartTime` / `condition.EndTime` E'TIBORGA OLINMAYDI.**
> Filtrlangan so'rov filtrsiz bilan **bir xil** natija qaytardi (`Size` hamon
> 100000, yozuvlar hamon fevraldan boshlandi). Xato emas, ogohlantirish ham
> yo'q — **jimgina noto'g'ri ma'lumot**. Vaqt bo'yicha poll qilmang.
>
> `factory.create` (stateful finder) ham `400` — bu firmware'da umuman yo'q.
>
> **To'g'ri strategiya:** `offset` + **`RecNo` kursori**. `RecNo` monoton
> o'sadi va global unikal — dedup uchun ideal kalit.

### Yozish — SINALMAGAN (Faza 1)

```
POST /cgi-bin/recordUpdater.cgi?action=insert&name=AccessControlCard
POST /cgi-bin/FaceInfoManager.cgi?action=add          # JSON + base64 JPEG
GET  /cgi-bin/accessControl.cgi?action=openDoor&channel=?
```

Majburiy parametrlar (mavjud yozuvlardan olingan):
`UserID`, `CardName`, `CardStatus=0`, `CardType=0`, `Doors[0]=0`,
`TimeSections[0]=0`, `ValidDateStart`, `ValidDateEnd`, `UserType=0`.

---

## 5. `UserID` formati va HEMIS mapping

**`UserID` — o'zgaruvchan uzunlikdagi ALFANUMERIK satr, raqam EMAS.**
50 ta namuna (turli offsetlardan):

| Shablon | Ulush | Namuna | Talqin |
|---|---|---|---|
| 12 raqam | 38/50 (76%) | `331221100491` | HEMIS talaba ID |
| 2 harf + 7 raqam | 7/50 (14%) | `AB5019365` | pasport seriyasi (xodimlar?) |
| 4 raqam | 4/50 (8%) | `1519` | qo'lda kiritilgan qisqa ID |
| 11 raqam | 1/50 (2%) | `31221101591` | ⚠️ ehtimol xato — 12 raqamlidan bitta kam |

### Xulosalar

1. **DB ustuni `VARCHAR(32)` bo'lsin, `BIGINT` EMAS.** Aks holda pasport
   formatidagi ID'lar (`AB5019365`) sig'maydi va butun sync buziladi.
2. HEMIS ID'si allaqachon 12 raqamli formatga mos — **transformatsiya kerak emas**,
   to'g'ridan-to'g'ri `UserID` sifatida ishlatiladi.
3. Mavjud ma'lumot **iflos**: 3 xil begona format aralashgan. Sync boshlashdan
   oldin **solishtirish (reconciliation) bosqichi** kerak — qaysi yozuv HEMIS'da
   bor, qaysi biri yo'q, qaysi biri xato kiritilgan.
4. `RecNo` ≠ `UserID`. `RecNo` — qurilmaning ichki, siyrak (sparse) raqami
   (max 13011, lekin 9801 ta yozuv). Update/delete uchun `RecNo` kerak,
   sync kaliti sifatida `UserID` ishlatiladi.

---

## 6. Vaqt zonasi masalasi

| Nima | Qiymat |
|---|---|
| Qurilma vaqti | `2026-08-11 11:48:28` (local) |
| Server vaqti (o'sha payt) | `2026-08-11 11:47:40 +0500` |
| **Farq** | **qurilma ~48 sekund OLDINDA** |
| NTP | **`Enable=false` — O'CHIRILGAN** |
| Qurilma TZ | `TimeZone=7` = `Ashkhabad` = **UTC+5** (Toshkent bilan bir xil) |
| DST | o'chirilgan |

**Log/hodisadagi vaqt formati — aniq hal qilindi:**
`CreateTime` / `UTC` / `RealUTC` — **Unix epoch, UTC**.
Tasdiq: `CreateTime=1771574232` = `2026-02-20 07:57:12 UTC`, o'sha yozuvning
snapshot fayl nomi esa `...20260220125712487.jpg` = `12:57:12` local.
Farq aynan **+5 soat**. Ya'ni epoch UTC, ekran local.

> ⚠️ **Risk:** NTP o'chiq bo'lgani uchun farq vaqt o'tishi bilan **o'sib boradi**.
> 24 qurilmada har biri o'z tezligida siljiydi — kirish/chiqish juftlashtirish
> buziladi. **Faza 1 da NTP yoqilishi kerak** (bu `setConfig` — bu taskda
> taqiqlangan edi, alohida ruxsat bilan bajariladi).

---

## 7. Event stream texnik detallari

**Ishlaydi.** 90 sekundlik sinovda **9 ta hodisa** + **4 heartbeat**, 512 945 bayt.

| Xususiyat | Qiymat |
|---|---|
| Content-Type | `multipart/x-mixed-replace; boundary=myboundary` |
| Ajratgich | `--myboundary` |
| Heartbeat | `Content-Type: text/plain`, `Content-Length:9`, tanasi `Heartbeat` |
| Hodisa | `Content-Type: text/plain`, `Content-Length: 1103–1105`, `key=value` qatorlar |
| Foto | `Content-Type: image/jpeg`, **37–65 KB**, har bir hodisada bor |

### Hodisa maydonlari (to'liq ro'yxat)

```
Events[0].EventBaseInfo.Code=AccessControl
Events[0].EventBaseInfo.Action=Pulse
Events[0].EventBaseInfo.Index=0
Events[0].UserID=AB5603181
Events[0].CardName=<ism>
Events[0].CardNo=            Events[0].CardType=0
Events[0].Method=15          # 15 = yuz
Events[0].Type=Entry
Events[0].Status=1           # 1 = muvaffaqiyat, 0 = muvaffaqiyatsiz
Events[0].ErrorCode=0
Events[0].Door=0             Events[0].ReaderID=1
Events[0].CreateTime=1786431765
Events[0].UTC=1786431765     Events[0].RealUTC=1786431765
Events[0].Similarity=93      # yuz o'xshashlik ballari
Events[0].Alive=100          # liveness (tiriklik) ballari
Events[0].FeatureId=24627
Events[0].SnapPath=/var/tmp/partsnap24.jpg
Events[0].UserType=0
Events[0].ImageInfo[N].{Type,Width,Height,Offset,Length}
```

### JPEG qismining tuzilishi — Python parseri uchun

Bitta `image/jpeg` qismi ichida **3 ta ketma-ket JPEG** bor. Ularni
`ImageInfo[N].Offset` va `.Length` bo'yicha kesib olish kerak:

| N | Type | O'lcham | Offset | Length |
|---|---|---|---|---|
| 0 | 1 | 360×640 | 0 | 9052 |
| 1 | 0 | 409×477 | 9052 | 15965 |
| 2 | 2 | 440×344 | 25017 | 12840 |

Yig'indi `9052+15965+12840 = 37857` = `Content-Length` — aynan mos.

### Muhim xatti-harakatlar

1. ⚠️ **Stream ochiq turganda qurilma port 80'ga YANGI ulanishlarni QABUL
   QILMAYDI** — `connection refused`. Stream yopilgach darhol tiklanadi
   (tekshirildi). Ya'ni event listener va poller **bir qurilmada bir vaqtda
   ishlay olmaydi**. Arxitekturada hisobga olinsin.
2. **Hodisada `RecNo` YO'Q.** Hodisani log yozuvi bilan bog'lash uchun
   `UserID` + `CreateTime` juftligidan foydalanish kerak.
3. **Takroriy hodisalar.** Bitta odam 21 sekund ichida 4 marta yozuv hosil
   qildi (`1786431056, 57, 75, 76`). **Debounce majburiy** — masalan bir xil
   `UserID` uchun N sekund ichidagi takrorlarni tashlab yuborish.
4. **Muvaffaqiyatsiz tanish:** `Status=0` va `UserID=` **bo'sh** bo'ladi.
   Parser buni albatta hisobga olsin (`None`/`null` bo'lmasin).
5. `curl` uchun **`-g` (`--globoff`) shart** — `Events=[AccessControl]` dagi
   `[ ]` ni curl glob deb qabul qiladi va so'rov `exit=3` bilan yiqiladi.
   (Bu xatoga ushbu discovery paytida tushildi va tuzatildi.)

---

## 8. Risklar va noaniqliklar

### 8.1 Kritik — Faza 1 da hal qilinishi shart

| # | Masala | Holat |
|---|---|---|
| 1 | **Yuz yuklash yo'li noma'lum.** `FaceInfoManager.cgi` CGI sifatida `getCaps`/`find` ga `400` berdi. Lekin RPC2 da `FaceInfoManager` **mavjud**. CGI `action=add` ishlaydimi — **sinalmagan** (yozuv taqiqlangan edi). | ⛔ Ochiq |
| 2 | **User qo'shish sinalmagan.** `recordUpdater.cgi?action=insert` — legacy hujjatga ko'ra to'g'ri yo'l, `recordFinder.cgi` ishlagani buni qo'llab-quvvatlaydi, lekin tasdiqlanmagan. | ⛔ Ochiq |
| 3 | **`openDoor` uchun `channel` qiymati noaniq.** Config indeksi `0` (`AccessControl[0]`), yozuvlarda `Door=0`, lekin Dahua hujjatida `channel=1`. Sinab bo'lmadi (eshik ochish taqiqlangan). | ⛔ Ochiq |
| 4 | **NTP o'chiq**, qurilma 48s oldinda. `setConfig` kerak. | ⛔ Ochiq |

### 8.2 Sinalmagan (yozuv talab qiladi)

- `recordUpdater.cgi` — `insert` / `update` / `remove`
- `FaceInfoManager.cgi?action=add` — base64 JPEG bilan
- `accessControl.cgi?action=openDoor`
- RPC2 sessiya login va `RecordUpdater.import` (ommaviy import)
- `RPC_Loadfile` orqali snapshot yuklab olish — Digest'ga `401` berdi,
  qoidaga muvofiq to'xtatildi (qurilma sog'lom qoldi, akkaunt bloklanmadi)

### 8.3 Yuz sifat parametrlari (foto validatsiya pipeline uchun — TASK-05)

`VideoAnalyseRule[0][0]` dan:

| Parametr | Qiymat |
|---|---|
| `EyesDistThreshold` | 60 (ko'zlar orasi min. masofa) |
| `MinQuality` | 50 |
| `SizeFilter.MinSize` | 700×700 (8192 normalizatsiyada) |
| `FilterUnAliveEnable` | `true` (anti-spoofing yoqilgan) |
| `FilterMaskUnAliveEnable` | `true` |
| `FaceAntifakeStrategy.Level` | 255 |
| `FaceImageFilterStrategy.Level` | `Low` |
| `CitizenPictureCompareRule.Threshold` | 60 |

> ⚠️ **Tanish (recognition) chegarasi aniq topilmadi.** `CitizenPictureCompareRule.Threshold=60`
> eng ehtimoliy nomzod, lekin u ID-karta fotosi bilan solishtirish uchun bo'lishi
> mumkin. **Amaliy o'lchov ishonchliroq:** hodisalardagi `Similarity` maydoni
> real ballarni beradi (kuzatilgan qiymat: **93**). Faza 1 da bir necha yuz
> hodisadan `Similarity` taqsimotini yig'ib, haqiqiy chegarani empirik aniqlash
> kerak.

---

## 9. Ochiq savollar — odamdan javob kerak

1. **Qolgan 23 ta terminalning IP ro'yxati** va har biri kirishmi (entry) yoki
   chiqishmi (exit)? Jami 24 ta: 12 kirish + 12 chiqish.
   → `probe.sh` IP'ni parametr sifatida qabul qiladi, ro'yxat kelishi bilan
   hammasini tekshirish mumkin.
2. **Test qurilma bormi?** Yozuv operatsiyalari (8.2 ro'yxati) jonli qurilmada
   sinalmaydi. Bittasini vaqtincha ishdan chiqarib test uchun ajratish mumkinmi?
3. **Barcha 24 qurilmada firmware bir xilmi?** Agar farq qilsa, API avlodi ham
   farq qilishi mumkin — har birini alohida tekshirish kerak.
4. **Qurilmadagi 9801 user bilan nima qilamiz?** HEMIS bilan solishtirilib
   tozalanadimi, yoki hammasi o'chirilib qaytadan sync qilinadimi?
   (3 xil ID formati aralashgan — 5-bandga qarang.)
5. **NTP serverini yoqishga ruxsat.** Bu `setConfig` — davomat aniqligi uchun
   kritik, lekin bu taskda taqiqlangan edi.
6. **Qurilmaning ichki `Attendance.*` moduli** ishlatilyaptimi? Agar ha bo'lsa,
   bizning davomat hisobimiz bilan ziddiyat bo'lishi mumkin.

---

## Ilova: yaratilgan fayllar

```
device-probe/
├── raw/
│   ├── 01-identity.txt          # qurilma pasporti
│   ├── 02-api-generation.txt    # API avlodi + kontrol test
│   ├── 03-config.txt            # config (NTP, eshik, schedule, VideoAnalyseRule)
│   ├── 03b-fullconfig.txt       # to'liq dump, 10552 qator  [gitignore]
│   ├── 04-users-sample.txt      # user namunalari             [gitignore: shaxsiy]
│   ├── 05-records.txt           # log namunalari              [gitignore: shaxsiy]
│   ├── 05b-statefind.txt        # factory.create sinovi
│   ├── 06-events.bin            # 512 KB event stream         [gitignore: ism+foto]
│   ├── 06-events-headers.txt    # HTTP sarlavhalar
│   ├── 07-rtsp.txt              # RTSP oqim ma'lumotlari
│   └── 192.168.30.222/          # probe.sh ning takroriy natijasi
├── DEVICE-REPORT.md             # ushbu fayl
├── API-MATRIX.md                # endpoint jadvali
├── probe.sh                     # 4.1–4.5, IP parametrli
├── events.sh                    # 4.6 event stream
├── rtsp.sh                      # 4.7 RTSP
├── .env                         # parol                       [gitignore]
└── .gitignore
```

**Ishlatish:**
```bash
./probe.sh 192.168.30.223 entry   # boshqa terminal
./events.sh 192.168.30.223 90
./rtsp.sh 192.168.30.223
```

`raw/06-events.bin` — Python multipart parseri uchun tayyor **test fixture**
(9 ta real hodisa + 27 ta JPEG).
