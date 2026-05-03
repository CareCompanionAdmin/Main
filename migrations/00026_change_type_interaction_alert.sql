-- 00026_change_type_interaction_alert.sql
--
-- Adds the `interaction_alert` value to the change_type enum so the
-- "Remind Me Later" path (POST /api/interaction-alerts) can persist a
-- treatment_changes row when the user defers a drug-interaction warning.
--
-- The handler at internal/handler/api/transparency_handler.go:197 was
-- inserting `ChangeType("interaction_alert")` and getting back a Postgres
-- enum-mismatch SQLSTATE error, surfaced to the user as a 500.

ALTER TYPE change_type ADD VALUE IF NOT EXISTS 'interaction_alert';
