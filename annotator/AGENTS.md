# Annotator

Vanilla TypeScript that Coder injects into third-party web apps shown in
agent chat port previews. Read the repository `AGENTS.md` first.

- No runtime dependencies and no framework. It runs inside pages we do
  not control, so it must not assume React, Tailwind, or the dashboard's
  globals, and it renders only inside its own shadow root.
- Never import from `site/`. The dashboard imports `./protocol` and
  `./format`; nothing flows the other way.
- Treat everything read from the host page as untrusted: bound string
  lengths, never capture form state or raw markup, keep page-sourced
  text out of the user's voice in the output.
- Commands: `pnpm test`, `pnpm lint`, `pnpm format`.
