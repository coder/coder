# Kubernetes provisioning-scopes validation

Local POC validation on 2026-10-22. The provisioning-scopes implementation follows
checkpoint `36f37e344a` on `feat/workspace-network-policies`; the companion provider
uses `feat/template-resources`. Credentials, runtime state, and detailed logs stay
outside the reproducible source files.

## Completed two-root lifecycle checks

The first generic Kubernetes revision used an embedded Cilium manifest. These
checks passed before switching that implementation to `coder_network_policy`:

- Native shared-resource plan/apply created `workspaces` and its network policy.
- Workspace creation before initial shared-resource apply was rejected. A linked
  workspace version then consumed the namespace output for both pod and PVC and
  pinned the applied shared-resource revision.
- Protected cold startup and `go build ./...` succeeded. Serpent stayed at
  `40b60bf4c1495caddc0292f862de275da292e6aa` with its original source and Makefile.
  Unchanged `make lint` exited 2 with five errcheck, three loopclosure, and one
  SA1019 finding. No patch or suppression was applied.
- GitHub HTTPS succeeded. Unapproved HTTP/HTTPS and public TCP/UDP DNS failed;
  Hubble recorded policy-denied traffic and rejected DNS queries.
- Through the split editor, adding `example.com` and applying allowed a new HTTPS
  connection with status 200. Reapplying the restricted revision blocked it again.
  Workspace build ID/state, pod UID, and restart count did not change.
- Stop/start preserved a marker, unchanged checkout, linter checksum, populated
  caches, PVC, and policy; the workspace pod UID changed as expected.
- Shared-resource destroy was rejected while a workspace existed. Native workspace
  deletion succeeded, then native shared-resource destroy removed the policy
  and namespace. The unrelated `coder/coder-restricted-egress` UID and complete
  policy spec were preserved.

Detailed local evidence is under
`scratch/coder-cilium/.local/evidence/template-kubernetes/`. Do not publish that
private directory wholesale.

## Module and full-editor follow-up

The current source calls `module "coder_network_policy"`. Its canonical source
is bundled into each revision archive, not injected by a policy-specific backend
or resolved from a mutable provisioner-local path. The template owns the namespace;
workspace configuration consumes its explicit output without a fallback.

Passed local checks:

- Both Terraform roots validate and pass their mocked tests.
- All 13 canonical module mocked tests pass.
- The packaged archive initializes and validates independently of the source
  checkout's module symlink.
- Terraform formatting and startup-script ShellCheck pass. The startup script is
  byte-identical to `serpent-dev-template/startup.sh`.

Passed live checks with the module-backed root:

- Full-editor Build and Publish created the namespace and module-managed CNP.
  Both exported outputs matched the template and were consumed by a new workspace.
- Created and renamed a nested text file, built the revision, and downloaded its
  archive to verify the file content and complete nested module tree. Deleted the
  file and directory through the editor. The final six source files are
  byte-identical to the repository root and canonical module.
- Increased the editor height through its accessible control; Monaco resized
  from 503 to 583 pixels.
- GitHub HTTPS returned 200; unapproved example.com HTTPS and GitHub HTTP failed.
  Adding example.com in the editor, building, and publishing returned HTTPS 200.
  Restoring the restrictive source and publishing denied new connections again.
  Hubble recorded dropped example.com DNS queries.
- Workspace build/version IDs, Terraform state hash, pod/PVC UIDs, restart counts,
  and the workspace marker stayed unchanged across both policy updates.
- Protected startup and `go build ./...` passed with the unchanged pinned Serpent
  checkout. The module-backed run did not repeat the earlier unchanged lint check.
- Deleted the temporary `serpent-module-check` workspace through Coder. The
  restrictive template namespace and policy remain ready for the demo. The
  unrelated policy's UID and complete spec were preserved.

Local module-run evidence is under `/tmp/opencode/serpent-module-*` and
`/tmp/opencode/template-module-*`. The full-editor screenshot is
`/tmp/opencode/template-resources-full-editor.png`.

