# Codex session grouping in AI Gateway

Before/after screenshots for the fix that makes AI Gateway recognize the
`session-id` header newer Codex CLI releases send, so Codex prompts group
into one session instead of one session per request.

Recorded 2026-06-12 against a v2.34.2-devel build with the fix applied.

- `before.jpg`: every Codex prompt appears as its own session (Threads: 1)
- `after.jpg`: prompts from one Codex conversation grouped into a single session (Threads: 3)
