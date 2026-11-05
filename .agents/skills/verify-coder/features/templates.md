# Templates

Users see the templates they can build workspaces from. Template admins create templates from the starter gallery or push a Terraform folder from the CLI.

## Sub-features

- `templates-list` lists templates in the UI and the CLI.
- `templates-page` opens one template with its docs, files and versions.
- `templates-starter` creates a template from a starter in the gallery.
- `templates-push` pushes a new version from a folder.
- `templates-delete` deletes a template.

## How to get to it (user POV)

- `Templates` in the navbar, or `$VC_WEB_URL/templates`.
- `$VC_WEB_URL/templates/docker` for the seeded template.
- `$VC_WEB_URL/starter-templates`, then a starter, then `Use template` (it links to `/templates/new?exampleId=<id>`).
- Run `coder templates list` or `coder templates push` in a terminal.

## Driving it with agent-browser and the coder CLI

Preconditions:

- `scripts/doctor.sh` passes. Its `starter-template` check fails when Docker works but the `docker` template is missing. Without Docker the list is empty and that check passes.

- **Whole journey.** Run `journeys/templates.sh`. It ends with `templates: 4/4 checks passed` in about 10 seconds and writes evidence to `build/verify-coder/evidence/<stamp>-templates/`.
- **List.** `vc_ab open "$VC_WEB_URL/templates"` and wait with `vc_ab wait --fn 'document.title === "Templates - Coder"'`. The table has a row button named `Docker Containers ... Create Workspace`, and `vc_ab wait 'a[href="/templates/docker/workspace"]'` finds its link. The empty-state text `Create your first template` is absent. `scripts/coder.sh templates list --output json | jq -r '.[].Template.name'` prints `docker`.
- **Template page.** `vc_ab open "$VC_WEB_URL/templates/docker"` redirects to `/templates/docker/docs`. The page has a `Create Workspace` link.
- **Create from a starter.** `vc_ab open "$VC_WEB_URL/starter-templates/docker"` (title `Docker Containers - Coder`), then `vc_ab find role link click --name "Use template"`. The URL becomes `/templates/new?exampleId=docker` (title `Create Template - Coder`). Wait for `input[name=name]`, fill it with a new name, and click `Save`. The import takes a few seconds, then the URL becomes `/templates/<name>/files`. Second view: `vc_api /api/v2/templates` lists the name.
- **Push a version.** Push only to a template you created in this run (`templates-starter`), never to `docker` or another template that was there before: a reused `.coderv2` can hold a developer's templates. `scripts/coder.sh templates pull <name> "$dir"`, with `dir="$(mktemp -d)/t"`, writes `main.tf` and `README.md`. Append a comment line to `main.tf`, then `scripts/coder.sh templates push <name> --directory "$dir" --message "<text>" --yes </dev/null`. It prints `Updated version at ...`. Second view: the template's `active_version_id` changes, and `vc_api /api/v2/templateversions/<id>` has that `message`. `scripts/coder.sh templates versions list <name>` shows the new version as `Active`. That list has no message column and version names are random, so match the row by the version's `.name` from the same `templateversions` read.
- **Delete.** `scripts/coder.sh templates delete <name> --yes`. The template no longer appears in `vc_api /api/v2/templates`.

## Gotchas

- The empty state (`Create your first template`) shows starter cards, including one headed `Docker Containers`. That text alone does not prove the template exists.
- The template path gains the organization (`/templates/coder/docker`) when organizations show. The seed has one organization.
- The `docker` template needs the `kreuzwerker/docker` Terraform provider. The first import downloads it, which takes minutes on a slow network.
- `coder templates create` is deprecated. Use `templates push`.
