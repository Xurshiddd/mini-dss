# TASK-01 — Dahua ASI7xxx terminal: discovery va API capability hisoboti

**Faza:** 0 (discovery)
**Rejim:** FAQAT O'QISH (read-only)
**Bajaruvchi:** agent
**Kutilgan vaqt:** 1–2 soat

---

## 1. Loyiha konteksti (agent buni bilishi kerak)

Biz Dahua face recognition terminali uchun **o'z boshqaruv tizimimizni** ("mini-DSS") yozamiz.
Dahua'ning rasmiy DSS dasturi litsenziyasiz cheklangan foydalanuvchi soni bilan ishlaydi,
lekin terminalning o'zi 50 000+ foydalanuvchini saqlaydi va Dahua CGI/HTTP API'ni
rasmiy hujjatlashtirgan. Ya'ni DSS shunchaki klient — biz o'z klientimizni yozamiz.
Bu qurilma egasi tomonidan o'z qurilmasining hujjatlashtirilgan API'sidan foydalanish,
hech qanday himoyani chetlab o'tish emas.

**Yakuniy tizim (kontekst uchun, bu taskda QURILMAYDI):**

| Element | Qaror |
|---|---|
| Maqsad | Kirish/chiqish nazorati + davomat (attendance) hisobi |
| Identifikatsiya | Faqat yuz (RFID karta ishlatilmaydi) |
| Foydalanuvchi manbasi | HEMIS (talaba/xodim bazasi) → sync → terminal |
| Backend | Laravel (admin panel) |
| Event listener | Alohida Python asyncio servisi |
| DB / Queue | PostgreSQL + Redis (BullMQ/Horizon) |
| Deploy | Docker, terminal bilan bir tarmoqda |
| Telegram | Keyingi fazada |

**Terminal:** `192.168.30.222` (statik IP, DHCP off, gateway `192.168.30.100`)
Web UI "WEB SERVICE" ko'rinishida (ehtimol Web 3.0 avlodi — tekshirilishi kerak).

---

## 2. Bu taskning maqsadi

Bitta savolga aniq javob berish: **qurilma API'ning qaysi avlodini qo'llaydi va
qaysi endpointlar amalda ishlaydi?**

Sababi: Dahua'da ikki xil, o'zaro mos kelmaydigan API to'plami bor. Xato to'plamni
tanlasak, kod yozilib bo'lgandan keyin qayta yozishga to'g'ri keladi. Bundan tashqari
ikkisini aralashtirish ma'lum bug'ga olib keladi — bir API bilan qo'shilgan
foydalanuvchi qurilma ekranida ko'rinmay qolishi mumkin.

| Vazifa | Legacy (Web 3.0) | Yangi (Web 5.0) |
|---|---|---|
| User qo'shish | `recordUpdater.cgi?action=insert&name=AccessControlCard` | `AccessUser.cgi?action=insertMulti` (JSON) |
| Face yuklash | `FaceInfoManager.cgi?action=add` (JSON + base64) | `AccessFace.cgi?action=insertMulti` |
| Loglar | `recordFinder.cgi?action=find&name=AccessControlCardRec` | bir xil |
| Real-time event | `snapManager.cgi?action=attachFileProc` | bir xil |
| Eshik ochish | `accessControl.cgi?action=openDoor&channel=1` | bir xil |

Auth: hamma joyda **HTTP Digest**.

**Asosiy hujjat:**
`https://files.dahua.support/Solutions/Access%20Control%20Solution/Integration/DAHUA%20ACCESS%20CONTROL%20PRODUCTS%20INTEGRATION%20INSTRUCTION%20Ver1.0.pdf`

---

## 3. QAT'IY CHEKLOVLAR — buzilmasin

Terminal **hozir ishlab turgan production qurilma**: unda allaqachon ro'yxatga olingan
foydalanuvchilar bor va odamlar u orqali kirib-chiqmoqda.

**TAQIQLANADI:**

- ❌ Har qanday `action=insert`, `action=update`, `action=remove`, `action=add`,
  `action=delete`, `action=clear`, `action=setConfig` chaqiruvi
