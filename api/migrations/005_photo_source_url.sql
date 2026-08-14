-- Manbadagi rasm manzili. Rasm yuklash alohida bosqichda ketadi, shuning
-- uchun URL saqlanishi kerak — sync paytida darhol yuklab bo'lmaydi
-- (8000+ rasm ketma-ket yuklansa soatlab vaqt ketadi).
ALTER TABLE people ADD COLUMN IF NOT EXISTS photo_source_url text;
