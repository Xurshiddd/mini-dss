#!/usr/bin/env bash
#
# write-test.sh — YOZUV yo'lini aniqlash. TASK-08.
#
# ⚠️ Bu skript discovery skriptlaridan FARQ QILADI: u qurilmaga YOZADI.
#
# Nima qiladi:
#   1. TEST0001 degan soxta foydalanuvchi qo'shadi
#   2. Qo'shilganini o'qib tasdiqlaydi va RecNo sini oladi
#   3. Unga test rasmini yuklashga urinadi
#   4. Qo'shgan narsasini O'CHIRADI (xato bo'lsa ham — trap orqali)
#
# Nima QILMAYDI:
#   - mavjud foydalanuvchilarga tegmaydi
#   - eshikni ochmaydi
#   - config o'zgartirmaydi
#   - qurilmani qayta ishga tushirmaydi
#
# Ishlatish:
#   ./write-test.sh                 # .env dagi DAHUA_IP
#   ./write-test.sh 192.168.30.223
#
set -uo pipefail

cd "$(dirname "$0")"
set -a; . ./.env; set +a

IP="${1:-${DAHUA_IP}}"
USER="${DAHUA_USER}"
PASS="${DAHUA_PASS}"

TEST_USER_ID="${TEST_USER_ID:-TEST0001}"
OUT="raw/write-test-$(date +%Y%m%d-%H%M%S).txt"
mkdir -p raw

# -g (--globoff) SHART: `Doors[0]` ichidagi [ ] ni curl glob deb qabul qiladi.
CURL=(curl -s -g --digest -u "${USER}:${PASS}" -m 20)

RECNO=""
# RecNo faqat UserID=TEST0001 bo'lgan yozuvdan olinganda "tasdiqlangan"
# hisoblanadi. Tasdiqlanmagan RecNo HECH QACHON o'chirilmaydi — u real
# odamning yozuvi bo'lib chiqishi mumkin.
RECNO_CONFIRMED=0

# ---------------------------------------------------------------- tozalash
# Skript qanday tugashidan qat'i nazar (xato, Ctrl-C, muvaffaqiyat) —
# qo'shilgan test yozuvi o'chiriladi.
cleanup() {
  if [[ -z "$RECNO" ]]; then
    return
  fi

  if [[ "$RECNO_CONFIRMED" != "1" ]]; then
    echo
    echo "⛔ TOZALASH BAJARILMADI — RecNo=${RECNO} tasdiqlanmagan."
    echo "   Bu RecNo ${TEST_USER_ID} ga tegishli ekani ISBOTLANMADI, shuning"
    echo "   uchun o'chirmayman — real foydalanuvchini o'chirib yuborish xavfi bor."
    echo
    echo "   Qurilma web UI'sidan qo'lda tekshirib o'chiring:"
    echo "     UserID = ${TEST_USER_ID}"
    return
  fi

  echo
  echo "=== TOZALASH: RecNo=${RECNO} (${TEST_USER_ID}) o'chirilmoqda ==="
  local body
  body=$("${CURL[@]}" -w $'\n[HTTP %{http_code}]' \
    "http://${IP}/cgi-bin/recordUpdater.cgi?action=remove&name=AccessControlCard&recno=${RECNO}")
  echo "$body" | tee -a "$OUT"

  if grep -q '\[HTTP 200\]' <<<"$body"; then
    echo "✅ Tozalandi."
  else
    echo "⚠️  O'CHIRILMADI. Qurilma web UI'sidan qo'lda o'chiring:"
    echo "    UserID = ${TEST_USER_ID}, RecNo = ${RECNO}"
  fi
}
trap cleanup EXIT

# --------------------------------------------------------------- yordamchi
#
# ⚠️ MUHIM: `recordFinder.cgi` da `condition.*` parametrlari JIMGINA
# e'tiborga olinmaydi (discovery'da tasdiqlangan — DEVICE-REPORT.md 4-band).
# Ya'ni "UserID=TEST0001 ni topib ber" deb so'rasak ham, qurilma BIRINCHI
# yozuvlarni qaytaradi.
#
# Shu sababli RecNo ni javobdan ko'r-ko'rona olib bo'lmaydi — u REAL odamning
# RecNo si bo'lib chiqadi va tozalash uni o'chirib yuboradi.
#
# Quyidagi funksiya RecNo ni FAQAT UserID aynan mos kelgan yozuvdan oladi.
recno_of() {
  local want="$1"
  tr -d '\r' | awk -F= -v want="$want" '
    /^records\[[0-9]+\]\.UserID=/ { split($1, a, /[][]/); uid[a[2]] = $2 }
    /^records\[[0-9]+\]\.RecNo=/  { split($1, a, /[][]/); rec[a[2]] = $2 }
    END { for (i in uid) if (uid[i] == want) { print rec[i]; exit } }
  '
}

