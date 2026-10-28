# Foundation phase-1 validation, 2026-10-28

This report covers the phase-1 implementation follow-up.
It supersedes conflicting implementation claims in the older validation reports, but preserves their historical observations.

> [!NOTE]
> The [current output contract](DESIGN.md#metadata-outputs-and-snapshots) supersedes the whole-object sensitivity limitation recorded below.
> `outputs` contains public roots only, and `sensitive_outputs` contains sensitive roots only and is always sensitive.
> Both retain exact types from the same pinned snapshot, so public `for_each` no longer needs `nonsensitive(...)` for unrelated secrets.
> Earlier whole-object observations remain historical. The split interface was verified separately in the follow-up below.

## Split-output follow-up

- The provider exposes public roots in `outputs` and sensitive roots in always-sensitive `sensitive_outputs`, including empty objects. It validates the entire snapshot before publishing either bucket and rejects missing or ambiguous sensitivity metadata.
- Real Terraform and a native Coder import/START consumed a generated credential through `sensitive_outputs` while using a public set directly in `for_each`. Two public instances were created without `nonsensitive(...)`.
- Private workspace state confirmed disjoint buckets, exact integer `9007199254740993`, `set(string)` preservation, intended secret consumption, and the same producing snapshot ID. Import, build, Foundation, and server logs checked did not disclose the secret.
- A secret-removal Plan identified the consumer's `sensitive_outputs.credential` reference without exposing its value. Native STOP, DELETE, Foundation Destroy, and final metadata cleanup all succeeded. The fixture used only `terraform_data`; no external infrastructure was created.
- The detail view lists output names, types, and sensitivity. Public values are visible; sensitive or unknown-sensitivity values use a fixed `********` mask without reading their values. Live desktop/mobile checks and accessibility checks passed.
- Provider tests, targeted provider race tests, provider vet, companion-patch reconstruction, all 16 example tests, 29 Foundation UI unit tests, 26 stories, frontend checks/lint/format, and scoped code/frontend reviews passed. Coder Foundation race tests and full provisioner-server, runner, workspace-builder, provisioner SDK, and client SDK suites also passed.
- Known display limitation remains: normal JavaScript JSON parsing rounds integers outside its safe range. The live UI displayed `9007199254740992` for the exact backend value `9007199254740993`; this follow-up does not claim lossless browser-number display.

Local browser evidence: `/tmp/opencode/foundation-outputs-live-*.png`. Provider override: `/home/coder/foundation-demo/provider.tfrc`. The development server was started through `scripts/develop.sh` and remains available on API port 3000 and UI port 8080; Serpent was not rebuilt or modified during this follow-up.

## Environment and boundaries

- Coder worktree: `feat/workspace-network-policies`, based on `737e69fc5f`; changes remain local and uncommitted.
- Development uses the existing `scripts/develop.sh` and `scripts/coder-dev.sh` entry points.
- The retained demo uses local PostgreSQL on port 5435. A private backup was taken before applying migrations 638 through 641. The separate built-in development database was used for editor-only browser checks.
- Kubernetes context: `default`, local API at `https://127.0.0.1:16443`.
- AWS credentials are excluded from the development process. No AWS infrastructure was created or changed.
- Runtime connection configuration is private at `/home/coder/foundation-demo/runtime.env`. Terraform source remains outside `scratch/` under `/home/coder/foundation-demo/`.
- The historical UNKNOWN Foundation `8e76702b-c48b-43e4-903b-3ee213271ef5` remains quarantined. It was not reset, discarded, or silently recovered.
- No backwards-compatible workspace snapshot backfill is provided. Existing successful demo workspaces must establish state-associated provenance through a new successful start before exercising cleanup with the new contract.

## Completed checks

### Automated checks

- Full generation completed, including database, protobuf, API types, audit mappings, and golden files.
- Full `wsbuilder`, `provisionerdserver`, runner, jobreaper, provisioner SDK, and client SDK suites passed after the live-discovered integration fixes.
- Foundation management, checkpoint-based Destroy/cancel/retry, database persistence, consumer counts, authorization, source-reference analysis, preview sanitization, output redaction, and exact-number tests passed.
- Targeted race checks passed for state provenance, Foundation recovery, and preview analysis.
- Frontend: 49 unit tests, relevant Storybook tests, `pnpm check`, full `pnpm lint`, and both frontend reviews passed.
- Provider patch reconstructs the relevant source against `e44b8cc9304e107f07bb4b34dd9df059c230a04d`; reconstructed provider/helpers/scripts tests and build passed.
- All 16 example Terraform mock tests and root/module validation passed.
- Broad `go vet` found an unchanged `upload_file_test.go:133` protobuf lock-copy warning. This is not a clean broad-vet result.

Final post-fix full generation, the six-package backend regression suite, and full race suites for `wsbuilder`, `provisionerdserver`, runner, and jobreaper passed. Race tests require `CGO_ENABLED=1` in this environment. The Terraform authority and partial-failure tests also passed with race detection. The broader Terraform suite passed excluding `TestProvision`, which encountered Terraform's provider-mirror panic; this is not a clean full Terraform-suite result.

### Real browser editing

Using the isolated development database, browser interaction verified Foundation creation, navigation between Templates and Agents, full multi-file editing, file creation/rename/deletion, save persistence after reload, separate Plan and Apply controls, save-only behavior, editor height adjustment, and the default-on protection setting.

No Terraform runs were started through the browser. A second read-only browser session on the retained database verified populated resource/output panels, visible sensitive-value redaction, deleted-output and unresolved-reference warnings, the fresh-plan disclaimer, and association-only template pages. A notification toast from the first session did not recur with the correct development cookie; it was an authentication setup issue, not a verified Foundation defect.

Screenshots and the detailed report are retained locally in `/tmp/opencode/foundation-ui-20261028/`.

### Retained Serpent deployment

- Confirmed source commit `40b60bf4c1495caddc0292f862de275da292e6aa`, Go 1.21.13, golangci-lint v1.53.3, and clean source tree.
- Original Makefile SHA-256: `822d6c2b97cc770c7244f0eb1fa8d971d4fa872b6c4ff6251e47a6d7cf980175`.
- `make lint` reproduced exit 2 and exactly nine expected findings: five errcheck, three loopclosure, and one SA1019.
- `github.com`, `proxy.golang.org`, and `sum.golang.org` returned HTTP 200 over HTTPS from the pod. The denied test hostname `example.com` did not resolve.
- Updated Foundation source to `coder_foundation`, preserving the existing policy identity. Plan and Apply reported zero additions, changes, or destructions. Both published outputs now carry string type metadata.
- After the import-metadata fix, a fresh workspace version imported, was promoted, and successfully updated the existing workspace through START. The agent is connected, ready, and healthy.
- Ordinary START rotated the agent token and replaced the pod and dependent `coder_metadata`. This is existing provider behavior, not a Foundation regression: `provider/agent.go` is byte-for-byte identical to the provider baseline. The template embeds that token in immutable pod environment variables. Only prebuild claims receive the token-reuse override.
- The namespace, network policy, and 10 GiB PVC retained their original Kubernetes UIDs. The replacement pod retained the same source, tools, Makefile, and nine expected lint findings. A local read-only plan again proposed token-driven replacement, so no second apply was issued.

### Live Kubernetes Foundation

Foundation `166d70a0-0922-4a0f-bb09-22a7df16d403` created the isolated namespace `foundation-validation-contract`.

- Plan succeeded without applying infrastructure and persisted a value-free advisory preview, including the sensitive/unknown flags.
- Apply succeeded. The ordinary API omitted the generated sensitive value while exposing its sensitivity metadata.
- JSON preserved integer `9007199254740993` exactly and described the collection as `set(string)`.
- Reapplying the same source succeeded with a distinct output snapshot ID identifying the new producing run.
- The generated secret was present in the private snapshot and absent from the checked Foundation list/detail responses, run logs, and server logs. The secret was not printed in the test output.
- A real Terraform plan rejected direct `for_each` over the sensitive snapshot's public set. Wrapping only that known-public set in `nonsensitive(...)` successfully planned two instances. Neither plan disclosed the generated secret, and neither was applied.
- Consumer import exposed the metadata-copy defect below. After the fix, a fresh import and real Kubernetes workspace start succeeded.
- Private workspace state preserved the exact integer and consumed the generated credential. Values were checked internally, not printed.
- Removing the required `endpoint` output produced an advisory preview flagging its deletion and unresolved references. Apply succeeded without compatibility enforcement.
- The next workspace start was accepted, then failed in Terraform. Stop succeeded with the original state-associated snapshot rather than the incompatible attempted snapshot. Database inspection confirmed the distinct attempted and retained-state snapshot IDs.
- Default-on Destroy protection returned HTTP 409 for both the running and stopped consumer.
- Reapplying the older source restored the removed output, produced a new snapshot, and allowed workspace START and STOP to succeed. No historical state restore was used.
- Disabling protection permitted Destroy with a stopped consumer. Destroy cleared current outputs while retaining identity, history, and association. Reapplying source recreated the Foundation infrastructure with the same Foundation identity and a new snapshot.
- A second Destroy followed by Delete allowed attachment of a replacement Foundation. The old workspace then deleted successfully using its retained inputs even though the original Foundation was deleted and the replacement had never applied.
- Native cleanup removed the test workspace, both Foundations, and template. Kubernetes confirmed that `foundation-validation-contract` no longer exists.

### Live empty-output consumers

- No-reference and optional `try(...)` consumers imported and started successfully against an attached, never-applied Foundation when imported after attachment.
- Missing required output validation returned HTTP 201 and subsequently failed in Terraform with `Unsupported attribute`, not HTTP 409.
- Preparation Apply initially exposed the metadata-copy defect below. After the fix, the fresh required-output import succeeded; the original failed import remained failed for diagnosis.
- Import-before-attachment initially exposed the routing defect below. After the fix, those workspace starts succeeded without reimporting.
- A required-output workspace succeeded with published outputs, then returned HTTP 201 and failed only in Terraform after the output was removed.
- A consumer started during an active Foundation Apply using previously published v1, while that Apply subsequently published v2. After a completed partial Apply failed, another consumer start succeeded using published v2 rather than attempted v3.
- All `fv-empty-` and `fv-activity-` test workspaces, Foundations, and templates were cleaned up through native APIs.

### Real partial-checkpoint recovery

Four scenarios executed 15 real Terraform jobs, with no state resets or direct database mutations:

- Initial partial Apply retained resource inventory and a checkpoint without publishing outputs. Destroy used that checkpoint and succeeded.
- Corrective Plan/Apply recovered a failed initial Apply, preserved the retained resource identity, and published the corrected outputs.
- A failed Destroy removed one resource and retained another. Corrective Apply preserved the remaining resource identity; subsequent Destroy succeeded.
- A sensitive output exceeding the 64 KiB publication limit failed publication but retained a usable checkpoint. Smaller-output Plan/Apply preserved the resource identity and recovered normally.

All four recovery Foundations remain as destroyed metadata records with run history and no infrastructure. The pre-existing UNKNOWN fixture remains quarantined and unchanged.

## Defects found during integration

These findings explain why passing isolated tests alone was insufficient:

1. New workspace provenance fields initially lacked audit registrations, preventing enterprise startup. Added audit mappings, with secret snapshot values ignored.
2. Optional snapshot SQL parameters initially serialized empty bytes as invalid JSON. Added safe defaults and regression coverage.
3. Destroy and queued cancellation initially used or preserved the wrong revision field. Destroy now uses the retained checkpoint revision; queued cancellation preserves it.
4. The runner initially marked every failed Foundation apply UNKNOWN, making server-side partial-checkpoint recovery unreachable. Added an explicit authoritative-state signal, runner regression tests, and successful live partial Apply/Destroy recovery checks.
5. Template import reconstructed START/STOP metadata but omitted Foundation identity, types, sensitivity, and snapshot ID. This caused the provider to reject nonempty output snapshots. Fixed forwarding, added regression tests, and verified fresh imports and workspace starts live.
6. Versions imported before attachment did not request a capable worker for a never-applied Foundation. Fixed routing independently of whether a revision is published, added regression tests, and verified the original failing scenario live.

## Deliberate limitations

- Apply freshly plans; it does not execute an earlier saved Plan artifact.
- Previews and source-reference warnings are advisory. Modules, dynamic references, and unsupported source forms can remain unresolved.
- **Superseded by the split output contract:** the tested provider marked the entire `outputs` attribute sensitive when any output was sensitive because the protocol cannot mark arbitrary dynamic keys individually.
  The recorded public `for_each` test required `nonsensitive(...)` with that provider, not with the current split interface.
- Exact numeric transport does not imply exact browser display: ordinary JavaScript JSON parsing can round integers beyond its safe-integer range.
- Preparation uses a failed import retained for diagnosis followed by a fresh import. There is no first-class deferred-validation state or same-job resume.
- Genuinely uncertain execution remains fenced for exceptional manual recovery.
- No compatibility enforcement, atomic Foundation/workspace release, automatic fleet upgrade, or automatic workspace rollback is provided.

Raw database backups, Terraform state, runtime configuration, and complete diagnostic directories must not be published wholesale.
