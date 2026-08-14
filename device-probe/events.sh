#!/usr/bin/env bash
#
# events.sh — real-time event stream sinovi (4.6). FAQAT O'QISH.
#
#   ./events.sh [ip] [sekund]
#
# DIQQAT: stream ochiq turganda qurilma port 80'ga YANGI HTTP ulanishlarni
# QABUL QILMAYDI ("connection refused"). Shu sababli probe.sh bilan bir vaqtda
# ishlatilmasin, va bir vaqtda faqat BITTA stream ochilsin.
#
set -euo pipefail
cd "$(dirname "$0")"
set -a; . ./.env; set +a

IP="${1:-${DAHUA_IP}}"
SECS="${2:-90}"
OUT="raw/${IP}"
mkdir -p "$OUT"

echo "=== event stream: ${IP}, ${SECS}s ==="

# -g (--globoff) SHART: Events=[AccessControl] ichidagi [ ] curl uchun glob.
curl -s -N -g -m "$SECS" --digest -u "${DAHUA_USER}:${DAHUA_PASS}" \
  -D "${OUT}/06-events-headers.txt" \
  "http://${IP}/cgi-bin/snapManager.cgi?action=attachFileProc&Flags[0]=Event&Events=[AccessControl]&heartbeat=5" \
  > "${OUT}/06-events.bin" 2> "${OUT}/06-events.err" || true

BYTES=$(wc -c < "${OUT}/06-events.bin")
echo "olindi: ${BYTES} bayt"
[[ "$BYTES" -eq 0 ]] && { echo "OGOHLANTIRISH: bo'sh. headers/err fayllarni tekshiring."; exit 1; }

echo "boundary:  $(grep -aoE 'boundary=[A-Za-z0-9_-]+' "${OUT}/06-events-headers.txt" | head -1)"
echo "hodisalar: $(grep -ac 'EventBaseInfo.Code=AccessControl' "${OUT}/06-events.bin")"
echo "heartbeat: $(grep -ac '^Heartbeat' "${OUT}/06-events.bin")"
echo "JPEG qism: $(grep -ac 'Content-Type: image/jpeg' "${OUT}/06-events.bin")"
echo
echo "maydonlar:"
grep -aoE 'Events\[0\]\.[A-Za-z0-9_.]+\[?[0-9]*\]?=' "${OUT}/06-events.bin" \
  | sed 's/Events\[0\]\.//; s/=$//' | sed -E 's/\[[0-9]+\]/[N]/' | sort -u | sed 's/^/  /'

echo
echo "ESLATMA: 06-events.bin ichida REAL ismlar va yuz fotolari bor."
echo "  .gitignore da turibdi — commit qilinmasin."
