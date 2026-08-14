\restrict rGQ3naz0uTXcR56jxw7lYOCeMVptUVx02HwpdxhnLhtWzmLrmyUuG33qQ4PrMnE
SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;
SET default_tablespace = '';
SET default_table_access_method = heap;
CREATE TABLE public.cache (
    key character varying(255) NOT NULL,
    value text NOT NULL,
    expiration bigint NOT NULL
);
CREATE TABLE public.cache_locks (
    key character varying(255) NOT NULL,
    owner character varying(255) NOT NULL,
    expiration bigint NOT NULL
);
CREATE TABLE public.device_person_sync (
    id bigint NOT NULL,
    device_id bigint NOT NULL,
    person_id bigint NOT NULL,
    state character varying(255) DEFAULT 'pending'::character varying NOT NULL,
    device_recno bigint,
    face_synced boolean DEFAULT false NOT NULL,
    last_error text,
    attempts smallint DEFAULT '0'::smallint NOT NULL,
    synced_at timestamp(0) without time zone,
    created_at timestamp(0) without time zone,
    updated_at timestamp(0) without time zone,
    CONSTRAINT device_person_sync_state_check CHECK (((state)::text = ANY ((ARRAY['pending'::character varying, 'synced'::character varying, 'failed'::character varying, 'removed'::character varying])::text[])))
);
CREATE SEQUENCE public.device_person_sync_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.device_person_sync_id_seq OWNED BY public.device_person_sync.id;
CREATE TABLE public.devices (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    ip character varying(45) NOT NULL,
    port smallint DEFAULT '80'::smallint NOT NULL,
    username character varying(255) DEFAULT 'admin'::character varying NOT NULL,
    password text NOT NULL,
    direction character varying(255) DEFAULT 'entry'::character varying NOT NULL,
    location character varying(255),
    model character varying(255),
    firmware character varying(255),
    serial character varying(255),
    is_active boolean DEFAULT true NOT NULL,
    last_seen_at timestamp(0) without time zone,
    last_error text,
    created_at timestamp(0) without time zone,
    updated_at timestamp(0) without time zone,
    CONSTRAINT devices_direction_check CHECK (((direction)::text = ANY ((ARRAY['entry'::character varying, 'exit'::character varying])::text[])))
);
CREATE SEQUENCE public.devices_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.devices_id_seq OWNED BY public.devices.id;
CREATE TABLE public.failed_jobs (
    id bigint NOT NULL,
    uuid character varying(255) NOT NULL,
    connection character varying(255) NOT NULL,
    queue character varying(255) NOT NULL,
    payload text NOT NULL,
    exception text NOT NULL,
    failed_at timestamp(0) without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);
CREATE SEQUENCE public.failed_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.failed_jobs_id_seq OWNED BY public.failed_jobs.id;
CREATE TABLE public.job_batches (
    id character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    total_jobs integer NOT NULL,
    pending_jobs integer NOT NULL,
    failed_jobs integer NOT NULL,
    failed_job_ids text NOT NULL,
    options text,
    cancelled_at integer,
    created_at integer NOT NULL,
    finished_at integer
);
CREATE TABLE public.jobs (
    id bigint NOT NULL,
    queue character varying(255) NOT NULL,
    payload text NOT NULL,
    attempts smallint NOT NULL,
    reserved_at integer,
    available_at integer NOT NULL,
    created_at integer NOT NULL
);
CREATE SEQUENCE public.jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.jobs_id_seq OWNED BY public.jobs.id;
CREATE TABLE public.migrations (
    id integer NOT NULL,
    migration character varying(255) NOT NULL,
    batch integer NOT NULL
);
CREATE SEQUENCE public.migrations_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.migrations_id_seq OWNED BY public.migrations.id;
CREATE TABLE public.password_reset_tokens (
    email character varying(255) NOT NULL,
    token character varying(255) NOT NULL,
    created_at timestamp(0) without time zone
);
CREATE TABLE public.people (
    id bigint NOT NULL,
    external_id character varying(255),
    source character varying(32) DEFAULT 'manual'::character varying NOT NULL,
    user_id character varying(32) NOT NULL,
    full_name character varying(255) NOT NULL,
    photo_path character varying(255),
    photo_hash character varying(64),
    photo_status character varying(255) DEFAULT 'missing'::character varying NOT NULL,
    photo_reject_reason text,
    photo_metrics json,
    valid_from date,
    valid_to date,
    is_active boolean DEFAULT true NOT NULL,
    synced_at timestamp(0) without time zone,
    created_at timestamp(0) without time zone,
    updated_at timestamp(0) without time zone,
    legacy_user_id character varying(32),
    merged_at timestamp(0) without time zone,
    CONSTRAINT people_photo_status_check CHECK (((photo_status)::text = ANY ((ARRAY['missing'::character varying, 'pending'::character varying, 'valid'::character varying, 'rejected'::character varying])::text[])))
);
CREATE SEQUENCE public.people_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.people_id_seq OWNED BY public.people.id;
CREATE TABLE public.sessions (
    id character varying(255) NOT NULL,
    user_id bigint,
    ip_address character varying(45),
    user_agent text,
    payload text NOT NULL,
    last_activity integer NOT NULL
);
CREATE TABLE public.sync_runs (
    id bigint NOT NULL,
    device_id bigint,
    kind character varying(32) NOT NULL,
    started_at timestamp(0) without time zone NOT NULL,
    finished_at timestamp(0) without time zone,
    total integer DEFAULT 0 NOT NULL,
    ok_count integer DEFAULT 0 NOT NULL,
    fail_count integer DEFAULT 0 NOT NULL,
    status character varying(255) DEFAULT 'running'::character varying NOT NULL,
    error text,
    created_at timestamp(0) without time zone,
    updated_at timestamp(0) without time zone,
    CONSTRAINT sync_runs_status_check CHECK (((status)::text = ANY ((ARRAY['running'::character varying, 'done'::character varying, 'failed'::character varying])::text[])))
);
CREATE SEQUENCE public.sync_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.sync_runs_id_seq OWNED BY public.sync_runs.id;
CREATE TABLE public.users (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    email character varying(255) NOT NULL,
    email_verified_at timestamp(0) without time zone,
    password character varying(255) NOT NULL,
    remember_token character varying(100),
    created_at timestamp(0) without time zone,
    updated_at timestamp(0) without time zone
);
CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;
ALTER TABLE ONLY public.device_person_sync ALTER COLUMN id SET DEFAULT nextval('public.device_person_sync_id_seq'::regclass);
ALTER TABLE ONLY public.devices ALTER COLUMN id SET DEFAULT nextval('public.devices_id_seq'::regclass);
ALTER TABLE ONLY public.failed_jobs ALTER COLUMN id SET DEFAULT nextval('public.failed_jobs_id_seq'::regclass);
ALTER TABLE ONLY public.jobs ALTER COLUMN id SET DEFAULT nextval('public.jobs_id_seq'::regclass);
ALTER TABLE ONLY public.migrations ALTER COLUMN id SET DEFAULT nextval('public.migrations_id_seq'::regclass);
ALTER TABLE ONLY public.people ALTER COLUMN id SET DEFAULT nextval('public.people_id_seq'::regclass);
ALTER TABLE ONLY public.sync_runs ALTER COLUMN id SET DEFAULT nextval('public.sync_runs_id_seq'::regclass);
ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);
ALTER TABLE ONLY public.cache_locks
    ADD CONSTRAINT cache_locks_pkey PRIMARY KEY (key);
