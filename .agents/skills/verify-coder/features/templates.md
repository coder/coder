# Templates

Users see the templates they can build workspaces from. Template admins create templates from the starter gallery or push a Terraform folder from the CLI.

## Sub-features

- `templates-list` lists templates in the UI and the CLI.
- `templates-page` opens one template with its docs, files and versions.
- `templates-starter` creates a template from a starter in the gallery.
- `templates-push` pushes a new version from a folder.

## How to get to it (user POV)

- `Templates` in the navbar, or `$VC_WEB_URL/templates`.
- `$VC_WEB_URL/templates/docker` for the seeded template.
- `$VC_WEB_URL/starter-templates`, then a starter, then `Use template` (it links to `/templates/new?exampleId=<id>`).
- Run `coder templates list` or `coder templates push` in a terminal.

## Driving it with agent-browser and the coder CLI

Preconditions:

- `scripts/doctor.sh` passes. Its `starter-template` check fails when Docker works but the `docker` template is missing. Without Docker the list is empty and that check passes.

- **List.** `vc_ab open "$VC_WEB_URL/templates"` and wait with `vc_ab wait --fn 'document.title === "Templates - Coder"'`. The table has a row button named `Docker Containers ... Create Workspace`, and `vc_ab get count 'a[href="/templates/docker/workspace"]'` prints `1`. The empty-state text `Create your first template` is absent. `scripts/coder.sh templates list --output json | jq -r '.[].Template.name'` prints `docker`.
- **Template page.** `vc_ab open "$VC_WEB_URL/templates/docker"`. The page has a `Create Workspace` link.
- **Create from a starter.** `vc_ab open "$VC_WEB_URL/starter-templates/docker"`, click the `Use template` link, fill `find label "Name"` with a new name, then `vc_ab find role button click --name "Save"`. The import runs a provisioner job. Prove it with `scripts/coder.sh templates list --output json`.
- **Push a version.** `scripts/coder.sh templates pull docker <dir>`, edit a file, then `scripts/coder.sh templates push docker --directory <dir> --message "verify" --yes`. `scripts/coder.sh templates versions list docker` shows the new active version.

## Gotchas

- The empty state (`Create your first template`) shows starter cards, including one headed `Docker Containers`. That text alone does not prove the template exists.
- The template path gains the organization (`/templates/coder/docker`) when organizations show. The seed has one organization.
- The `docker` template needs the `kreuzwerker/docker` Terraform provider. The first import downloads it, which takes minutes on a slow network.
- `coder templates create` is deprecated. Use `templates push`.
