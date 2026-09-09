-- Rasm endi ALOHIDA jadvalda.
--
-- ⚠️ Sabab: qo'lda almashtirilgan rasm HEMIS oqimi tomonidan JIMGINA
-- yo'q qilinardi. `PeopleNeedingPhoto` "manbadagi URL o'zgardimi" degan
-- savolga `photo_metrics->>'source_url'` orqali javob berardi, qo'lda
-- yuklashda esa metrics'ga bu kalit yozilmasdi — ya'ni har safar "o'zgargan"
-- chiqardi. Ustiga-ustak ikkala oqim ham BIR XIL faylga (`<user_id>.jpg`)
-- yozardi, demak baytlar ham qaytarib bo'lmaydigan tarzda almashardi.
--
-- Yechim: har bir rasm o'z yozuvi va o'z fayli bilan saqlanadi, terminalga
-- ketadigani esa `people.photo_override_id` orqali ATAYLAB tanlanadi.
-- HEMIS oqimi bu jadvalga TEGMAYDI.
CREATE TABLE IF NOT EXISTS public.person_photos (
    id          bigserial PRIMARY KEY,
    person_id   bigint NOT NULL REFERENCES public.people (id) ON DELETE CASCADE,

    -- 'manual'   — operator yuklagan (fayl yoki kamera)
    -- 'terminal' — terminaldan olingan qoralamadan biriktirilgan
    -- 'hemis'    — manbadan olingan (hozircha people.photo_path da, zaxira uchun)
    source      varchar(16) NOT NULL,

    -- ⚠️ Faqat FAYL NOMI (yo'l emas). Yo'l qo'shilsa `filepath.Base` uni
    -- baribir kesib tashlaydi va ikki xil yozuv bir xil faylni ko'rsatib
    -- qolishi mumkin.
    file_name   varchar(255) NOT NULL,

    -- Manba manzili: HEMIS URL yoki `device://<id>/<UserID>`.
    source_url  text,

    status        varchar(16) NOT NULL DEFAULT 'pending',
    reject_reason text,
    metrics       json,

    -- Terminaldan olingan bo'lsa — qaysi terminaldan.
    device_id   bigint REFERENCES public.devices (id) ON DELETE SET NULL,

    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT person_photos_source_check
        CHECK (source IN ('manual', 'terminal', 'hemis')),
    CONSTRAINT person_photos_status_check
        CHECK (status IN ('pending', 'valid', 'rejected'))
);

CREATE INDEX IF NOT EXISTS person_photos_person_idx
    ON public.person_photos (person_id, id DESC);

-- Terminalga QAYSI rasm ketishi. NULL bo'lsa — `people.photo_path`, ya'ni
-- HEMIS oqimidagi rasm.
--
-- ⚠️ `ON DELETE SET NULL`: rasm yozuvi o'chirilsa odam HEMIS rasmiga
-- qaytadi, "rasmsiz" bo'lib qolmaydi.
ALTER TABLE public.people
    ADD COLUMN IF NOT EXISTS photo_override_id bigint
        REFERENCES public.person_photos (id) ON DELETE SET NULL;

-- Qisman indeks: almashtirilganlar kam, to'liq indeks keraksiz.
CREATE INDEX IF NOT EXISTS people_photo_override_idx
    ON public.people (photo_override_id) WHERE photo_override_id IS NOT NULL;
