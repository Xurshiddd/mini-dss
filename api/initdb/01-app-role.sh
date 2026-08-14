#!/bin/sh
# Ilova uchun NOSUPERUSER rol yaratadi (audit 1b-band).
#
# ⚠️ Postgres bu skriptni FAQAT bo'sh volume'da (birinchi init) ishga
# tushiradi. Mavjud bazada rol qo'lda yaratilgan — bu yangi o'rnatishlar
# uchun.
#
# Bootstrap superuser (POSTGRES_USER=minidss) COPY FROM PROGRAM orqali
# RCE'ga imkon beradi, shuning uchun ilova uni ISHLATMAYDI — cheklangan
# minidss_app roli bilan ulanadi.
set -e

if [ -z "$APP_DB_PASSWORD" ]; then
  echo "⚠️  APP_DB_PASSWORD berilmagan — ilova roli yaratilmadi" >&2
  exit 0
fi

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<SQL
CREATE ROLE minidss_app LOGIN PASSWORD '${APP_DB_PASSWORD}'
    NOSUPERUSER NOCREATEDB NOCREATEROLE;

GRANT CONNECT ON DATABASE ${POSTGRES_DB} TO minidss_app;
GRANT USAGE ON SCHEMA public TO minidss_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO minidss_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO minidss_app;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO minidss_app;

-- Kelajakda superuser yaratadigan jadval/sequence'larga ham avtomatik.
ALTER DEFAULT PRIVILEGES FOR ROLE ${POSTGRES_USER} IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO minidss_app;
ALTER DEFAULT PRIVILEGES FOR ROLE ${POSTGRES_USER} IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO minidss_app;
SQL

echo "✅ minidss_app roli yaratildi (NOSUPERUSER)"
