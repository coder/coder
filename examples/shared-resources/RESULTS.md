# Foundation validation

This report records the Foundation ownership changes and local validation on 2026-10-27.
It does not replace the historical Kubernetes results or claim that network enforcement was retested.

> [!NOTE]
> October 28, 2026: The [current Foundation contract](README.md) supersedes the historical contract claims below about startup gates, cleanup snapshots, partial-failure recovery, output identity/types/sensitivity, and destruction protection.
> Public roots belong in `outputs`, and sensitive roots belong in the always-sensitive `sensitive_outputs`, with exact types from the same pinned snapshot.
> The observations below remain evidence of the implementation tested at the time, not validation of the current contract.
> This note does not record a rerun of any live checks.

## Versioning and output-contract follow-up

The later live tests are recorded in [Contract tests](CONTRACT-TESTS.md).
They reproduced failed-start snapshots blocking stop/delete, unrecoverable partial
Foundation applies, different outputs from the same source revision, loss of
Terraform collection types, and queued starts bypassing a later active-run gate.
All test workspaces were deleted. One UNKNOWN Foundation and its template remain
because supported recovery and cleanup are unavailable; they contain only
`terraform_data` state and no external infrastructure.

The tests left the working Serpent deployment unchanged. Before these tests, the
user-authorized live setup moved to the current default Kubernetes context,
replaced `serpent-foundation`, and provisioned `silver-manatee-30` with a real pod
and 10 GiB PVC in `workspaces`. The agent is connected and ready. The current
template version is `serpent-kubernetes-clean`; the obsolete bootstrap placeholder
was removed. Its source is in `/home/coder/foundation-demo/serpent-dev/`.
Local agent downloads use `http://10.0.2.2:3000`, reachable from the cluster VM,
with the corresponding allowance applied through the Foundation's network-policy
module. The current Foundation ID is `58d0c435-122b-4c0d-aa4a-a01f0bb12943`.

The standalone Foundation UI now provides template-style list/detail pages,
Source Code, Resources, Versions, and Runs tabs, and an actions menu with Settings
and Edit files. The full multi-file editor is retained. Resource inventory is
read from retained Terraform state, not refreshed live. Focused final validation
passed nine Foundation unit tests, one TemplateFilesPage test, 16 management
stories, frontend static checks, and browser smoke checks. Inventory backend
tests and focused Go vet also passed. Foundation implementation remains local
and uncommitted.

## Initial native lifecycle environment and safety

The local instance used `./scripts/develop.sh`, built-in provisioners, a fresh development database, and the companion provider override.
After the development harness restarted, validation resumed against that same database and existing test resources.
All infrastructure operations used `terraform_data` only.
No Kubernetes resources, namespaces, provisioner pods, service accounts, or cloud infrastructure were created or changed.
The existing Serpent configuration, startup script, and resource identities were preserved.

The branch includes upstream main at `28192213ab`, merged as `737e69fc5f` without rewriting published history.
The merge required renumbering the existing POC migrations to 000634 through 000636.
The Foundation migration is 000637.
An older local POC database with the original migration numbers must not be pointed at this build without a backed-up reconciliation of its migration history.
The existing demo database was not reset or migrated for these checks.

## Completed native lifecycle checks

