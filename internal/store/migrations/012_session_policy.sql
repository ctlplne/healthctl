-- Sessions record when the owner last proved the password and was last
-- active, for idle time-outs and re-authentication before sensitive actions.
-- Sign-in attempts are counted per client, so one client can't lock out others.
-- A session that existed before this migration hasn't proved the password
-- recently, so it starts outside the re-authentication window.
ALTER TABLE sessions ADD COLUMN authenticated_at timestamptz NOT NULL DEFAULT 'epoch';
ALTER TABLE sessions ALTER COLUMN authenticated_at SET DEFAULT now();
ALTER TABLE sessions ADD COLUMN last_seen timestamptz NOT NULL DEFAULT now();
CREATE TABLE login_attempts (
  client text PRIMARY KEY,
  window_start timestamptz NOT NULL,
  attempts integer NOT NULL
);
