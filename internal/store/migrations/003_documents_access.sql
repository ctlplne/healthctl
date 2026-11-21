CREATE TABLE encounter_documents (
    encounter_id text NOT NULL REFERENCES encounters(id) ON DELETE CASCADE,
    document_id text NOT NULL,
    original bytea NOT NULL,
    PRIMARY KEY (encounter_id, document_id)
);
CREATE TABLE brief_access (
    encounter_id text PRIMARY KEY REFERENCES encounters(id) ON DELETE CASCADE,
    digest bytea NOT NULL UNIQUE,
    history_revision integer NOT NULL,
    expires_at timestamptz NOT NULL
);
-- Idempotency receipts retain the same HMAC key across payload-key rotations.
CREATE TABLE receipt_key (id smallint PRIMARY KEY CHECK(id=1), wrapped bytea NOT NULL);
