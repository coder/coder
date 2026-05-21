# Concepts

Designs and experimental patterns layered on top of the shipping
[Worker Pool](../system-identity.md) and [Personal Workers](../personal-workers.md) recipes.
These pages are concepts, not copy-paste recipes; treat them as
references for what's possible and what's blocked.

- [Autoscaling the Worker Pool](./autoscaling.md). Router that watches
  Cursor's fleet API and scales Coder workspaces on top of the
  prebuild baseline.
- [User identity on a shared pool](./user-identity.md). Why per-user
  attribution on the shared Worker Pool isn't shippable today and
  what would unblock it.
- [AI Governance integration](./ai-governance.md). How the two
  worker-identity paths interact with Coder AI Gateway today.
- [Implementation notes](./implementation-notes.md). Staged plan, open
  questions for both Coder and Cursor, and design history.
