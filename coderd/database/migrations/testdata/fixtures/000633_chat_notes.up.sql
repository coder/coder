INSERT INTO chat_notes (chat_id, path, content)
SELECT c.id, 'checkpoint.md', 'Fixture checkpoint.'
FROM (
    SELECT id FROM chats ORDER BY created_at, id LIMIT 1
) AS c;