- ❌ `accessControl.cgi?action=openDoor` / `closeDoor` (eshikni ochmaslik)
- ❌ `magicBox.cgi?action=reboot`, `rebootTime`, factory reset, firmware yuklash
- ❌ `userManager.cgi?action=addUser` / `modifyPassword` (qurilma admin akkauntiga tegmaslik)
- ❌ Tarmoq sozlamalariga tegish (IP/DHCP/gateway) — qurilma yo'qolib qoladi
- ❌ Parolni topishga urinish, bir nechta parol variantini sinash

**MAJBURIY:**

- ✅ Sekundda maksimum **1 so'rov** (`sleep 1` har chaqiruv orasida).
  Dahua qurilmalari ko'p so'rovda javob bermay qo'yadi.
- ✅ Parol **bir marta** sinaladi. Agar `401` kelsa — DARHOL to'xtash va odamdan
  so'rash. Dahua bir necha muvaffaqiyatsiz urinishdan keyin admin akkauntni
  vaqtincha bloklaydi, bu esa jonli tizimni ishdan chiqaradi.
- ✅ Parol repoga yozilmaydi. `.env` ichida, `.env` esa `.gitignore`da.
- ✅ Bir vaqtda faqat **bitta** event stream ulanishi.
- ✅ Har bir chaqiruvning xom (raw) javobi faylga saqlanadi.

Agar biror qadam noaniq bo'lsa yoki javob kutilganidan farq qilsa —
**taxmin qilmaslik, to'xtab so'rash.**

---

## 4. Bajarish qadamlari

### 4.0 — Tayyorgarlik

```bash
mkdir -p device-probe/raw && cd device-probe
cat > .env <<'EOF'
DAHUA_IP=192.168.30.222
DAHUA_USER=admin
DAHUA_PASS=          # <-- odamdan so'rang, bu yerga yozing, commit qilmang
EOF
echo ".env" > .gitignore
```

Tekshiring: `curl --version`, `jq --version`, `ffprobe -version`.
Yo'q bo'lsa o'rnatish uchun buyruqni odamga ko'rsating.

Tarmoq: `ping -c 3 192.168.30.222` va `nc -zv 192.168.30.222 80 554`.
Agar port 80 yopiq bo'lsa — Docker konteyner ichidan chiqmaganini tekshiring
(host network yoki to'g'ri bridge kerak).

### 4.1 — Qurilma identifikatsiyasi

Har birini alohida ishga tushirib, natijani `raw/01-identity.txt`ga yozing:

```
/cgi-bin/magicBox.cgi?action=getDeviceType
/cgi-bin/magicBox.cgi?action=getSoftwareVersion
/cgi-bin/magicBox.cgi?action=getHardwareVersion
/cgi-bin/magicBox.cgi?action=getSystemInfo
/cgi-bin/magicBox.cgi?action=getSerialNo
/cgi-bin/magicBox.cgi?action=getVendor
/cgi-bin/magicBox.cgi?action=getProductDefinition
```

Chaqiruv shabloni:

```bash
curl -s -m 10 --digest -u "$DAHUA_USER:$DAHUA_PASS" \
     -w "\n[HTTP %{http_code}]\n" "http://$DAHUA_IP$ENDPOINT"
```

**Kerakli natija:** aniq model (masalan `DHI-ASI7213Y-V3`) va firmware versiyasi.
Bu ikkisi hisobotning eng muhim qatori.

### 4.2 — API avlodini aniqlash

Faqat **hisoblash/o'qish** actionlari. Har biri uchun HTTP kodni yozib oling:
`200` = mavjud, `400/404/500` = mavjud emas.

```
/cgi-bin/AccessUser.cgi?action=count
/cgi-bin/AccessFace.cgi?action=count
/cgi-bin/AccessCard.cgi?action=count
/cgi-bin/AccessUser.cgi?action=list&count=1
/cgi-bin/recordFinder.cgi?action=getQuerySize&name=AccessControlCard
/cgi-bin/recordFinder.cgi?action=getQuerySize&name=AccessControlCardRec
/cgi-bin/recordFinder.cgi?action=getQuerySize&name=FaceRecognition
```

