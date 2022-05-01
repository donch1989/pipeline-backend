BEGIN;
CREATE TYPE valid_mode AS ENUM (
  'MODE_UNSPECIFIED',
  'MODE_SYNC',
  'MODE_ASYNC'
);
CREATE TYPE valid_status AS ENUM (
  'STATUS_UNSPECIFIED',
  'STATUS_INACTIVATED',
  'STATUS_ACTIVATED',
  'STATUS_ERROR'
);
CREATE TABLE IF NOT EXISTS public.pipeline (
  id UUID NOT NULL,
  owner_id UUID NOT NULL,
  display_name VARCHAR(255) NOT NULL,
  description VARCHAR(1023) NULL,
  recipe JSONB NOT NULL,
  mode VALID_MODE DEFAULT 'MODE_UNSPECIFIED' NOT NULL,
  status VALID_STATUS DEFAULT 'STATUS_UNSPECIFIED' NOT NULL,
  create_time TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
  update_time TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
  delete_time TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NULL,
  CONSTRAINT pipeline_pkey PRIMARY KEY (id, display_name)
);
CREATE UNIQUE INDEX unique_owner_id_display_name_delete_time ON public.pipeline (owner_id, display_name)
WHERE delete_time IS NULL;
CREATE INDEX pipeline_id_create_time_pagination ON public.pipeline (id, create_time);
COMMIT;