# So'rovni EKRANGA CHIQARADI, keyin yuboradi. Hech narsa yashirin emas.
req() {
  local label="$1" url="$2"
  echo
  echo "── ${label}"
  echo "   ${url}"
  local body
  body=$("${CURL[@]}" -w $'\n[HTTP %{http_code}]' "http://${IP}${url}")
  { echo "===== ${label} ====="; echo "http://${IP}${url}"; echo "$body"; echo; } >> "$OUT"
  echo "$body" | sed 's/^/   /'

  # Task qoidasi: 401 kelsa DARHOL to'xtaymiz, qayta urinmaymiz.
  if grep -q '\[HTTP 401\]' <<<"$body"; then
    echo
    echo "⛔ 401 — autentifikatsiya rad etildi. TO'XTATILDI."
    echo "   Dahua bir necha urinishdan keyin admin akkauntni bloklaydi."
    exit 1
  fi

  RESPONSE="$body"
}

echo "════════════════════════════════════════════════════════════"
echo " YOZUV TESTI — ${IP}"
echo " Test foydalanuvchi: ${TEST_USER_ID}"
echo " Natija: ${OUT}"
echo "════════════════════════════════════════════════════════════"

# --- 0. Boshlang'ich holat ------------------------------------------------
# Filtr ishlamagani uchun oxirgi yozuvlarni ko'ramiz: yangi qo'shilgan yozuv
# ro'yxatning OXIRIDA paydo bo'ladi.
req "0. Foydalanuvchilar soni" \
    "/cgi-bin/recordFinder.cgi?action=getQuerySize&name=AccessControlCard"
BEFORE_COUNT=$(grep -oE '^Size=[0-9]+' <<<"$RESPONSE" | head -1 | cut -d= -f2)
echo "   Boshlang'ich: ${BEFORE_COUNT} ta foydalanuvchi"
sleep 1

# --- 1. USER QO'SHISH ----------------------------------------------------
# Parametrlar qurilmadagi MAVJUD yozuvlardan nusxa olindi (device-probe/raw/04):
#   Doors[0]=0         — yagona eshik, indeks 0
#   TimeSections[0]=0  — yagona yoqilgan jadval (24/7)
#   CardStatus=0, CardType=0, UserType=0 — barcha mavjud yozuvlarda shunday
VALID_START="$(date +%Y-%m-%d) 00:00:00"
VALID_END="$(date -d '+1 day' +%Y-%m-%d 2>/dev/null || date -v+1d +%Y-%m-%d) 23:59:59"

# ⚠️ `CardNo` MAJBURIY. Usiz qurilma 400 qaytaradi — garchi mavjud 9801
# yozuvning hammasida CardNo qiymati BO'SH bo'lsa ham. Ya'ni qiymat bo'sh
# bo'lishi mumkin, lekin parametrning o'zi so'rovda bo'lishi shart.
# (write-test-variants.sh bilan aniqlangan, 2026-08-11)
req "1. USER QO'SHISH (recordUpdater.cgi insert)" \
    "/cgi-bin/recordUpdater.cgi?action=insert&name=AccessControlCard&UserID=${TEST_USER_ID}&CardName=WRITE%20TEST&CardNo=${TEST_USER_ID}&CardStatus=0&CardType=0&UserType=0&Doors[0]=0&TimeSections[0]=0&ValidDateStart=${VALID_START// /%20}&ValidDateEnd=${VALID_END// /%20}"

if ! grep -q '\[HTTP 200\]' <<<"$RESPONSE"; then
  echo
  echo "❌ XULOSA: recordUpdater.cgi?action=insert ISHLAMAYDI."
  echo "   Demak legacy CGI orqali yozib bo'lmaydi — RPC2 yo'li kerak."
  echo "   Sync engine RPC2 klienti ustiga quriladi."
  exit 0
fi

# Javobdan RecNo ni olamiz (odatda `recNo=13012` yoki `RecNo=13012`)
RECNO=$(grep -oiE 'recno[= ]+[0-9]+' <<<"$RESPONSE" | grep -oE '[0-9]+' | head -1)
echo
echo "   ✅ Qo'shildi. RecNo = ${RECNO:-<javobda yo\'q>}"
sleep 1

