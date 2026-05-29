INSERT INTO hold_templates (name, description, provider_glob, blocked_actions, is_default)
SELECT 'Standard Hold', 'Default hold template for all providers', '*', '{}', true
WHERE NOT EXISTS (SELECT 1 FROM hold_templates WHERE is_default = true);
