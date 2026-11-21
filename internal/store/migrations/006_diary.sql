CREATE TABLE diary_entries (
 owner_id integer NOT NULL REFERENCES owners(id) ON DELETE CASCADE,
 id text NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 payload bytea NOT NULL,
 digest bytea NOT NULL,
 PRIMARY KEY(owner_id,id)
);
