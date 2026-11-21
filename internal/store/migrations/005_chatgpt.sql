-- This host-local vault is omitted from portable backups. The host ID and
-- OAuth refresh credentials must never be activated on two restored copies.
CREATE TABLE chatgpt_vault (
    owner_id smallint PRIMARY KEY REFERENCES owners ON DELETE CASCADE,
    host_id text NOT NULL DEFAULT ('urn:uuid:' || gen_random_uuid()::text),
    accounts bytea
);
