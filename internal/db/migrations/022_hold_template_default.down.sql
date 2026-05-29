DROP INDEX IF EXISTS hold_templates_single_default;
ALTER TABLE hold_templates DROP COLUMN IF EXISTS is_default;
