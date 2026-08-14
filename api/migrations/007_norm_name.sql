-- Ismni solishtirish uchun normallashtirish.
--
-- HEMIS bitta odamni ikki modulda ham qaytaradi (o'qiyotgan xodimlar):
-- xodim sifatida bitta ID bilan, talaba sifatida BOSHQA ID bilan. Umumiy
-- kalit yo'q — yagona bog'lovchi belgi bu ism.
--
-- ⚠️ Manbalar ismni har xil yozadi: "O‘G‘LI" va "o g li", katta-kichik harf,
-- apostroflar. Shuning uchun harfdan boshqa hamma narsa olib tashlanadi.
CREATE OR REPLACE FUNCTION public.norm_name(text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$ SELECT lower(regexp_replace($1, '[^a-zA-Z]', '', 'g')) $$;

-- Bunsiz har sync'da 8 000+ qator bo'yicha to'liq skanerlash bo'lardi.
CREATE INDEX IF NOT EXISTS people_norm_name_idx
    ON public.people (public.norm_name(full_name));
