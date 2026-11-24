-- A data key encrypts the records. The installation key only wraps it and the
-- receipt key, so rotating the installation key re-wraps two keys instead of
-- rewriting every record. Installations created before this migration keep
-- their records under their original key, now wrapped as the data key.
CREATE TABLE data_key (
  id smallint PRIMARY KEY CHECK (id = 1),
  wrapped bytea NOT NULL,
  legacy boolean NOT NULL
);
ALTER TABLE receipt_key ADD COLUMN independent boolean NOT NULL DEFAULT false;
