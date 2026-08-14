-- Ota bo'lim HEMIS'dan ko'pincha faqat ID bo'lib keladi va u bolasidan
-- KEYIN kelishi mumkin. Shu sababli havolani xom holda saqlaymiz va sync
-- oxirida `parent_id` ga aylantiramiz — aks holda tartib tufayli daraxt
-- bog'lanmay qoladi.
ALTER TABLE departments
    ADD COLUMN IF NOT EXISTS parent_external_id bigint;

CREATE INDEX IF NOT EXISTS departments_parent_external_idx
    ON departments (parent_external_id) WHERE parent_external_id IS NOT NULL;
