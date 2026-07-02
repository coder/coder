-- chat_message_search_text extracts the searchable plain text from a chat
-- message's JSONB content: the text of all type='text' parts, space-joined in
-- order. The CASE guard returns NULL for non-array content (legacy
-- content_version=0 rows store a scalar JSON string, and
-- jsonb_array_elements raises an error on scalars); legacy content is
-- intentionally excluded from search. Non-text parts (reasoning, tool calls,
-- files, ...) are excluded. IMMUTABLE so it can back an expression index.
CREATE FUNCTION chat_message_search_text(content jsonb) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE WHEN jsonb_typeof(content) = 'array' THEN (
        SELECT string_agg(part->>'text', ' ' ORDER BY ordinality)
        FROM jsonb_array_elements(content) WITH ORDINALITY AS t(part, ordinality)
        WHERE part->>'type' = 'text'
    ) END
$$;
