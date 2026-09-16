# Mini-DSS

Terminallar (Dahua ASI7213Y) uchun kirish nazorati platformasi: HEMIS'dan
odamlarni oladi, rasmini tekshiradi, terminallarga yozadi va kirish
yozuvlarini o'qiydi.

```
caddy      HTTPS reverse proxy     → :443, :80  (yagona tashqi kirish)
web/       Nuxt admin panel        → Caddy ortida (/)
api/       Go API                  → Caddy ortida (/api/*), lokal :6081
face-api/  InsightFace validator   → lokal :6001
postgres                           → lokal :6432
ntp        terminallar soati uchun → :123/udp
```

⚠️ Kirish: `https://<server-ip>` (ichki CA — brauzer ogohlantiradi).
Brauzer API'ga NISBIY yo'l bilan boradi (`/api/*`), same-origin. Tashqi
portlar faqat Caddy'da; qolgani `127.0.0.1`. Batafsil: TASK-02.

## Kod uslubi

Go kodi uchun [style.md](style.md) — Uber Go Style Guide. Yangi kod shunga
mos yozilsin, mavjud kod tegilganda shu tomonga tortilsin.

Loyihaning o'z qo'shimchalari:

- **Izohlar o'zbekcha.** Kod ingliz tilida, izohlar o'zbekcha — jamoa shunday
  o'qiydi.
- **`⚠️` faqat qimmatga tushgan bilim uchun.** Tajriba orqali topilgan,
  hujjatda yozilmagan qurilma xatti-harakatlari shu belgi bilan izohlanadi
  (masalan: `CardNo` majburiy, vaqt filtri jimgina ishlamaydi). Oddiy izohga
  bu belgi qo'yilmaydi — aks holda ogohlantirish qiymatini yo'qotadi.
- **`⛔` qaytarib bo'lmaydigan amallar uchun** (`ClearAllUsers`, `Wipe`).

## Qurilma bilan ishlashda bilish shart

- API avlodi **legacy (Web 3.0)**: `recordFinder.cgi` / `recordUpdater.cgi` /
  `FaceInfoManager.cgi`. `AccessUser.getMulti` YO'Q ("Method not found"),
  garchi `system.listService` uni ko'rsatsa ham.
- **Metodni taxmin qilmang — so'rang.** `<service>.listMethod` (masalan
  `AccessFace.listMethod`) qurilmadagi HAQIQIY ro'yxatni beradi.
  ⚠️ `system.listMethod` esa `name`/`service` parametrini jimgina e'tiborsiz
  qoldiradi va doim `system` ning o'z metodlarini qaytaradi — shu sababli
  `AccessFace.*` "yo'q" deb noto'g'ri xulosa qilingan edi. U BOR:
  `list`, `startFind`/`doFind`/`stopFind`, `insertMulti`, `removeMulti`.
- **Yozish o'qishdan ancha og'ir va CGI bilan XAVFLI.** Qurilma digest
  challenge'ida ulanishni yopadi (`Connection: close`), shuning uchun HAR BIR
  CGI so'rovi 3 TA yangi TCP ulanish ochadi. ~2500 odamdan keyin terminal
  yangi ulanish qabul qilmay qo'yadi (dial timeout) va SOATLAB o'ziga
  kelmaydi — o'lchangan (2026-09-16): 2509 odam yozilgach qolgan 718 tasi
  2 soat davomida 10 soniyadan "failed" bo'lib chiqdi.
  Yozish RPC2 orqali ketsin: sessiya ulanishi qayta ishlatiladi.
- **Qurilma yozish SUR'ATINI cheklaydi:** "MAX INSERT RATE EXCEEDED" — bu
  yozuvning aybi emas, biroz kutib qayta urinilsa o'tadi.
- **Yuz YOZISH ham RPC2 da** (jonli terminalda o'lchangan, 2026-09-16):

      AccessFace.insertMulti {FaceList:[{UserID, PhotoData:[b64]}]}  → 0.45 s
      AccessFace.updateMulti {FaceList:[...]}   mavjud yuz ustiga     → 0.29 s
      AccessFace.removeMulti {UserIDList:[...]}                       → 0.07 s

  ⚠️ Yozishda parametr `FaceList`, O'QISHDA esa javob `FaceDataList` —
  yozishga `FaceDataList` berilsa "Request invalid param!" keladi.
  ⚠️ `insertMulti` MAVJUD yuz ustiga yozmaydi ("Batch Process Error") —
  almashtirish uchun `updateMulti`.
  ⚠️ `FaceInfoManager.add` RPC2 da bor, lekin ishlamaydi: foydalanuvchi
  qurilmada bo'lsa ham `faceInfoManagerErrorUserNotExist` qaytaradi.
  ⚠️ To'da bo'lib yozish deyarli foyda bermaydi (1 ta → 469 ms/yuz,
  4 talik to'da → 402 ms/yuz): vaqt tarmoqda emas, qurilmaning yuzdan
  belgi ajratishida ketadi. Eski CGI yo'li (`FaceInfoManager.cgi?action=add`)
  0.75 s va yuqoridagi ulanish halokatiga olib keladi.
- **Yuzni qaytib o'qish RPC2 da, CGI da EMAS.** Jonli terminalda
  tasdiqlangan (2026-09-09):

      FaceInfoManager.get {UserID}       → info.PhotoData[0]  (bitta)
      AccessFace.list    {UserIDList}    → FaceDataList[]     (to'da)
      AccessFace.startFind/doFind/stopFind → yuzi BOR odamlar ro'yxati

  ⚠️ `FaceInfoManager.cgi` da o'qish YO'Q: `find`, `get`, `getFace`, `list`,
  `getCaps`, `export` — GET ham, JSON POST ham HTTP 400 beradi, garchi shu
  CGI'ning `add` va `remove` action'lari ishlasa ham.
  ⚠️ Harf registri xizmatlar orasida FARQ QILADI:
  `FaceInfoManager.doFind` → `token`/`offset`/`count`, javob `info`;
  `AccessFace.doFind` → `Token`/`Offset`/`Count`, javob `Info`.
  ⚠️ `startFind` ning `condition` parametri e'tiborsiz qoladi — filtrlash
  yo'q, faqat sahifalash.
  Shunga qaramay `face_synced = true` hamon faqat "HTTP OK keldi" degani;
  haqiqiy holatni `GET /api/devices/{id}/face-probe?user_id=X` ko'rsatadi.
- **Rasm manbai muhim.** `people.photo_path` — faqat HEMIS oqimi.
  Qo'lda qo'yilgan va terminaldan olingan rasmlar `person_photos` da va
  `people.photo_override_id` orqali tanlanadi; HEMIS oqimi ularga TEGMAYDI.
- **Yo'nalish QURILMADAN aniqlanadi** (`devices.direction`), hodisadagi
  `Type` maydonidan emas.
- Qurilma vaqti **lokal**, loglardagi `CreateTime` esa **epoch UTC**.

Batafsil: [ARCHITECTURE.md](ARCHITECTURE.md),
[device-probe/DEVICE-REPORT.md](device-probe/DEVICE-REPORT.md),
[device-probe/API-MATRIX.md](device-probe/API-MATRIX.md).
