DROP INDEX IF EXISTS idx_chat_model_configs_provider_model;

WITH ranked AS (
	SELECT
		id,
		ROW_NUMBER() OVER (
			PARTITION BY provider, model
			ORDER BY updated_at DESC, created_at DESC, id DESC
		) AS rownum
	FROM chat_model_configs
)
DELETE FROM chat_model_configs AS cmc
USING ranked
WHERE
	cmc.id = ranked.id
	AND ranked.rownum > 1;

ALTER TABLE chat_model_configs
	ADD CONSTRAINT chat_model_configs_provider_model_key UNIQUE (provider, model);
