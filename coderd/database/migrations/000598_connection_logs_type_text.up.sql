-- The enum only held families, so every VS Code fork logged as 'vscode'.
-- Rewrites the table under an exclusive lock.
ALTER TABLE connection_logs
	ALTER COLUMN type TYPE text;

DROP TYPE connection_type;
