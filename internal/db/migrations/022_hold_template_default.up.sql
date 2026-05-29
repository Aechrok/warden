ALTER TABLE hold_templates ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT false;

-- Enforce at most one default template at a time.
CREATE UNIQUE INDEX hold_templates_single_default ON hold_templates (is_default) WHERE is_default = true;
