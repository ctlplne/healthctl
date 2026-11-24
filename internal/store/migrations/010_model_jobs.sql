-- Background model calls are jobs, so a restart resumes them and a crash in one
-- call is recorded instead of lost. No clinical content is stored here.
CREATE TABLE model_jobs (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  owner_id smallint NOT NULL REFERENCES owners ON DELETE CASCADE,
  encounter_id text NOT NULL,
  task text NOT NULL CHECK (task IN ('interview', 'assessment')),
  revision int NOT NULL,
  local_plan boolean NOT NULL DEFAULT false,
  status text NOT NULL CHECK (status IN ('queued', 'running', 'done', 'failed', 'superseded')),
  attempts int NOT NULL DEFAULT 0,
  error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  finished_at timestamptz
);
CREATE INDEX model_jobs_queue ON model_jobs(status, id);
CREATE INDEX model_jobs_encounter ON model_jobs(encounter_id, status);
