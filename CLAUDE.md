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
  `FaceInfoManager.cgi`. `AccessFace.*` va `AccessUser.getMulti` YO'Q
  ("Method not found"), garchi `system.listService` ularni ko'rsatsa ham.
- **Yozish o'qishdan ancha og'ir.** Ketma-ket yozuvda ~90 so'rovdan keyin
  qurilma javob bermay qo'yishi mumkin. Oraliq ≥700 ms, backoff bilan.
- **Yuzni qaytib o'qib bo'lmaydi.** `face_synced = true` faqat "HTTP OK keldi"
  degani. Yuz holatini tekshirishning yagona yo'li — kirish loglari
  (`GET /api/devices/{id}/events`).
- **Yo'nalish QURILMADAN aniqlanadi** (`devices.direction`), hodisadagi
  `Type` maydonidan emas.
- Qurilma vaqti **lokal**, loglardagi `CreateTime` esa **epoch UTC**.

Batafsil: [ARCHITECTURE.md](ARCHITECTURE.md),
[device-probe/DEVICE-REPORT.md](device-probe/DEVICE-REPORT.md),
[device-probe/API-MATRIX.md](device-probe/API-MATRIX.md).
