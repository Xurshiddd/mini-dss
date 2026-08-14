-- Kirish/chiqish hodisalari.
--
-- Nega bazada saqlanadi: qurilmadagi log — 100 000 yozuvli halqa bufer,
-- eskisi ustiga yozib boriladi. Hisobot uchun har safar qurilmaga borish
-- ham sekin (sahifalab o'qish), ham ishonchsiz (eski yozuv yo'qolgan
-- bo'lishi mumkin).
CREATE TABLE IF NOT EXISTS public.access_events (
    id            bigserial PRIMARY KEY,
    device_id     bigint NOT NULL REFERENCES public.devices(id) ON DELETE CASCADE,

    -- ⚠️ Dedup kaliti. `RecNo` qurilmada monoton o'sadi va takrorlanmaydi —
    -- shuning uchun bir hodisa ikki marta yozilmasligiga kafolat beradi.
    rec_no        bigint NOT NULL,

    -- Qurilma bilgan ID. Odam keyin o'chirilsa ham hodisa qoladi.
    user_id       varchar(32) NOT NULL,
    person_id     bigint REFERENCES public.people(id) ON DELETE SET NULL,

    -- ⚠️ timestamptz: qurilma logidagi `CreateTime` — epoch UTC, hisobot esa
    -- Toshkent kuni bo'yicha. Zonani bazaning o'zi hisoblasin.
    happened_at   timestamptz NOT NULL,

    -- ⚠️ QURILMADAN olinadi (`devices.direction`), hodisadagi `Type`
    -- maydonidan EMAS — u ishonchsiz.
    direction     varchar(8) NOT NULL,

    status        smallint NOT NULL,   -- 1 = tanidi, 0 = tanimadi
    error_code    smallint,
    method        smallint,            -- 15 = yuz
    snapshot_url  text,                -- qurilmadagi surat yo'li

    created_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT access_events_device_recno_key UNIQUE (device_id, rec_no)
);

-- Hisobot so'rovlari: "shu odam, shu oraliqda".
CREATE INDEX IF NOT EXISTS access_events_person_time_idx
    ON public.access_events (person_id, happened_at)
    WHERE person_id IS NOT NULL AND status = 1;

-- Kunlik hisobot: "shu kunda kim bo'lgan".
CREATE INDEX IF NOT EXISTS access_events_time_idx
    ON public.access_events (happened_at);

-- Yig'ishni qayerdan davom ettirishni bilish uchun.
CREATE INDEX IF NOT EXISTS access_events_device_recno_idx
    ON public.access_events (device_id, rec_no DESC);
