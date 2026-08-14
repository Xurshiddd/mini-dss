-- Kirish holati: doimiy yoki vaqtincha.
--
-- ⚠️ HEMIS'dan keladigan `status` (Ishlamoqda / O'qimoqda / ...) BOSHQA narsa —
-- u manbadagi holat. Bu yerdagi `access_status` esa BIZNING tizimda odamga
-- kirish berilganmi va qachongacha — shuni bildiradi.
--
-- Muddat `valid_to` da saqlanadi (u allaqachon bor va terminalga
-- `ValidDateEnd` sifatida yuboriladi).
--   access_status = 'permanent'  -> muddatsiz
--   access_status = 'temporary'  -> valid_to gacha; valid_to bo'sh bo'lsa cheksiz

ALTER TABLE people
    ADD COLUMN IF NOT EXISTS access_status varchar(16) NOT NULL DEFAULT 'permanent';

CREATE INDEX IF NOT EXISTS people_access_status_idx ON people (access_status);
-- Muddati o'tganlarni tez topish uchun.
CREATE INDEX IF NOT EXISTS people_valid_to_idx ON people (valid_to)
    WHERE valid_to IS NOT NULL;

-- Terminaldan import qilinganlarning turi 'unknown' edi — UI'da "Boshqa"
-- deb ko'rsatiladi, shuning uchun qiymatni ham shunga keltiramiz.
UPDATE people SET person_type = 'other' WHERE person_type = 'unknown';
