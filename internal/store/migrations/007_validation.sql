-- Fixed, nonclinical validation outcomes for each model operation.
ALTER TABLE model_operations
  ADD COLUMN validator_version text NOT NULL DEFAULT '',
  ADD COLUMN rejected_items jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN failure_reason text NOT NULL DEFAULT '';
