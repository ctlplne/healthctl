-- Sessions record when the owner last proved the password and was last
-- active, for idle time-outs and re-authentication before sensitive actions.
-- Sign-in attempts are counted per client, so one client can't lock out others.
ALTER TABLE sessions ADD COLUMN authenticated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE sessions ADD COLUMN last_seen timestamptz NOT NULL DEFAULT now();
CREATE TABLE login_attempts (
  client text PRIMARY KEY,
  window_start timestamptz NOT NULL,
  attempts integer NOT NULL
);
