ALTER TABLE chat_messages ADD COLUMN content_text text;

-- Maintain content_text as the space-joined text of a message's `text` content
-- parts. The jsonb_typeof guard keeps legacy/non-array content (pre-000434
-- scalar strings) from raising "cannot extract elements from a scalar".
CREATE FUNCTION chat_messages_set_content_text() RETURNS trigger
	LANGUAGE plpgsql AS $$
BEGIN
	IF NEW.content IS NOT NULL AND jsonb_typeof(NEW.content) = 'array' THEN
		SELECT string_agg(part->>'text', ' ' ORDER BY ordinality)
		INTO NEW.content_text
		FROM jsonb_array_elements(NEW.content) WITH ORDINALITY AS t(part, ordinality)
		WHERE part->>'type' = 'text';
	ELSE
		NEW.content_text := NULL;
	END IF;
	RETURN NEW;
END;
$$;

CREATE TRIGGER chat_messages_content_text
	BEFORE INSERT OR UPDATE OF content ON chat_messages
	FOR EACH ROW
	EXECUTE FUNCTION chat_messages_set_content_text();

-- Backfill existing rows.
UPDATE chat_messages cm
SET content_text = sub.txt
FROM (
	SELECT m.id,
		string_agg(part->>'text', ' ' ORDER BY ordinality) AS txt
	FROM chat_messages m,
		jsonb_array_elements(m.content) WITH ORDINALITY AS t(part, ordinality)
	WHERE jsonb_typeof(m.content) = 'array' AND part->>'type' = 'text'
	GROUP BY m.id
) sub
WHERE cm.id = sub.id;

-- GIN expression index for full-text search over the extracted text. The
-- two-argument to_tsvector(regconfig, text) form is IMMUTABLE, which an index
-- expression requires.
CREATE INDEX idx_chat_messages_content_tsv
	ON chat_messages USING GIN (to_tsvector('simple', content_text));
