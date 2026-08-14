#!/usr/bin/env bash
#
# write-test-variants.sh — `insert` ning turli parametr variantlarini sinaydi.
#
# Nima uchun: `recordUpdater.cgi?action=insert` 400 qaytardi, lekin discovery'da
# aniqlanganidek Dahua 400 ni HAM "bunday CGI yo'q", HAM "parametr noto'g'ri"
# uchun qaytaradi. Ya'ni bitta urinish yetarli emas — insert umuman yo'qmi
# yoki men parametrni xato berdimmi, shuni ajratish kerak.
#
# Xavfsizlik: har urinishdan keyin foydalanuvchilar SONI tekshiriladi.
# Son o'zgarsa — variant ISHLADI, darhol to'xtab tozalaymiz.
#
set -uo pipefail
cd "$(dirname "$0")"
set -a; . ./.env; set +a

IP="${1:-${DAHUA_IP}}"
TEST_USER_ID="${TEST_USER_ID:-TEST0001}"
OUT="raw/write-variants-$(date +%Y%m%d-%H%M%S).txt"
mkdir -p raw

CURL=(curl -s -g --digest -u "${DAHUA_USER}:${DAHUA_PASS}" -m 20)

count_users() {
  "${CURL[@]}" "http://${IP}/cgi-bin/recordFinder.cgi?action=getQuerySize&name=AccessControlCard" \
    | grep -oE '^Size=[0-9]+' | head -1 | cut -d= -f2
}

recno_of() {
  local want="$1"
  tr -d '\r' | awk -F= -v want="$want" '
    /^records\[[0-9]+\]\.UserID=/ { split($1, a, /[][]/); uid[a[2]] = $2 }
    /^records\[[0-9]+\]\.RecNo=/  { split($1, a, /[][]/); rec[a[2]] = $2 }
    END { for (i in uid) if (uid[i] == want) { print rec[i]; exit } }
  '
}

# Muvaffaqiyatli variant topilsa — o'sha yozuvni topib o'chiramiz.
cleanup_test_user() {
  local total offset body recno
  total=$(count_users)
  offset=$(( total > 10 ? total - 10 : 0 ))
  body=$("${CURL[@]}" "http://${IP}/cgi-bin/recordFinder.cgi?action=doSeekFind&name=AccessControlCard&count=15&offset=${offset}")
  recno=$(recno_of "$TEST_USER_ID" <<<"$body")

  if [[ -z "$recno" ]]; then
    echo "⚠️  ${TEST_USER_ID} topilmadi — qo'lda tekshiring!"
    return
  fi

  echo "Tozalash: RecNo=${recno}"
  "${CURL[@]}" -w $'\n[HTTP %{http_code}]' \
    "http://${IP}/cgi-bin/recordUpdater.cgi?action=remove&name=AccessControlCard&recno=${recno}" \
    | tee -a "$OUT"

  if [[ "$(count_users)" == "$BASE_COUNT" ]]; then
    echo "✅ Tozalandi, son boshlang'ich holatga qaytdi (${BASE_COUNT})."
  else
    echo "⚠️  Son qaytmadi! Qo'lda tekshiring: UserID=${TEST_USER_ID}"
  fi
}

BASE_COUNT=$(count_users)
echo "Boshlang'ich foydalanuvchilar soni: ${BASE_COUNT}"
echo "Natija: ${OUT}"
echo

BASE="action=insert&name=AccessControlCard"
COMMON="UserID=${TEST_USER_ID}&CardName=WRITETEST&CardStatus=0&CardType=0&UserType=0"

# Har bir variant: "tavsif|HTTP metodi|query"
VARIANTS=(
  "A. asosiy (GET)|GET|${BASE}&${COMMON}&Doors[0]=0&TimeSections[0]=0"
  "B. POST|POST|${BASE}&${COMMON}&Doors[0]=0&TimeSections[0]=0"
  "C. CardNo bilan|GET|${BASE}&${COMMON}&CardNo=${TEST_USER_ID}&Doors[0]=0&TimeSections[0]=0"
  "D. TimeSections=255|GET|${BASE}&${COMMON}&Doors[0]=0&TimeSections[0]=255"
  "E. Doors/TimeSections'siz|GET|${BASE}&${COMMON}"
  "F. amal muddati bilan|GET|${BASE}&${COMMON}&Doors[0]=0&TimeSections[0]=0&ValidDateStart=2026-08-11%2000:00:00&ValidDateEnd=2027-08-11%2023:59:59"
  "G. faqat UserID+CardName|GET|${BASE}&UserID=${TEST_USER_ID}&CardName=WRITETEST"
)

for v in "${VARIANTS[@]}"; do
  IFS='|' read -r label method query <<<"$v"

  echo "── ${label}"
  echo "   ${method} /cgi-bin/recordUpdater.cgi?${query}"

  if [[ "$method" == "POST" ]]; then
    body=$("${CURL[@]}" -X POST -w $'\n[HTTP %{http_code}]' \
           "http://${IP}/cgi-bin/recordUpdater.cgi?${query}")
  else
    body=$("${CURL[@]}" -w $'\n[HTTP %{http_code}]' \
           "http://${IP}/cgi-bin/recordUpdater.cgi?${query}")
  fi

  { echo "===== ${label} ====="; echo "${method} ${query}"; echo "$body"; echo; } >> "$OUT"
  echo "$body" | sed 's/^/   /'

  if grep -q '\[HTTP 401\]' <<<"$body"; then
    echo "⛔ 401 — TO'XTATILDI."
    exit 1
  fi

  sleep 1
  now=$(count_users)

  if [[ "$now" != "$BASE_COUNT" ]]; then
    echo
    echo "🎉 ISHLADI — foydalanuvchilar soni ${BASE_COUNT} → ${now}"
    echo "   Ishlaydigan variant: ${label}"
    echo "   ${method} /cgi-bin/recordUpdater.cgi?${query}"
    echo
    cleanup_test_user
    exit 0
  fi

  echo "   (son o'zgarmadi: ${now})"
  sleep 1
done

echo
echo "════════════════════════════════════════════════════════════"
echo " Hamma variant muvaffaqiyatsiz — son ${BASE_COUNT} da qoldi."
echo " Xulosa: recordUpdater.cgi bu firmware'da YOZUVNI QABUL QILMAYDI."
echo " Sync engine RPC2 klienti ustiga qurilishi kerak."
echo "════════════════════════════════════════════════════════════"
