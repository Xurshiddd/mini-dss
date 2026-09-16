-- Terminaldagi karta ma'lumoti qaysi versiyada yozilganini kuzatish.
ALTER TABLE public.device_person_sync
    ADD COLUMN IF NOT EXISTS user_hash text;

-- Logout qilingan JWT'lar restartdan keyin qayta tirilmasligi uchun.
CREATE TABLE IF NOT EXISTS public.revoked_tokens (
    token_hash text PRIMARY KEY,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS revoked_tokens_expires_at_idx
    ON public.revoked_tokens (expires_at);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.revoked_tokens TO minidss_app;
