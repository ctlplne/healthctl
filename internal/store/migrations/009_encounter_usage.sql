-- Link each model call to its encounter so the owner can see the cost and
-- waiting time of one encounter. Older rows have no encounter.
ALTER TABLE model_operations ADD COLUMN encounter_id text;
CREATE INDEX model_operations_encounter ON model_operations(owner_id, encounter_id);
