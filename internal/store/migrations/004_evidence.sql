CREATE TABLE evidence_versions (
  owner_id smallint NOT NULL REFERENCES owners ON DELETE CASCADE,
  collection text NOT NULL,
  version text NOT NULL,
  origin text NOT NULL CHECK (origin IN ('bundled','operator')),
  status text NOT NULL CHECK (status IN ('active','retired','withdrawn')),
  reason text NOT NULL DEFAULT '',
  installed_at timestamptz NOT NULL DEFAULT now(),
  manifest jsonb NOT NULL,
  content_hash text NOT NULL,
  PRIMARY KEY(owner_id,collection,version)
);
CREATE UNIQUE INDEX evidence_single_active ON evidence_versions(owner_id,collection) WHERE status='active';
CREATE TABLE model_operations (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  owner_id smallint NOT NULL REFERENCES owners ON DELETE CASCADE,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  provider text NOT NULL, model text NOT NULL,
  returned_model text NOT NULL, destination_hash text NOT NULL,
  status text NOT NULL, requests int NOT NULL, retries int NOT NULL,
  input_tokens bigint NOT NULL, output_tokens bigint NOT NULL,
  usage_complete boolean NOT NULL, duration_ms bigint NOT NULL,
  estimated_usd double precision
);
CREATE INDEX model_operations_owner ON model_operations(owner_id,occurred_at);
CREATE TABLE evidence_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, owner_id smallint NOT NULL REFERENCES owners ON DELETE CASCADE,
 collection text NOT NULL, version text NOT NULL, action text NOT NULL, reason text NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
