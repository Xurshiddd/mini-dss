-- Terminaldan olingan yuz rasmlari — QORALAMA.
--
-- Nega kerak: eski DSS orqali kiritilgan xodimlarning rasmi faqat
-- terminalda qolgan, HEMIS'da esa ular uchun yuz yo'q yoki yaroqsiz rasm
-- keladi. Terminaldagi rasm — yagona nusxa. U shu jadvalga `UserID` va
-- terminaldagi ismi bilan tushadi, keyin kerakli odamga biriktiriladi.
--
-- ⚠️ Qoralama odamga BOG'LANMAGAN bo'lishi mumkin: terminalda bazada
-- umuman yo'q odamlar ham bor. Shuning uchun `person_id` NULL qabul qiladi
-- va kalit sifatida qurilmadagi `UserID` ishlatiladi.
CREATE TABLE IF NOT EXISTS public.photo_drafts (
    id          bigserial PRIMARY KEY,

    -- Terminaldagi UserID va CardName.
    user_id     varchar(32) NOT NULL,
    full_name   varchar(255) NOT NULL DEFAULT '',

    -- Qaysi terminaldan olingan.
    device_id   bigint REFERENCES public.devices (id) ON DELETE SET NULL,

    -- ⚠️ Faqat fayl nomi; fayl `<PHOTO_DIR>/drafts/` ichida yotadi.
    file_name   varchar(255) NOT NULL,

    status        varchar(16) NOT NULL DEFAULT 'pending',
    reject_reason text,
    metrics       json,

    -- Biriktirilgan bo'lsa — kimga va qachon.
    person_id   bigint REFERENCES public.people (id) ON DELETE SET NULL,
    attached_at timestamptz,

    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT photo_drafts_user_id_key UNIQUE (user_id),
    CONSTRAINT photo_drafts_status_check
        CHECK (status IN ('pending', 'valid', 'rejected'))
);

CREATE INDEX IF NOT EXISTS photo_drafts_person_idx
    ON public.photo_drafts (person_id) WHERE person_id IS NOT NULL;

-- ⚠️ Moslashtirish ism bo'yicha ham ketadi (terminaldagi UserID eski DSS
-- raqami bo'lishi mumkin va HEMIS ID'si bilan mos kelmaydi). Bunsiz har
-- biriktirishda 10 000 qatorli to'liq skanerlash bo'lardi.
CREATE INDEX IF NOT EXISTS photo_drafts_norm_name_idx
    ON public.photo_drafts (public.norm_name(full_name));
