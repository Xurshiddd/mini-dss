-- Odamni o'chirish IKKI BOSQICHLI.
--
-- Sabab: qurilma o'chiq yoki tarmoqda bo'lmasligi mumkin. Agar odamni
-- bazadan darhol o'chirsak, `device_person_sync` yozuvi ham yo'qoladi va
-- uni terminaldan olib tashlashning iloji QOLMAYDI — odam bazada yo'q,
-- lekin eshikni ochaveradi va buni hech kim sezmaydi.
--
-- Shuning uchun: avval `pending_delete = true` belgilanadi, sync har bir
-- terminaldan olib tashlaydi, hammasidan tozalangach yozuv bazadan o'chadi
-- (`PurgeDeletedPeople`).
ALTER TABLE public.people
    ADD COLUMN IF NOT EXISTS pending_delete boolean DEFAULT false NOT NULL;

-- Qisman indeks: belgilanganlar juda kam, to'liq indeks keraksiz.
CREATE INDEX IF NOT EXISTS people_pending_delete_idx
    ON public.people (pending_delete) WHERE pending_delete;
