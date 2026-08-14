# API-MATRIX — DHI-ASI7213Y-V3 @ 192.168.30.222

Sinov sanasi: **2026-08-11**, firmware `1.000.0000014.0.R (build 2021-08-23)`.
Auth: **HTTP Digest**, hamma CGI endpointlarda.

> **`400` ni qanday o'qish kerak:** kontrol test o'tkazildi — mavjud bo'lmagan CGI
> (`zzzNotExist.cgi`) ham, mavjud CGI'ning noto'g'ri actioni ham bir xil
> `400 Bad Request` qaytaradi. Ya'ni **`400` "endpoint yo'q" degani emas**.
> Xulosalar `200` qaytargan ijobiy dalillarga asoslangan.

---

## 1. Ishlaydi (tasdiqlangan, `200`)

| Endpoint | Natija |
|---|---|
| `magicBox.cgi?action=getDeviceType` | `type=DHI-ASI7213Y-V3` |
| `magicBox.cgi?action=getSoftwareVersion` | `1.000.0000014.0.R,build:2021-08-23` |
| `magicBox.cgi?action=getHardwareVersion` | `1.00` |
| `magicBox.cgi?action=getSystemInfo` | SoC `HI3516DV300`, SN, deviceType |
| `magicBox.cgi?action=getSerialNo` | `7J0A8D6PAJ7315A` |
| `magicBox.cgi?action=getVendor` | `Dahua` |
| `global.cgi?action=getCurrentTime` | `result=2026-08-11 11:48:28` (local) |
| `configManager.cgi?action=getConfig&name=NTP` | NTP o'chiq, TZ=7 |
| `configManager.cgi?action=getConfig&name=Locales` | DST o'chiq |
| `configManager.cgi?action=getConfig&name=AccessControl` | 1 ta eshik |
| `configManager.cgi?action=getConfig&name=AccessControlGeneral` | bidirect, voice ro'yxati |
| `configManager.cgi?action=getConfig&name=AccessTimeSchedule` | 128 slot, 1 tasi yoqilgan |
| `configManager.cgi?action=getConfig&name=VideoAnalyseRule` | yuz sifat filtrlari |
| `configManager.cgi?action=getConfig&name=All` | to'liq dump, ~650 KB / 10552 qator |
| `recordFinder.cgi?action=getQuerySize&name=AccessControlCard` | `Size=9801` |
| `recordFinder.cgi?action=getQuerySize&name=AccessControlCardRec` | `Size=100000` |
| `recordFinder.cgi?action=doSeekFind&name=AccessControlCard&count=N&offset=M` | user ro'yxati |
| `recordFinder.cgi?action=doSeekFind&name=AccessControlCardRec&count=N&offset=M` | log yozuvlari |
| `recordFinder.cgi?action=find&name=...&count=N` | `doSeekFind` bilan bir xil |
| `snapManager.cgi?action=attachFileProc&Flags[0]=Event&Events=[AccessControl]&heartbeat=5` | multipart event stream + JPEG |
| `rtsp://.../cam/realmonitor?channel=1&subtype=0` | H.264 High 1280×720 30fps |
| `rtsp://.../cam/realmonitor?channel=1&subtype=1` | H.264 High 640×480 30fps |
| `GET /` va `/static/js/app.*.js` | Vue SPA, RPC2 metod nomlari ichida |

## 2. Ishlamaydi (`400`)

| Endpoint | Izoh |
|---|---|
| `AccessUser.cgi?action=count` / `list` | Web 5.0 CGI to'plami — yo'q |
| `AccessFace.cgi?action=count` | Web 5.0 — yo'q |
| `AccessCard.cgi?action=count` | Web 5.0 — yo'q |
| `AccessControl.cgi?action=getCaps` | yo'q |
| `FaceInfoManager.cgi?action=getCaps` / `find` | CGI sifatida yo'q (RPC2 da bor — quyiga qarang) |
| `recordUpdater.cgi?action=getCaps` | `getCaps` yaroqsiz action; CGI mavjudligi **aniqlanmadi** |
| `recordFinder.cgi?action=getQuerySize&name=FaceRecognition` | bunday jadval yo'q |
| `recordFinder.cgi?action=factory.create` | **stateful finder umuman yo'q** |
| `configManager.cgi?...&name=FaceRecognition` / `FaceRecognitionServer` | bunday config yo'q |
| `magicBox.cgi?action=getProductDefinition` | yo'q |