`FaceInfoManager.cgi` va `recordUpdater.cgi` uchun **hech qanday yozuv actioni
sinalmaydi.** Ular mavjudligini bilvosita aniqlaymiz: `action=get` yoki
`action=getCaps` bor bo'lsa shuni sinang, bo'lmasa "aniqlanmadi" deb yozing.

Natija → `raw/02-api-generation.txt`

### 4.3 — Sig'im va konfiguratsiya (o'qish)

```
/cgi-bin/configManager.cgi?action=getConfig&name=AccessControl
/cgi-bin/configManager.cgi?action=getConfig&name=AccessTimeSchedule
/cgi-bin/configManager.cgi?action=getConfig&name=FaceRecognitionServer
/cgi-bin/configManager.cgi?action=getConfig&name=NTP
/cgi-bin/configManager.cgi?action=getConfig&name=Locales
/cgi-bin/global.cgi?action=getCurrentTime
```

Bu yerda ayniqsa quyidagilarni topib, hisobotda alohida ajratib ko'rsating:
- Eshik (door/channel) soni va indekslari — `openDoor&channel=` uchun to'g'ri qiymat
- `TimeSections` / schedule indekslari — user qo'shishda majburiy parametr
- Yuz o'xshashlik chegarasi (threshold/similarity) qiymati
- NTP holati va qurilma vaqti server vaqtidan farqi (davomat aniqligi uchun kritik)

Natija → `raw/03-config.txt`

### 4.4 — Mavjud foydalanuvchilarni sanash (faqat o'qish)

Qurilmada hozir nechta user va nechta face template borligini aniqlang:

```
/cgi-bin/recordFinder.cgi?action=doSeekFind&name=AccessControlCard&count=5
```

yoki yangi API bo'lsa `AccessUser.cgi?action=list&count=5`.

**Diqqat:** natijada real odamlarning ismlari va ID'lari bo'ladi. Hisobotga
**faqat sonlarni va bitta anonimlashtirilgan namuna yozuvni** (ismi `***` bilan
almashtirilgan) kiriting — to'liq ro'yxatni hisobotga qo'ymang.

`UserID` formatini aniqlash muhim: raqammi, satrmi, uzunligi qancha?
HEMIS ID'sini shu formatga moslashimiz kerak bo'ladi.

Natija → `raw/04-users-sample.txt`

### 4.5 — Loglarni o'qish va pagination xatti-harakati

```
/cgi-bin/recordFinder.cgi?action=find&name=AccessControlCardRec&count=10
```

Keyin oxirgi yozuvning `CreateTime` qiymatini `StartTime` sifatida berib
ikkinchi sahifani so'rang. Aniqlang:

