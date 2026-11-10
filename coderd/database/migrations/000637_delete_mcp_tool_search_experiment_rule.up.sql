-- Delete the stored runtime rule for mcp-tool-search, which is not a
-- user-scoped experiment. The server would otherwise keep listing the rule
-- as ignored, and the rules API cannot delete it.
DELETE FROM site_configs WHERE key = 'experiment_rule:mcp-tool-search';