Backend archive/API/SDK tests, race checks, generation, and scoped lint passed.
Independent review caught uploaded Terraform state becoming authoritative on an
initial apply; archive validation now rejects root state and runtime directories,
with eight regressions and passing race/lint checks. Follow-up archive review
found no remaining findings. Frontend verification passed 60 unit tests, 49
stories, formatting, type checks, lint, and both frontend reviews. Review roles
ran sequentially because the child agent lacked subagent tools.

## Same-name editor switching follow-up

A real-browser regression reproduced workspace `main.tf` content appearing in
the shared-resource editor after leaving an inactive model behind. Monaco's
global raw-path model lookup reused the previous editor session's content.

The shared wrapper now uses session-scoped full-path URIs, disposes its owned
models on exit, and rejects stale change callbacks. Browser checks verified both
scope switching and nested same-name files, with drafts, undo, and selection
preserved. Real-model regression tests, 47 focused unit tests, 19 stories,
formatting, type checks, lint, and both frontend reviews passed. The eight review
roles ran sequentially. No live Build/Publish or infrastructure mutation was
needed for this fix. Evidence: `/tmp/opencode/monaco-model-review.md`.

## Direct typed outputs follow-up

The companion provider source is in `/home/coder/terraform-provider-coder`.
`coder_template_outputs` now exposes typed `outputs`, `id`, and `revision_id`;
there is no `outputs_json` attribute or compatibility layer. Workspace pod/PVC
namespaces read `data.coder_template_outputs.current.outputs.namespace` directly.

- Provider acceptance/race suites, protocol tests, configuration regressions,
  script tests, vet, formatting, and generated docs passed.
- Generic and Kubernetes workspace mock tests passed, including heterogeneous
  nested values and nulls without string coercion.
- Native import of workspace version `typed-outputs` and creation of
  `serpent-typed-check` succeeded. Its Terraform state contained typed outputs
  from revision `e62346e3-cad6-4000-a0b1-68cfb041a217`; both pod and PVC used
  `workspaces`. The temporary workspace was deleted through Coder.
- The restrictive shared resources were not republished. This check verifies
  output consumption, not a repeat of the earlier Serpent/network smoke tests.

Local evidence: `/tmp/opencode/serpent-typed-state-proof.json` and
`/tmp/opencode/serpent-typed-outputs-*.log`. The reproducible provider patch applies
cleanly to its documented base using `git apply --unidiff-zero`.

## Provisioning scope terminology follow-up

On 2026-10-26, the customer-facing names became **Shared resources** and
**Per-workspace resources**, with **Shared** and **Per-workspace** editor tabs.
Both are provisioning scopes within one template. API, database, experiment, and
Terraform data-source identifiers are unchanged; no state migration is needed.

- Updated UI labels, editor component names, diagnostics, provider descriptions,
  and reproducible demo documentation.
- Passed 42 frontend unit tests, 49 stories, full frontend lint/type checks,
  formatting, and both frontend reviews. All eight deep-review roles reported
  no findings.
- Visually inspected both editor scopes, shared resource management, and destroy
  confirmation, including mobile layouts. Screenshots are local under
  `/tmp/opencode/shared-*.png` and `/tmp/opencode/per-workspace-*.png`.
- Focused backend and CLI tests passed; Swagger was regenerated without changing
  API contracts.
- Provider focused/race tests, the full provider suite with the inherited
  `CODER_WORKSPACE_BUILD_ID` unset, vet, formatting, and generated docs passed.
  The regenerated provider patch reconstructs the source from its documented base.
- Generic and Kubernetes per-workspace mock tests passed with the updated provider;
  generic shared-root validation, Terraform formatting, launcher tests, and
  Markdown checks passed. No live Build/Publish or infrastructure changes were
  made for this terminology update.

## Limits

Publish applies a fresh Terraform plan, not the earlier Build plan artifact.
Terraform success is not enforcement attestation. These checks cover selected new
connections, not every possible destination or misuse of an allowed service.
Allowed policies compose additively. Payload inspection, namespace migration,
output-dependent parameter/preset/tag previews, and supported UNKNOWN recovery
are not implemented by this POC.