- `totalCount` va `found` qanday qaytadi
- Ikki sahifada takrorlangan yozuvlar bormi (Dahua hujjati bu haqda ogohlantiradi —
  dedup kerak bo'ladi)
- `Method` maydonining qiymati (yuz orqali ochilishda `15` kutiladi)
- `Type` maydoni `Entry`/`Exit` sifatida to'g'ri kelayotganini tekshiring —
  kirish/chiqish farqlash shu maydonga bog'liq
- Vaqt UTC'dami yoki local? (Toshkent UTC+5)

Natija → `raw/05-records.txt`

### 4.6 — Real-time event stream sinovi

Bu qadamda **odam yordami kerak.** Ishga tushirishdan oldin odamga aytib qo'ying:
"terminal oldiga borib yuzingizni ko'rsatib qo'ying".

```bash
timeout 90 curl -s -N --digest -u "$DAHUA_USER:$DAHUA_PASS" \
  "http://$DAHUA_IP/cgi-bin/snapManager.cgi?action=attachFileProc&Flags[0]=Event&Events=[AccessControl]&heartbeat=5" \
  > raw/06-events.bin
```

Keyin tahlil qiling:

- `Heartbeat` xabarlari 5 sekundda kelayotganini tasdiqlang
- `Events[0].Code=AccessControl` hodisasi qo'lga tushdimi
- Hodisa ichida qaysi maydonlar bor: `UserID`, `Type`, `Status`, `Method`,
  `RecNo`, `Name`
- multipart boundary formati qanday (Python parseri uchun kerak)
- Hodisa bilan birga **JPEG snapshot** kelyaptimi? Kelsa — o'lchami qancha?
  (bu davomat jurnalida foto saqlash imkonini beradi)

Agar `snapManager.cgi` ishlamasa, alternativa sifatida sinang:
`/cgi-bin/eventManager.cgi?action=attach&codes=[All]&heartbeat=5`

Xom binar faylni saqlab qo'ying — Python parserini yozishda test fixture bo'ladi.

### 4.7 — RTSP oqim

```bash
ffprobe -rtsp_transport tcp -v error -show_streams -show_format \
  "rtsp://$DAHUA_USER:$DAHUA_PASS@$DAHUA_IP:554/cam/realmonitor?channel=1&subtype=0" \
  2>&1 | tee raw/07-rtsp.txt
```

`subtype=1` (sub stream) bilan ham sinang. Kodek (H.264/H.265), rezolyutsiya,
FPS'ni yozib oling. Web UI'da Main Format `720P / 30fps / 2Mbps`,
Extra Format `VGA / 30fps / 1024Kbps` ko'rsatilgan — amalda shundayligini
tasdiqlang. (Video Standard `NTSC` qo'yilgan — buni hisobotda qayd qiling,
lekin O'ZGARTIRMANG.)

---

## 5. Yakuniy natija

Quyidagi fayllar yaratilishi kerak:

```
device-probe/
├── raw/                      # barcha xom javoblar (01..07)
├── DEVICE-REPORT.md          # asosiy hisobot
├── API-MATRIX.md             # endpoint → ishlaydi/ishlamaydi jadvali
└── probe.sh                  # qayta ishga tushirish uchun skript
```

### `DEVICE-REPORT.md` tarkibi

1. **Xulosa** — 3–4 qatorda: model, firmware, API avlodi, keyingi qadam
2. **Qurilma pasporti** — model, firmware, seriya, sig'im, eshik soni
3. **API avlodi qarori** — legacy yoki yangi, va nima asosida shunday xulosa qilindi
4. **Adapter uchun aniq endpointlar** — user qo'shish, face yuklash, log o'qish,
   event, eshik ochish uchun to'liq URL shabloni va parametrlar
5. **`UserID` formati** va HEMIS ID'sini unga qanday map qilish taklifi
6. **Vaqt zonasi masalasi** — qurilma vaqti, NTP, UTC farqi
7. **Event stream texnik detallari** — boundary, maydonlar, snapshot bor/yo'q
8. **Risklar va noaniqliklar** — sinab ko'rilmagan (yozuv talab qiladigan)
   narsalar ro'yxati, ular Faza 1'da test qurilmasida sinaladi
9. **Ochiq savollar** — odamdan aniqlanishi kerak bo'lgan narsalar

### `probe.sh`

Butun 4.1–4.5 qadamlarini qayta ishga tushiradigan idempotent skript.
Parolni `.env`dan oladi, `raw/`ga yozadi, `sleep 1` bilan rate-limit qiladi,
`set -euo pipefail` ishlatadi. Keyinchalik boshqa terminallarni tekshirishda
qayta ishlatiladi.

---

## 6. Bu taskda BAJARILMAYDI

Quyidagilar keyingi tasklar, hozir boshlanmasin:

- HEMIS API'sini o'rganish → TASK-02
- Laravel loyihasini yaratish → TASK-03
- Python event listener yozish → TASK-04
- Foto validatsiya pipeline (InsightFace) → TASK-05
- Docker compose → TASK-06

Agent faqat discovery qiladi va hisobot beradi. Kod arxitekturasi hisobot
o'qilgandan keyin, odam bilan kelishilib tanlanadi.
