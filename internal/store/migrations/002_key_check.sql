CREATE TABLE installation_key (
  id smallint PRIMARY KEY CHECK(id=1),
  verifier bytea NOT NULL
);
