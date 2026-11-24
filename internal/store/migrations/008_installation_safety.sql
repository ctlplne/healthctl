-- Fixed times and codes that let the app show whether backups and the
-- recovery kit are in place. No clinical content.
CREATE TABLE backup_status (
  id smallint PRIMARY KEY CHECK (id = 1),
  last_attempt timestamptz NOT NULL,
  last_success timestamptz,
  last_error text NOT NULL DEFAULT '',
  destination_kind text NOT NULL DEFAULT ''
);
CREATE TABLE recovery_kit (
  owner_id smallint PRIMARY KEY REFERENCES owners ON DELETE CASCADE,
  fingerprint_digest bytea NOT NULL,
  confirmed_at timestamptz NOT NULL
);