| Check                           | Observed result                                                                                                                                                                                                   |
|---------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Standalone ownership            | An unattached Foundation was created, renamed, planned, and applied before association with a template.                                                                                                           |
| Settings                        | Name and routing updates did not execute Terraform.                                                                                                                                                               |
| Routing                         | A plan with a deliberately unmatched tag remained queued. Updating Foundation routing did not rewrite the queued job's routing. Explicit cancellation succeeded before acquisition.                               |
| Native jobs                     | Terraform jobs, persisted logs, and CLI job listing worked with distinct Foundation and template metadata.                                                                                                        |
| Association                     | Binding an applied Foundation to a bootstrap template created no run. Both directions of the one-to-one constraint rejected conflicting bindings.                                                                 |
| Output interface                | Real workspace state retained nested objects, booleans, numbers, and strings. The sensitive output was absent. Foundation ID, name, template IDs, revision ID, and snapshot provenance were distinct and correct. |
| Independent apply               | A second Foundation apply did not rebuild the workspace or rewrite its previously recorded state.                                                                                                                 |
| Stop snapshot                   | Stopping after the second apply retained the first build's revision and output values.                                                                                                                            |
| Explicit adoption               | The next explicit workspace start received the second applied revision and output values.                                                                                                                         |
| Shared rollback                 | Applying an older Foundation source revision restored its greeting without changing the existing workspace build or its newer pinned state. This is not a general compatibility guarantee.                        |
| Workspace rollback              | Rolling a workspace back to its bootstrap version left the complete Foundation response, including outputs, applied revision, and run history, unchanged. Starting the consumer version again succeeded.          |
| Destruction guards              | Foundation destruction was rejected while a consumer workspace existed. Foundation deletion and template deletion were rejected while shared infrastructure remained.                                             |
| State-producing source          | A newer unapplied draft could not be used for destruction. Destruction using the actual applied revision succeeded after consumer cleanup.                                                                        |
| Cleanup                         | Workspace deletion, Foundation destruction, Foundation deletion, and template deletion succeeded through native APIs. Soft-deleted history was retained.                                                          |
| Member permissions              | An ordinary member could not read, update, apply, delete, or bind the test Foundation.                                                                                                                            |
| Interrupted workspace operation | The harness restart interrupted a workspace stop. After the reaper marked it failed, an explicit stop retry and deletion succeeded without manual state removal.                                                  |

The primary lifecycle Foundation was `f8b8c5a1-7904-480e-ad84-e2e4a3627cb0`.
Its template was `4e50a2f5-de0e-4159-8a00-120ab321bd2e` and consumer workspace was `7ddaec14-7a6f-498f-af06-61ab56746cf9`.
These test resources were cleaned up.
Local detailed evidence is in `/tmp/opencode/foundation-api-results.md` and `/tmp/opencode/foundation-check/`.
Do not publish the evidence directory wholesale because runtime credentials and state require separate review.

## Findings from integration

- A bound Foundation without any saved revision initially allowed workspace starts without Foundation metadata, despite validation receiving that metadata.
  The intended rule is now explicit: binding gates all subsequent starts until the first successful apply, even if workspace Terraform does not reference outputs.
  Stop and delete must remain available using the preceding build's snapshot.
- The first live create-and-attach flow exposed missing Foundation support in the resource-ID authorization lookup.
  Creation succeeded, but the subsequent permission check failed.
  The UI retained the saved Foundation and offered attachment retry rather than creating a duplicate or running Terraform.
- An active Foundation run conservatively blocks newly scheduled workspace starts, including a read-only plan.
  Already-queued starts are not rechecked at acquisition, as the later contract tests demonstrate.
  This is a point for further product discussion, not an impact-assessment guarantee.
- Independent state does not mean independent physical effects.
  A Foundation rollback can change live infrastructure while existing workspace state still records a newer snapshot.
  Compatibility enforcement remains unresolved and is not claimed.

Subsequent live validation confirmed that a bound, newly recreated Foundation
rejects initial workspace startup until a successful apply. The corrected
resource-ID authorization path passed the completed create-and-attach flow.
The working Serpent Foundation is retained for the demo.

## Automated validation

- Provider acceptance and race suites, vet, focused Terraform tests, and protocol capability tests passed.
- All 16 example Terraform mock tests, validation for five roots/modules, formatting, Markdown checks, links, shell-block syntax, and example-package tests passed.
- The Foundation frontend passed 104 unit tests across 11 files and 63 stories across eight files, plus full static checks and both frontend reviews.
- Backend focused lifecycle, authorization, routing, migration-preservation, fixture, and audit tests passed.
  Final broad verification and the integration regression checks are tracked separately until complete.

## Limits

Cross-organization behavior has automated coverage, but live creation of a second organization was blocked by the local instance's Premium requirement.
No acquired Foundation mutation was intentionally interrupted, and no failed Terraform destroy was injected.
The cleanup retry above followed a consumer guard, not an infrastructure-side destroy failure.
Supported UNKNOWN recovery remains unimplemented.
No automatic compatibility checker, impact-planning engine, workspace fleet reconciliation, or forced upgrade policy is claimed.
