-- Example:
-- This fixture is applied after migrations/000024_site_config.up.sql
-- and inserts a value into site_configs that must not cause issues in
-- future migrations.

INSERT INTO site_configs (key, value) VALUES ('mytest', 'example');
