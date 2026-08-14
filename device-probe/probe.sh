#!/usr/bin/env bash
#
# probe.sh — Dahua ASI7xxx terminal discovery (FAQAT O'QISH)
#
# Ishlatish:
#   ./probe.sh                    # .env dagi DAHUA_IP
#   ./probe.sh 192.168.30.223     # boshqa terminal
#   ./probe.sh 192.168.30.223 exit  # yo'nalish yorlig'i bilan (hisobot uchun)
#
# Natija: raw/<ip>/01..07 fayllari.
#
# QAT'IY: bu skript hech qanday yozuv operatsiyasini bajarmaydi.
# insert/update/remove/add/delete/clear/setConfig/openDoor/reboot — YO'Q.
#
set -euo pipefail

cd "$(dirname "$0")"

# ---- konfiguratsiya -------------------------------------------------------
if [[ ! -f .env ]]; then
  echo "XATO: .env topilmadi. Namuna:" >&2
  echo "  DAHUA_IP=192.168.30.222" >&2
  echo "  DAHUA_USER=admin" >&2
  echo "  DAHUA_PASS=..." >&2
  exit 1
fi
set -a; . ./.env; set +a

IP="${1:-${DAHUA_IP:?DAHUA_IP kerak}}"
LABEL="${2:-}"
USER="${DAHUA_USER:?DAHUA_USER kerak}"
PASS="${DAHUA_PASS:?DAHUA_PASS kerak}"

OUT="raw/${IP}"
mkdir -p "$OUT"

# Rate limit: Dahua ko'p so'rovda javob bermay qo'yadi.
RATE_SLEEP="${RATE_SLEEP:-1}"

# ---- yordamchilar ---------------------------------------------------------

# MUHIM: -g (--globoff) shart. curl `[` va `]` ni glob deb qabul qiladi va
# Events=[AccessControl] kabi URL'lar exit=3 (URL malformat) bilan yiqiladi.
CURL=(curl -s -g --digest -u "${USER}:${PASS}")

# get <fayl> <endpoint>  — bitta CGI chaqiruvi, xom javob faylga qo'shiladi
get() {
  local file="$1" ep="$2"
  { echo "===== /cgi-bin/${ep} ====="
    "${CURL[@]}" -m 20 -w "\n[HTTP %{http_code}]\n" "http://${IP}/cgi-bin/${ep}" || true
    echo
  } >> "${OUT}/${file}"
  sleep "$RATE_SLEEP"
}

section() { echo; echo "### $*"; }

echo "=== probe.sh: ${IP} ${LABEL:+(${LABEL})} — $(date '+%F %T %z') ==="

# ---- 4.0 tarmoq -----------------------------------------------------------
section "4.0 tarmoq"
: > "${OUT}/00-network.txt"
{
  echo "probe vaqti (server): $(date '+%F %T %z')"
  echo "server UTC:           $(date -u '+%F %T') UTC"
} >> "${OUT}/00-network.txt"
for port in 80 554; do
  if (exec 3<>"/dev/tcp/${IP}/${port}") 2>/dev/null; then
    echo "port ${port}: OCHIQ" | tee -a "${OUT}/00-network.txt"
  else
    echo "port ${port}: YOPIQ" | tee -a "${OUT}/00-network.txt"
  fi
done

# ---- 4.1 identifikatsiya --------------------------------------------------
section "4.1 identifikatsiya"
: > "${OUT}/01-identity.txt"
for a in getDeviceType getSoftwareVersion getHardwareVersion getSystemInfo getSerialNo getVendor; do
  get 01-identity.txt "magicBox.cgi?action=${a}"
done
grep -E '^(type|version|deviceType|serialNumber|processor)=' "${OUT}/01-identity.txt" || true

# ---- 4.2 API avlodi -------------------------------------------------------
section "4.2 API avlodi"
: > "${OUT}/02-api-generation.txt"
# Yangi (Web 5.0) CGI to'plami — bu firmware'da kutilmaydi
for ep in "AccessUser.cgi?action=count" "AccessFace.cgi?action=count" "AccessCard.cgi?action=count"; do
  get 02-api-generation.txt "$ep"
done
# Legacy (Web 3.0) to'plami — kutilgan ishlaydigan yo'l
for n in AccessControlCard AccessControlCardRec; do
  get 02-api-generation.txt "recordFinder.cgi?action=getQuerySize&name=${n}"
