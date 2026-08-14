#!/usr/bin/env bash
#
# rtsp.sh — RTSP oqim tekshiruvi (4.7). FAQAT O'QISH.
#
#   ./rtsp.sh [ip]
#
# ffprobe kerak. Windows: winget install Gyan.FFmpeg
#
set -euo pipefail
cd "$(dirname "$0")"
set -a; . ./.env; set +a

IP="${1:-${DAHUA_IP}}"
OUT="raw/${IP}"
mkdir -p "$OUT"

FFPROBE="${FFPROBE:-ffprobe}"
if ! command -v "$FFPROBE" >/dev/null 2>&1; then
  # winget o'rnatgan joyni qidiramiz (PATH yangilanmagan bo'lishi mumkin)
  FFPROBE=$(find "${LOCALAPPDATA:-/c/Users/$USER/AppData/Local}/Microsoft/WinGet" \
            -name ffprobe.exe 2>/dev/null | head -1 || true)
  [[ -z "$FFPROBE" ]] && { echo "XATO: ffprobe topilmadi. winget install Gyan.FFmpeg"; exit 1; }
fi

: > "${OUT}/07-rtsp.txt"
for st in 0 1; do
  echo "===== subtype=${st} =====" | tee -a "${OUT}/07-rtsp.txt"
  "$FFPROBE" -rtsp_transport tcp -v error -show_streams -show_format \
    -of default=noprint_wrappers=1 \
    "rtsp://${DAHUA_USER}:${DAHUA_PASS}@${IP}:554/cam/realmonitor?channel=1&subtype=${st}" 2>&1 \
    | grep -iE 'codec_name|codec_type|width=|height=|avg_frame_rate|bit_rate|profile|format_name|error|denied' \
    | tee -a "${OUT}/07-rtsp.txt"
  echo | tee -a "${OUT}/07-rtsp.txt"
  sleep 1
done
