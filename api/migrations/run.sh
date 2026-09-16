#!/bin/sh
set -eu

psql -v ON_ERROR_STOP=1 -c \
  'CREATE TABLE IF NOT EXISTS public.schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())'

has_schema="$(psql -Atqc "SELECT to_regclass('public.devices') IS NOT NULL")"
applied="$(psql -Atqc 'SELECT count(*) FROM public.schema_migrations')"

# Mavjud o'rnatishlar 001–011 ni qo'lda olgan. Ularni baseline qilib,
# yangi migratsiyalarnigina bajaramiz.
if [ "$has_schema" = "t" ] && [ "$applied" = "0" ]; then
  for file in /migrations/00[1-9]_*.sql /migrations/01[01]_*.sql; do
    [ -f "$file" ] || continue
    version="$(basename "$file")"
    psql -v ON_ERROR_STOP=1 -c \
      "INSERT INTO public.schema_migrations(version) VALUES ('$version') ON CONFLICT DO NOTHING"
  done
fi

for file in /migrations/*.sql; do
  version="$(basename "$file")"
  done_before="$(psql -Atqc "SELECT EXISTS(SELECT 1 FROM public.schema_migrations WHERE version = '$version')")"
  [ "$done_before" = "t" ] && continue
  psql -v ON_ERROR_STOP=1 -f "$file"
  psql -v ON_ERROR_STOP=1 -c \
    "INSERT INTO public.schema_migrations(version) VALUES ('$version')"
done