ALTER TABLE ONLY public.cache
    ADD CONSTRAINT cache_pkey PRIMARY KEY (key);
ALTER TABLE ONLY public.device_person_sync
    ADD CONSTRAINT device_person_sync_device_id_person_id_unique UNIQUE (device_id, person_id);
ALTER TABLE ONLY public.device_person_sync
    ADD CONSTRAINT device_person_sync_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_ip_port_unique UNIQUE (ip, port);
ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.failed_jobs
    ADD CONSTRAINT failed_jobs_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.failed_jobs
    ADD CONSTRAINT failed_jobs_uuid_unique UNIQUE (uuid);
ALTER TABLE ONLY public.job_batches
    ADD CONSTRAINT job_batches_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.jobs
    ADD CONSTRAINT jobs_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.migrations
    ADD CONSTRAINT migrations_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.password_reset_tokens
    ADD CONSTRAINT password_reset_tokens_pkey PRIMARY KEY (email);
ALTER TABLE ONLY public.people
    ADD CONSTRAINT people_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.people
    ADD CONSTRAINT people_source_external_id_unique UNIQUE (source, external_id);
ALTER TABLE ONLY public.people
    ADD CONSTRAINT people_user_id_unique UNIQUE (user_id);
ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.sync_runs
    ADD CONSTRAINT sync_runs_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_unique UNIQUE (email);
ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);
CREATE INDEX cache_expiration_index ON public.cache USING btree (expiration);
CREATE INDEX cache_locks_expiration_index ON public.cache_locks USING btree (expiration);
CREATE INDEX device_person_sync_device_id_state_index ON public.device_person_sync USING btree (device_id, state);
CREATE INDEX devices_is_active_index ON public.devices USING btree (is_active);
CREATE INDEX failed_jobs_connection_queue_failed_at_index ON public.failed_jobs USING btree (connection, queue, failed_at);
CREATE INDEX jobs_queue_index ON public.jobs USING btree (queue);
CREATE INDEX people_is_active_index ON public.people USING btree (is_active);
CREATE INDEX people_legacy_user_id_index ON public.people USING btree (legacy_user_id);
CREATE INDEX people_photo_status_index ON public.people USING btree (photo_status);
CREATE INDEX sessions_last_activity_index ON public.sessions USING btree (last_activity);
CREATE INDEX sessions_user_id_index ON public.sessions USING btree (user_id);
CREATE INDEX sync_runs_device_id_kind_index ON public.sync_runs USING btree (device_id, kind);
ALTER TABLE ONLY public.device_person_sync
    ADD CONSTRAINT device_person_sync_device_id_foreign FOREIGN KEY (device_id) REFERENCES public.devices(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.device_person_sync
    ADD CONSTRAINT device_person_sync_person_id_foreign FOREIGN KEY (person_id) REFERENCES public.people(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.sync_runs
    ADD CONSTRAINT sync_runs_device_id_foreign FOREIGN KEY (device_id) REFERENCES public.devices(id) ON DELETE SET NULL;
\unrestrict rGQ3naz0uTXcR56jxw7lYOCeMVptUVx02HwpdxhnLhtWzmLrmyUuG33qQ4PrMnE