done
# Kontrol: mavjud bo'lmagan CGI ham 400 qaytaradi — 400 o'zi dalil emas
get 02-api-generation.txt "zzzNotExist.cgi?action=count"
grep -E '^=====|^Size=|^\[HTTP' "${OUT}/02-api-generation.txt" || true

# ---- 4.3 konfiguratsiya ---------------------------------------------------
section "4.3 konfiguratsiya"
: > "${OUT}/03-config.txt"
echo "SERVER_TIME=$(date '+%F %T %z') / UTC=$(date -u '+%F %T')" >> "${OUT}/03-config.txt"
get 03-config.txt "global.cgi?action=getCurrentTime"
for n in NTP Locales AccessControl AccessControlGeneral AccessTimeSchedule VideoAnalyseRule; do
  get 03-config.txt "configManager.cgi?action=getConfig&name=${n}"
done
# To'liq dump — alohida faylga (kattaligi ~650 KB)
"${CURL[@]}" -m 60 -o "${OUT}/03b-fullconfig.txt" "http://${IP}/cgi-bin/configManager.cgi?action=getConfig&name=All" || true
sleep "$RATE_SLEEP"
{
  echo "--- muhim qiymatlar ---"
  grep -E 'NTP\.(Enable|TimeZone)' "${OUT}/03-config.txt" || true
  grep -E 'AccessControl\[[0-9]+\]\.Name=' "${OUT}/03-config.txt" || true
  grep -E 'AccessTimeSchedule\[[0-9]+\]\.Enable=true' "${OUT}/03-config.txt" || true
} | tee -a "${OUT}/03-config.txt"

# ---- 4.4 foydalanuvchilar -------------------------------------------------
section "4.4 foydalanuvchilar"
: > "${OUT}/04-users-sample.txt"
get 04-users-sample.txt "recordFinder.cgi?action=getQuerySize&name=AccessControlCard"
get 04-users-sample.txt "recordFinder.cgi?action=doSeekFind&name=AccessControlCard&count=3"
# UserID format namunasi — turli offsetlardan
for off in 0 2000 5000 8000; do
  get 04-users-sample.txt "recordFinder.cgi?action=doSeekFind&name=AccessControlCard&count=5&offset=${off}"
done
echo "UserID shablonlari (9=raqam, A=harf):"
grep -aoE 'UserID=[^ ]+' "${OUT}/04-users-sample.txt" | sed 's/UserID=//' | tr -d '\r' \
  | awk 'NF{p=$0; gsub(/[0-9]/,"9",p); gsub(/[A-Za-z]/,"A",p); print length($0)"  "p}' \
  | sort | uniq -c | sort -rn

# ---- 4.5 loglar -----------------------------------------------------------
section "4.5 loglar"
: > "${OUT}/05-records.txt"
get 05-records.txt "recordFinder.cgi?action=getQuerySize&name=AccessControlCardRec"
get 05-records.txt "recordFinder.cgi?action=doSeekFind&name=AccessControlCardRec&count=3"
# Eng yangi yozuvlar: buffer 100000 ta, oxiridan olamiz
SIZE=$(grep -aoE '^Size=[0-9]+' "${OUT}/05-records.txt" | head -1 | cut -d= -f2 || echo 0)
if [[ "${SIZE:-0}" -gt 5 ]]; then
  get 05-records.txt "recordFinder.cgi?action=doSeekFind&name=AccessControlCardRec&count=5&offset=$((SIZE-5))"
fi
echo "eng eski / eng yangi CreateTime:"
grep -aoE 'CreateTime=[0-9]+' "${OUT}/05-records.txt" | sed 's/CreateTime=//' \
  | sort -n | sed -n '1p;$p' | while read -r t; do echo "  $t = $(date -u -d "@$t" '+%F %T') UTC"; done

echo
echo "=== tugadi: ${OUT}/ ==="
echo "ESLATMA: 4.6 (event stream) va 4.7 (RTSP) alohida ishga tushiriladi —"
echo "  event stream ochiq turganda qurilma port 80'ga YANGI ulanish qabul qilmaydi."
echo "  ./events.sh ${IP} 90     — event stream"
echo "  ./rtsp.sh   ${IP}        — RTSP tekshiruvi"
