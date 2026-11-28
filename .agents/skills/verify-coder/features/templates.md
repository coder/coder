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

## Driving it with the coder CLI and the API

Preconditions:

- `scripts/doctor.sh` passes. Its `starter-template` check fails when Docker works but the `docker` template is missing. Without Docker the list is empty and that check passes, so check `docker info` first.

- **List.** `vc_coder templates list --output json | jq -r '.[].Template.name'` prints `docker`, and `vc_api /api/v2/templates` lists it.
- **Web UI.** This skill drives no browser, so report `templates-page`, the UI list and creating from the starter gallery as not verified.
- **Create from a starter in the CLI.** `vc_coder templates init --id docker "$dir"`, with `dir="$(mktemp -d)/t"`, writes the starter's files. Then `vc_coder templates push <new name> --directory "$dir" --yes </dev/null` creates the template. Second view: `vc_api /api/v2/templates` lists the name. Run `vc_coder templates delete <new name> --yes` when you are done.
- **Push a version.** Push only to a template you created in this run, never to `docker` or another template that was there before: a reused `.coderv2` can hold a developer's templates. `vc_coder templates pull <name> "$dir"`, with `dir="$(mktemp -d)/t"`, writes `main.tf` and `README.md`. Append a comment line to `main.tf`, then `vc_coder templates push <name> --directory "$dir" --message "<text>" --yes </dev/null`. It prints `Updated version at ...`. Second view: the template's `active_version_id` changes, and `vc_api /api/v2/templateversions/<id>` has that `message`. `vc_coder templates versions list <name>` shows the new version as `Active`. That list has no message column and version names are random, so match the row by the version's `.name` from the same `templateversions` read.
- **Delete.** `vc_coder templates delete <name> --yes`. The template no longer appears in `vc_api /api/v2/templates`.

## Gotchas

- The `docker` template needs the `kreuzwerker/docker` Terraform provider. The first import downloads it, which takes minutes on a slow network.
- `coder templates create` is deprecated. Use `templates push`.
