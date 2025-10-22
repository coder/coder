INSERT INTO chats (id, owner_id, created_at, updated_at, title) VALUES
('00000000-0000-0000-0000-000000000001', '0ed9befc-4911-4ccf-a8e2-559bf72daa94', '2023-10-01 12:00:00+00', '2023-10-01 12:00:00+00', 'Test Chat 1');

INSERT INTO chat_messages (id, chat_id, created_at, model, provider, content) VALUES
(1, '00000000-0000-0000-0000-000000000001', '2023-10-01 12:00:00+00', 'annie-oakley', 'cowboy-coder', '{"role":"user","content":"Hello"}'),
(2, '00000000-0000-0000-0000-000000000001', '2023-10-01 12:01:00+00', 'annie-oakley', 'cowboy-coder', '{"role":"assistant","content":"Howdy pardner! What can I do ya for?"}');
