# Coder Agents: external auth login link

Fix for a dead-end error: when a template requires external auth the workspace
owner hasn't linked yet, Coder Agents' `create_workspace` tool now surfaces a
clickable login link instead of just failing.

Recorded 2026-09-04 against `ben/chatd-external-auth-link` branch.

## What changed

- `requireWorkspaceOwnerExternalAuth` (coderd/workspaces.go) includes each
  missing provider's authenticate URL in the error detail.
- The chatd `create_workspace` tool surfaces an `action_required` hint so the
  model reliably tells the user to open the link and retry.

The clip shows the chat asking to create a workspace from a template that
requires GitHub auth, and the assistant replying with a clickable
`/external-auth/github` link. The clip is trimmed before the OAuth redirect,
since the dev environment uses a dummy GitHub OAuth client (no real client
secret), so following the link past Coder's own page is expected to fail
there - unrelated to this fix.

![Demo](recording.gif)
