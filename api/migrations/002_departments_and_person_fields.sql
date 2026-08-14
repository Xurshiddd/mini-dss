-- HEMIS ma'lumot modeli: bo'limlar (ichma-ich) va odam maydonlari.
--
-- HEMIS'da bo'limlar daraxt ko'rinishida: fakultet → kafedra, markaz → bo'lim.
-- `parent_id` shuni ifodalaydi. Qo'lda ham bo'lim qo'shish mumkin, shuning
-- uchun `external_id` NULL bo'lishi mumkin.

CREATE TABLE IF NOT EXISTS departments (
    id           bigserial PRIMARY KEY,
    external_id  bigint,                    -- HEMIS department.id (qo'lda qo'shilganda NULL)
    code         varchar(64),               -- HEMIS department.code, masalan "331-298"
    name         varchar(255) NOT NULL,
    parent_id    bigint REFERENCES departments (id) ON DELETE SET NULL,
    kind         varchar(64),               -- structureType.name: Fakultet / Bo'lim / Markaz
    is_active    boolean NOT NULL DEFAULT true,
    created_at   timestamp(0) NOT NULL DEFAULT now(),
    updated_at   timestamp(0) NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS departments_external_id_key ON departments (external_id)
    WHERE external_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS departments_parent_idx ON departments (parent_id);

-- Yo'nalish/mutaxassislik (talabalar uchun) va lavozim (xodimlar uchun)
-- alohida jadval talab qilmaydi — ular odamda matn sifatida saqlanadi.
-- Bo'lim esa daraxt bo'lgani va filtrlashda ishlatilgani uchun jadvalda.

ALTER TABLE people
    ADD COLUMN IF NOT EXISTS person_type     varchar(16)  NOT NULL DEFAULT 'employee',
    ADD COLUMN IF NOT EXISTS department_id   bigint REFERENCES departments (id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS gender          varchar(16),
    ADD COLUMN IF NOT EXISTS birth_date      date,
    ADD COLUMN IF NOT EXISTS status          varchar(64),   -- Ishlamoqda / O'qimoqda / ...
    -- xodim
    ADD COLUMN IF NOT EXISTS staff_position  varchar(255),
    -- talaba
    ADD COLUMN IF NOT EXISTS specialty       varchar(255),
    ADD COLUMN IF NOT EXISTS student_group   varchar(64),
    ADD COLUMN IF NOT EXISTS level_name      varchar(64),
    ADD COLUMN IF NOT EXISTS education_form  varchar(64),
    ADD COLUMN IF NOT EXISTS education_type  varchar(64),
    ADD COLUMN IF NOT EXISTS payment_form    varchar(64);

CREATE INDEX IF NOT EXISTS people_department_idx ON people (department_id);
CREATE INDEX IF NOT EXISTS people_person_type_idx ON people (person_type);
CREATE INDEX IF NOT EXISTS people_status_idx ON people (status);

-- Terminaldan import qilinganlarning turi noma'lum — 'unknown' qilamiz,
-- aks holda ular xodim bo'lib ko'rinadi.
UPDATE people SET person_type = 'unknown' WHERE source = 'terminal';
UPDATE people SET person_type = 'student' WHERE source = 'elms_student';

-- sync_runs ga HEMIS bosqichini ham yozamiz; `device_id` NULL bo'lishi mumkin.
ALTER TABLE sync_runs
    ADD COLUMN IF NOT EXISTS created  integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS updated  integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS skipped  integer NOT NULL DEFAULT 0;
