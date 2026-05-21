# Personal Workers

A Personal Worker is one Coder workspace per developer, running one
[Cursor self-hosted](https://cursor.com/docs/cloud-agent/my-machines.md)
worker that's registered to that developer's Cursor account. The
worker shows up in the developer's own
[cloud agents dashboard](https://cursor.com/agents), and only their
sessions route to it. No service account, no shared pool.

If you're on Cursor Team or above and you want a per-developer
self-hosted setup that takes ten minutes to ship, this is the page.

> [!NOTE]
> Personal Workers register as **My Machines** workers in Cursor, not
> as pool workers. Sessions reach them via the cursor.com environment
> picker or `worker=<name>` / `machine=<name>` from Slack, GitHub, or
> Linear. There's no shared inventory; Alice's workers don't serve
> Bob's sessions.

## What developers do

1. Generate a personal Cursor API key at
   [cursor.com/dashboard](https://cursor.com/dashboard) under
   Integrations → API Keys.
2. Create a new workspace from the `cursor-personal-worker` template
   in Coder.
3. Paste the API key into the workspace parameter.
4. Wait for the workspace to start. The worker registers itself
   automatically.
5. Go to [cursor.com/agents](https://cursor.com/agents), pick the new
   machine from the environment dropdown, submit a session.

That's it. The worker stays connected as long as the workspace is
running. Cursor's idle-release timeout exits the worker process after
8h of no work; restarting the workspace re-registers it.

## Template

A minimal template lives at
[`examples/cursor-personal-worker/main.tf`](./examples/cursor-personal-worker/main.tf).
The shape is:

```hcl
data "coder_parameter" "cursor_api_key" {
  name        = "cursor_api_key"
  display_name = "Cursor personal API key"
  description = "Generate from cursor.com/dashboard -> Integrations -> API Keys."
  type        = "string"
  mutable     = true
}

data "coder_parameter" "git_repo_url" {
  name        = "git_repo_url"
  display_name = "Git repository URL"
  default     = "https://github.com/your-org/your-repo"
  type        = "string"
  mutable     = false
}

resource "coder_agent" "main" {
  env = {
    CURSOR_API_KEY = data.coder_parameter.cursor_api_key.value
    GIT_REPO_URL   = data.coder_parameter.git_repo_url.value
  }

  startup_script = <<-EOT
    set -eu
    export PATH="$HOME/.local/bin:$PATH"
    if ! command -v cursor-agent >/dev/null; then
      curl -fsSL https://cursor.com/install | bash
    fi

    REPO_DIR="$HOME/workspace"
    if [ ! -d "$REPO_DIR/.git" ]; then
      git clone "$GIT_REPO_URL" "$REPO_DIR"
    fi

    nohup cursor-agent --api-key "$CURSOR_API_KEY" worker \
      --worker-dir "$REPO_DIR" \
      --name "coder-${data.coder_workspace_owner.me.name}" \
      start \
      > "$HOME/cursor-agent.log" 2>&1 &
  EOT
}
```

Notes:

- **No `--pool` flag.** Personal keys can't start pool workers; Cursor
  rejects that combination. Workers started without `--pool` register
  as My Machines workers, scoped to the key's owner.
- **`--name` is the user's worker name.** It's what shows up in the
  dropdown and what they reference in Slack or GitHub as
  `worker=<name>`. Embedding the Coder workspace owner makes it
  unique per developer.
- **`cursor_api_key` is a mutable parameter, not a template variable.**
  Each developer pastes their own key when they create their
  workspace. The platform team doesn't see or store the keys.

## Identity

- **Workspace owner** = the developer (Coder).
- **Git push identity** = the developer's git credential, wired
  through [Coder external auth](../../admin/external-auth/index.md).
- **Cursor worker identity** = the developer's Cursor account (via
  their personal API key).
- **Cursor session log** = sessions are attributed to the developer
  who triggered them.

End to end, the human is the user of record on every surface.

## Where it fits

- For an admin-operated shared pool (system identity, no per-user
  attribution), use [Worker Pool](./system-identity.md).
- For per-user identity on a *shared* pool, see
  [User identity on a shared pool](./concepts/user-identity.md). This
  is unshippable today; see that page for what would unblock it.

## Related

- [Worker Pool](./system-identity.md): admin-operated central pool,
  requires Cursor Enterprise.
- Cursor [My Machines](https://cursor.com/docs/cloud-agent/my-machines.md)
  documentation.