## 3. Alohida holat (`401`)

| Endpoint | Izoh |
|---|---|
| `/cgi-bin/RPC_Loadfile/<path>.jpg` | Digest auth qabul qilmaydi — RPC2 sessiya kerak. **Sinov to'xtatildi** (task qoidasi). Qurilma sog'lom, akkaunt bloklanmadi (keyingi so'rov `200`). |

## 4. YOZUV — 2026-08-11 da amalda TASDIQLANDI ✅

`TEST0001` soxta foydalanuvchi bilan sinaldi va darhol tozalandi
(`write-test.sh`, `write-test-variants.sh`). Qurilma boshlang'ich holatiga
qaytdi: 9801 foydalanuvchi.

**RPC2 KERAK EMAS — hammasi legacy CGI orqali ishlaydi.**

| Endpoint | Natija |
|---|---|
| `recordUpdater.cgi?action=insert&name=AccessControlCard` | ✅ `200`, `RecNo=13012` qaytardi |
| `recordUpdater.cgi?action=remove&name=AccessControlCard&recno=N` | ✅ `200`, `OK` |
| `POST FaceInfoManager.cgi?action=add` (JSON) | ✅ `200`, `OK` |
| `FaceInfoManager.cgi?action=remove&UserID=X` | ✅ `200`, `OK` |
| `recordUpdater.cgi?action=clear&name=AccessControlCard` | ✅ `200` — **HAMMA userni bir so'rovda o'chiradi** |
| `FaceInfoManager.cgi?action=clear` | ❌ `400` — ommaviy clear YO'Q, yuzlar birma-bir o'chiriladi |
| `recordUpdater.cgi?action=update` | ⛔ hali sinalmagan |
| `accessControl.cgi?action=openDoor&channel=?` | ⛔ sinalmagan; `channel` 0 yoki 1 — **noaniq** |

### ⚠️ `CardNo` MAJBURIY — eng muhim detal

```
CardNo'siz   → 400 Bad Request
CardNo bilan → 200 OK
```

Qurilmadagi mavjud 9801 yozuvning **hammasida `CardNo` qiymati BO'SH**.
Ya'ni qiymat bo'sh bo'lishi mumkin, lekin **parametrning o'zi so'rovda
bo'lishi shart**. Bu hujjatda yozilmagan.

Bitta `400` ga qarab "legacy yozmaydi, RPC2 kerak" degan xulosa qilish —
xato bo'lardi va butunlay keraksiz RPC2 klienti yozilardi.

### Ishlaydigan so'rovlar

```bash
# 1. User qo'shish  → RecNo=<N> qaytaradi
GET /cgi-bin/recordUpdater.cgi?action=insert&name=AccessControlCard
    &UserID=TEST0001&CardName=WRITETEST&CardNo=TEST0001
    &CardStatus=0&CardType=0&UserType=0
    &Doors[0]=0&TimeSections[0]=0
    &ValidDateStart=2026-08-11%2000:00:00&ValidDateEnd=2027-08-11%2023:59:59

# 2. Yuz yuklash — JSON tanasi bilan POST
POST /cgi-bin/FaceInfoManager.cgi?action=add
Content-Type: application/json
{"UserID":"TEST0001","Info":{"UserID":"TEST0001","PhotoData":["<base64 JPEG>"]}}

# 3. O'chirish — user va yuz ALOHIDA o'chiriladi
GET /cgi-bin/recordUpdater.cgi?action=remove&name=AccessControlCard&recno=<N>
GET /cgi-bin/FaceInfoManager.cgi?action=remove&UserID=TEST0001
```

> ⚠️ **User o'chirilganda yuz shabloni O'CHMAYDI.** Ikkisi alohida saqlanadi —
> `recordUpdater` faqat `AccessControlCard` yozuvini o'chiradi. Yetim yuz
> qolmasligi uchun `FaceInfoManager.cgi?action=remove` ham chaqirilishi kerak.

> ⚠️ **YOZUV SUR'ATI — o'qishdan ancha og'ir.** 300 ms oraliq bilan ketma-ket
> `FaceInfoManager.cgi?action=remove` yuborilganda qurilma **~90 ta so'rovdan
> keyin javob bermay qo'ydi** (`cURL error 28`, connection timeout). Qurilma
> o'zi tiklandi (qulash emas, ortiqcha yuklanish). Yozuv sikllarida **≥700 ms**
> oraliq, timeout'da backoff va uzilganda kaldan davom etish shart.
> (`app/Console/Commands/DeviceWipe.php` shunday yozilgan.)

> ⚠️ **base64'ni buyruq qatoriga bermang.** 60 KB rasm ~80 KB base64 beradi va
> `curl --data "$JSON"` "Argument list too long" bilan yiqiladi — so'rov
> qurilmagacha yetib bormaydi. Faylga yozib `--data-binary @fayl` ishlatilsin.

## 5. RPC2 qatlami (qurilmaning o'z web UI'si shuni ishlatadi)

`app.js` bundle'idan **statik o'qish** orqali aniqlandi — hech biri chaqirilmadi.
Kirish uchun RPC2 sessiya login kerak (Faza 1 ga qoldirildi).

| RPC2 metod | Vazifa |
|---|---|
| `RecordUpdater.insert` / `.update` / `.remove` / `.removeEx` / `.clear` | user yozish |
| `RecordUpdater.import` / `.importFile` / `.exportFile` / `.getFileImportState` | **ommaviy (bulk) import** — 24 qurilmaga sync uchun eng istiqbolli yo'l |
| `RecordFinder.startFind` / `.doFind` / `.doSeekFind` / `.getQuerySize` / `.stopFind` / `.destroy` | log o'qish |
| `AccessUser.list` / `.startFind` / `.doFind` / `.stopFind` | user o'qish |
| `FaceInfoManager.getCaps` | yuz algoritmi imkoniyatlari (`caps.RecognitionAlgorithm`) |
| `Attendance.webStatis`, `.addCheckGroup`, `.getWorkOrder` … | qurilmaning **ichki davomat** moduli |
| `accessControl.getDoorStatus`, `accessControl.factory` | eshik holati |

---

## 6. Adapter uchun tayyor shablonlar

```bash
# --- User sanash ---
GET /cgi-bin/recordFinder.cgi?action=getQuerySize&name=AccessControlCard
# -> Size=9801

# --- User o'qish (sahifalab) ---
GET /cgi-bin/recordFinder.cgi?action=doSeekFind&name=AccessControlCard&count=100&offset=0

# --- Log o'qish (sahifalab) ---
GET /cgi-bin/recordFinder.cgi?action=doSeekFind&name=AccessControlCardRec&count=100&offset=99900
#     DIQQAT: condition.StartTime / condition.EndTime E'TIBORGA OLINMAYDI.
#     Vaqt bo'yicha filtr ishlamaydi — offset + RecNo kursoridan foydalaning.

# --- Real-time event stream ---
GET /cgi-bin/snapManager.cgi?action=attachFileProc&Flags[0]=Event&Events=[AccessControl]&heartbeat=5
#     curl uchun -g (--globoff) SHART, aks holda [ ] glob deb qabul qilinadi.
#     Stream ochiq turganda qurilma port 80'ga yangi ulanish qabul qilmaydi.

# --- RTSP ---
rtsp://admin:PASS@IP:554/cam/realmonitor?channel=1&subtype=0   # 720p
rtsp://admin:PASS@IP:554/cam/realmonitor?channel=1&subtype=1   # VGA
```
