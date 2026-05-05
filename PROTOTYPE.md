# Docker Error UX Prototype

Better error messages when Docker isn't available, instead of raw
Terraform output.

**Branch:** `bpmct/docker-error-ux`
**Recording:** [GIF + mp4](https://github.com/coder/coder/tree/recordings/recordings/docker-error-ux)
**Linear:** [DEVREL-14](https://linear.app/codercom/issue/DEVREL-14)

## The problem

When Docker is missing or unreachable, workspace builds fail with
`terraform plan: exit status 1`. Users see a wall of Terraform
logs and have to dig through them to figure out what went wrong.

## What this prototype does

Three layers work together:

### 1. Template: early Docker check with a human error message

`examples/templates/docker/main.tf` adds an `external` data source
that pings the Docker socket before any Docker resources are created.
A `precondition` on `docker_volume.home_volume` checks the result
and fails with a clear message if Docker is unreachable:

```
Could not reach Docker at /var/run/docker.sock.

This template needs a working Docker socket on the provisioner host.
A few things to check:
  - Is Docker installed?
  - Is the daemon running? (sudo systemctl start docker)
  - Can the Coder user access the socket?

Docs: https://coder.com/docs/admin/templates/extending-templates/docker-in-workspaces
```

The check uses `curl --unix-socket` to hit Docker's `/_ping`
endpoint, so it catches socket-exists-but-daemon-dead cases too.

### 2. Backend: surface the diagnostic instead of "exit status 1"

`provisioner/terraform/executor.go` adds a `diagnosticCollector`
that captures Terraform error diagnostics during plan/apply. When
a plan or apply fails, the error message uses the last diagnostic
summary instead of the generic exit code.

Key detail: the log-processing goroutine has to finish before we
read the diagnostics. The code explicitly closes writers and drains
goroutine channels before checking `diags.Summary()`. The defer
is kept as a safety net for other code paths.

### 3. Frontend: format the error nicely

`site/src/modules/provisioners/BuildErrorAlert.tsx` is a shared
component used in three places:

- **Workspace page** (`Workspace.tsx`) - replaces the old plain `<Alert>`
- **Build page** (`WorkspaceBuildPageView.tsx`) - shown when build status is "failed"
- **Template import drawer** (`BuildLogsDrawer.tsx`) - shown as "Template import failed"

The component:
- Strips the Terraform boilerplate prefix ("Coder encountered an error...")
- Groups lines into paragraphs (blank lines become visual breaks)
- Turns URLs into clickable links

## How to test

```bash
git checkout bpmct/docker-error-ux

# Start the dev server
./scripts/develop.sh

# Hide Docker to trigger the failure
sudo mv /var/run/docker.sock /var/run/docker.sock.bak

# Create a workspace from the docker template via the UI
# It will fail with the formatted error message

# Restore when done
sudo mv /var/run/docker.sock.bak /var/run/docker.sock
```

## What's rough / needs work

- **Indentation in executor.go** is inconsistent in a few spots
  (tabs got mixed with the surrounding code). Needs a cleanup
  pass before this is PR-ready.
- **The boilerplate prefix stripping** in `BuildErrorAlert.tsx`
  is brittle (exact string match). If Coder changes that prefix
  text, it stops working. A more robust approach might be to have
  the backend strip it, or use a structured error field.
- **`diagnosticCollector` only returns the last error.** If
  multiple resources fail, earlier diagnostics are lost. Might
  want to concatenate them or return the most "interesting" one.
- **No tests yet** for `BuildErrorAlert` (needs Storybook stories)
  or for the `diagnosticCollector` logic.
- **The `external` data source requires `curl`** on the
  provisioner host. Most Linux systems have it, but it's not
  guaranteed. Could fall back to checking socket existence only.

## Ideas for extending this

- **Why `precondition` not `check`**: Terraform `check` blocks
  (added in v1.5) only produce warnings, they don't block the
  operation. If Docker is missing, continuing is pointless since
  every Docker resource will fail with a worse error. `precondition`
  blocks the plan with a clear error, which is what we want.
  `check` blocks are better for post-deploy health validation
  ("is this service responding?") where you want to know but
  not block.
- **Other templates**: the same `external` + `precondition` pattern
  works for any prerequisite check (Kubernetes connectivity,
  cloud credentials, etc.). Could be a reusable Terraform module.
- **Structured error field**: instead of string-matching the
  boilerplate prefix on the frontend, add a separate field to the
  job error payload that contains just the diagnostic text.
- **Warning-level diagnostics**: currently only errors are
  captured. Terraform `check` blocks produce warnings that could
  surface as non-fatal alerts in the UI.
- **Template author docs**: a guide on writing good precondition
  messages (keep it short, include a docs link, use blank lines
  for structure).
