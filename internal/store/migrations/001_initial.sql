CREATE TABLE owners (
  id smallint PRIMARY KEY CHECK (id = 1),
  username text NOT NULL UNIQUE,
  password_hash bytea NOT NULL,
  provider jsonb NOT NULL DEFAULT '{"kind":"","model":"","endpoint":"","structured":"auto"}'::jsonb
);
CREATE TABLE sessions (
  digest bytea PRIMARY KEY,
  owner_id smallint NOT NULL REFERENCES owners ON DELETE CASCADE,
  expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_expiry ON sessions (expires_at);
CREATE TABLE login_budget (
  id smallint PRIMARY KEY CHECK (id = 1),
  window_start timestamptz NOT NULL,
  attempts integer NOT NULL
);
CREATE TABLE encounters (
  id text PRIMARY KEY,
  owner_id smallint NOT NULL REFERENCES owners,
  revision bigint NOT NULL,
  payload bytea NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  create_request text NOT NULL,
  create_digest bytea NOT NULL,
  UNIQUE (owner_id, create_request)
);
CREATE INDEX encounters_owner_recent ON encounters (owner_id, updated_at DESC);
CREATE TABLE submissions (
  encounter_id text NOT NULL REFERENCES encounters(id) ON DELETE CASCADE,
  request_id text NOT NULL,
  digest bytea NOT NULL,
  PRIMARY KEY (encounter_id, request_id)
);
CREATE TABLE audit_events (
  seq bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  owner_id smallint NOT NULL REFERENCES owners,
  encounter_id text REFERENCES encounters(id) ON DELETE CASCADE,
  event text NOT NULL,
  settings_change jsonb
);
