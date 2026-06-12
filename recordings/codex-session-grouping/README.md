# Codex session grouping in AI Gateway

Before/after screenshots for the fix that makes AI Gateway recognize the
`session-id` header newer Codex CLI releases send, so Codex prompts group
into one session instead of one session per request.

The same three-prompt Codex conversation was run for both captures
("Write a haiku about Pittsburgh" -> "Now make it about Coder" ->
"Translate it to Spanish", via `codex exec` + `codex exec resume --last`).

Recorded 2026-06-12 against a v2.34.2-devel build.

- `before.jpg`: pre-fix behavior, the conversation appears as 3 separate sessions, each with Threads: 1
- `after.jpg`: with the fix, the same conversation is a single session with Threads: 3
- `after-session-detail.jpg`: clicked into the session, showing all 3 threads on the session timeline
