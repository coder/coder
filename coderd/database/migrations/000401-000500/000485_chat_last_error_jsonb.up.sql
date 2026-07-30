ALTER TABLE chats
  ALTER COLUMN last_error TYPE jsonb
  USING CASE
    WHEN last_error IS NULL THEN NULL
    ELSE jsonb_build_object(
      'message', last_error,
      'kind', 'generic'
    )
  END;
