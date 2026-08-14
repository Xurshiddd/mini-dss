-- Jonli hodisa oqimi uchun.
--
-- ⚠️ Oqimdan kelgan hodisada `RecNo` YO'Q — u faqat qurilma jurnalida
-- bo'ladi. Shuning uchun ustun NULL qabul qiladigan bo'ladi.
ALTER TABLE public.access_events ALTER COLUMN rec_no DROP NOT NULL;

-- ⚠️ `(device_id, user_id, happened_at)` UNIKAL EMAS: qurilma bitta odamni
-- bir soniya ichida ikki marta yozishi mumkin (real ma'lumotda uchradi).
-- Shuning uchun bu yerda unikal indeks emas, oddiy indeks — u faqat
-- "shu hodisa allaqachon bormi" tekshiruvini tezlashtiradi. Takrorlanishning
-- oldini olish INSERT ichidagi `NOT EXISTS` sharti bilan qilinadi.
CREATE INDEX IF NOT EXISTS access_events_dedup_idx
    ON public.access_events (device_id, user_id, happened_at);