# --- 2. TASDIQLASH -------------------------------------------------------
# Filtr ishlamaydi → ro'yxat OXIRIDAN o'qiymiz. Yangi yozuv shu yerda bo'ladi.
TAIL_OFFSET=$(( BEFORE_COUNT > 5 ? BEFORE_COUNT - 5 : 0 ))
req "2. Haqiqatan qo'shildimi (ro'yxat oxiri, offset=${TAIL_OFFSET})" \
    "/cgi-bin/recordFinder.cgi?action=doSeekFind&name=AccessControlCard&count=10&offset=${TAIL_OFFSET}"

FOUND_RECNO=$(recno_of "$TEST_USER_ID" <<<"$RESPONSE")

if [[ -n "$FOUND_RECNO" ]]; then
  echo "   ✅ Qurilmada topildi. RecNo = ${FOUND_RECNO}"

  # insert javobidagi RecNo bilan solishtiramiz — farq qilsa, insert javobiga
  # ishonmaymiz va faqat UserID bo'yicha tasdiqlangan RecNo ni ishlatamiz.
  if [[ -n "$RECNO" && "$RECNO" != "$FOUND_RECNO" ]]; then
    echo "   ⚠️  insert javobidagi RecNo (${RECNO}) topilgan yozuvnikidan (${FOUND_RECNO}) farq qiladi."
    echo "       Tozalash uchun tasdiqlangani ishlatiladi."
  fi
  RECNO="$FOUND_RECNO"
  RECNO_CONFIRMED=1
else
  echo "   ⚠️  insert 200 qaytardi, lekin ${TEST_USER_ID} ro'yxat oxirida topilmadi."
  if [[ -n "$RECNO" ]]; then
    echo "       insert javobidagi RecNo=${RECNO} bilan o'chirishga urinamiz,"
    echo "       lekin o'chirishdan oldin u haqiqatan TEST yozuvi ekani tekshiriladi."
  fi
fi
sleep 1

# --- 3. YUZ YUKLASH ------------------------------------------------------
# Eng noaniq qadam. FaceInfoManager.cgi o'qish actionlariga 400 bergan edi,
# lekin qurilmaning web UI bundle'ida `FaceInfoManager` moduli bor (RPC2 da).
if [[ -f test-face.jpg ]]; then
  echo
  echo "── 3. YUZ YUKLASH (FaceInfoManager.cgi add)"
  B64=$(base64 -w0 test-face.jpg 2>/dev/null || base64 -i test-face.jpg | tr -d '\n')

  # ⚠️ JSON'ni `--data "$JSON"` bilan uzatib bo'lmaydi: base64 o'nlab KB
  # bo'lgani uchun "Argument list too long" xatosi chiqadi va so'rov
  # QURILMAGACHA YETIB BORMAYDI. Faylga yozib `--data @fayl` bilan uzatamiz.
  PAYLOAD=$(mktemp)
  printf '{"UserID":"%s","Info":{"UserID":"%s","PhotoData":["%s"]}}' \
         "$TEST_USER_ID" "$TEST_USER_ID" "$B64" > "$PAYLOAD"
  echo "   POST /cgi-bin/FaceInfoManager.cgi?action=add  (JSON $(wc -c < "$PAYLOAD") bayt)"

  FACE=$("${CURL[@]}" -X POST -H 'Content-Type: application/json' --data-binary "@${PAYLOAD}" \
        -w $'\n[HTTP %{http_code}]' \
        "http://${IP}/cgi-bin/FaceInfoManager.cgi?action=add")
  rm -f "$PAYLOAD"
  { echo "===== 3. FaceInfoManager add ====="; echo "$FACE"; echo; } >> "$OUT"
  echo "$FACE" | sed 's/^/   /'

  if grep -q '\[HTTP 200\]' <<<"$FACE"; then
    echo "   ✅ Yuz yuklandi — CGI yo'li ishlaydi."
  else
    echo "   ❌ Yuz yuklanmadi — RPC2 yo'li kerak bo'ladi."
  fi
else
  echo
  echo "── 3. YUZ YUKLASH — o'tkazib yuborildi"
  echo "   test-face.jpg fayli yo'q. Yuz testini ham qilish uchun shu papkaga"
  echo "   bitta portret rasm qo'ying (istalgan odam, terminalga saqlanmaydi —"
  echo "   test yozuvi bilan birga o'chiriladi)."
fi

echo
echo "════════════════════════════════════════════════════════════"
echo " Xom javoblar: ${OUT}"
echo "════════════════════════════════════════════════════════════"
# trap EXIT tozalashni bajaradi
