# Foundation versioning and output-contract tests

Live validation on 2026-10-27 against the existing development API at
`http://127.0.0.1:3000`, using its built-in provisioners and companion Coder provider.
These tests assess design risks, not production readiness. No compatibility
checker, recovery implementation, or application code was changed.

> [!NOTE]
> October 28, 2026: The [current Foundation contract](README.md) supersedes the historical contract claims below about startup gates, cleanup snapshots, partial-failure recovery, output identity/types/sensitivity, and destruction protection.
> Public roots belong in `outputs`, and sensitive roots belong in the always-sensitive `sensitive_outputs`, with exact types from the same pinned snapshot.
> The observations below remain evidence of the implementation tested at the time, not validation of the current contract.
> This note does not record a rerun of any live checks.

## Scope and preservation

Three isolated fixture groups exercised output evolution, failure recovery, and
snapshot identity/concurrency. Managed infrastructure consisted only of
`terraform_data` resources. The failure fixture used `local-exec` with `exit 1`.
The concurrency fixture used two temporary FIFOs and a bounded local-exec gate,
not an arbitrary sleep, to control worker acquisition.

The working Serpent Foundation, template, workspace, Kubernetes pod, PVC, and
network policy were not changed by these tests. All fixture workspaces were
deleted. Two fixture Foundations and their templates were destroyed/deleted.
One Foundation and its template remain because UNKNOWN blocks native cleanup;
that fixture has no external infrastructure. No database writes, forced deletion,
state replacement, server restart, extra workers, or cloud access were used.

Reusable source lives under `/home/coder/foundation-demo/contract-tests/` in
`shape/`, `failure/`, and `snapshot/`. Detailed evidence paths appear below.

## Findings that affect the design

### 1. Failed starts can prevent stop and delete

**Live reproduced.** A workspace successfully consumed Foundation V2. V3 renamed
`contract.endpoint.url` to `uri`. Starting the existing, previously validated
workspace template failed during Terraform planning with `Unsupported attribute`.
The workspace retained its complete V2 Terraform state, but subsequent stop and
delete also failed on the missing V3 attribute.

Restoring the live Foundation to compatible V2 did not repair stop. A separate
sequence reproduced this with a shared rollback removing a newer template's
required `routing` output: failed start, failed stop, failed delete, and still
failed delete after restoring compatible live outputs.

A successful diagnostic stop using another compatible template distinguished
three possibilities:

- Live Foundation was V2.
- Retained successful workspace state was V4.
- Last failed start had attempted V1.

The resulting Terraform state recorded **V1**. Cleanup really consumed the failed
attempt's snapshot, not current outputs or the inputs associated with retained
successful state. The failed operations themselves did not discard that state.

Implementation: `coderd/wsbuilder/templateresources.go` copies the latest build's
snapshot for non-start transitions. Build inputs and inherited Terraform state
can therefore have different provenance after a planning failure.

**Decision needed:** distinguish attempted inputs from state-associated cleanup
inputs. Merely choosing the last successful build is insufficient for partially
successful workspace applies. The state checkpoint and the inputs that produced
it need an explicit relationship.

Recovery in these fixtures required either a successful start with compatible
live outputs before cleanup, or an explicitly selected template compatible with
the failed snapshot. Neither is a general safe recovery strategy for real
infrastructure. A cleanup path should not require provisioning successfully first.

### 2. Partial Foundation apply has no supported recovery

**Live reproduced.** F1 applied successfully. F2 updated one resource, created a
second resource, and deliberately failed its creation provisioner. Logs proved
the update completed before failure. Retained inventory included the second
resource. Published outputs and `applied_revision_id` remained F1; status became
`unknown`.

Every native plan, retry, rollback, destroy, and corrected-source apply returned
HTTP 409:

> The previous mutation has an unknown outcome. Manual reconciliation is required; automatic recovery is not supported.

Workspace stop and delete succeeded with their prior F1 snapshot. Even after all
consumers were deleted, Foundation destruction/deletion and template deletion
remained blocked. Saving corrected source worked, but applying it did not.

**Decision needed:** a supported, audited reconciliation lifecycle for partial
checkpoints, with checkpoint-producing source/run provenance separate from the
last published output revision. Do not simply clear the reservation: worker
termination and the authoritative checkpoint must first be established.

**Related code-only risk:** `templateResourceOutputs` rejects output payloads over
64 KiB after Terraform reports a successful apply. The finalizer converts that
publication failure to UNKNOWN too. Infrastructure uncertainty and an invalid
output contract are not necessarily the same condition. A partial initial apply
also lacks a successful applied revision for the current destroy contract. These
additional cases were not live-injected to avoid more undeletable fixtures.

### 3. A source revision does not identify an output snapshot

**Live reproduced.** The same Foundation revision was applied twice. A
`terraform_data` resource with `triggers_replace = timestamp()` generated a new
resource ID each time. Consumers before and after those applies recorded:

- Identical revision: `3f4e7ffe-1c63-427a-b4ba-21e995b39784`.
- First deployment: `c59e1650-0eef-b381-dfe3-f91f4919c854`.
- Second deployment: `dd28e9ca-a958-ffaa-9fba-5c981fdfc48b`.

The first workspace was not rebuilt, and a later stop preserved its first
snapshot even though that shared deployment ID no longer existed. This is correct
snapshot freezing, not preservation of historical infrastructure.

**Decision needed:** separately identify source revision, producing apply run,
immutable output snapshot, and any future compatibility generation. Revision
comparison alone cannot answer whether two workspaces use the same deployment.
Current run history does not retain a first-class resulting output snapshot, and
the output data source exposes the revision rather than a producing run ID.

### 4. Terraform collection types are lost in transfer

**Live reproduced.** The Foundation explicitly exported `set(string)`,
`list(string)`, and `map(string)` values. Consumer state represented both
collections as tuples and the map as an object. A consumer using the exported set
directly in `for_each` failed template validation:

> The given "for_each" argument value is unsuitable: the "for_each" argument must be a map, or set of strings, and you have provided a value of type tuple.

The server retains JSON values but discards Terraform output type descriptors;
the companion provider infers types from JSON. This preserves nested JSON values,
not the original Terraform contract. The primitive/nested-value tests previously
reported as typed-output validation did not cover collection-type fidelity.

**Decision needed:** either carry Terraform type information alongside values, or
explicitly define a JSON-shaped interface that requires consumer conversions.
Do not promise transparent Terraform output interoperability with the current
implementation. Typed null and collection element constraints also need coverage.

**Smaller live-found bug:** the Foundation API rounded `9007199254740993` to
`9007199254740992`, while workspace Terraform state retained the exact integer.
A long decimal was also rounded in the API but remained exact in raw consumer
state. The transport to Terraform is precise; the API's decoded number view is
not. Avoid using that view for exact snapshot comparison or hashing.

### 5. The startup gate only excludes newly scheduled starts

**Live reproduced with a controlled worker gate.** One fixture occupied the
available capable worker. A second workspace start was queued while the
Foundation was healthy. Foundation routing was then changed to an unavailable
test pool, and an apply was reserved there. New workspace starts returned 409.
After releasing the occupied worker, the previously queued start nevertheless
ran successfully while the Foundation apply remained pending.

Evidence:

- Foundation run `32d998ea-23dd-45c9-92f0-5e8dc5833042`, created
  `2026-10-27T13:44:23.887795Z`, remained pending.
- Workspace build `19de98f0-7cd0-483e-9a83-414c27d2623f` started at
  `2026-10-27T13:44:24.066719Z` and succeeded.
- The unacquired Foundation run was canceled normally; routing was restored.
  Both FIFOs were removed.

**Decision needed:** define whether gating is admission-only or excludes
Foundation mutations throughout workspace execution. Rechecking at acquisition
would close this queued-start window, but would not prevent an apply from
starting after a workspace build has already acquired its snapshot.

**Code-only extension:** workspace acquisition does not recheck UNKNOWN either.
A queued start can therefore outlive a later quarantine. Actual simultaneous
infrastructure mutation and post-quarantine acquisition were not live-tested.

### 6. Validation and rollback need explicit assessment semantics

**Live reproduced:** previously successful template validation remained successful
while starts later failed against renamed outputs, changed nested types, or a
shared rollback removing newer outputs. Workspace rollback selected an older
template against the latest Foundation; it did not restore the Foundation.
Shared rollback succeeded despite breaking a newer template's next start.

These are expected consequences of the explicitly deferred compatibility
checker, not evidence that such a checker is malfunctioning. However, a green
historical validation must not be presented as compatibility with today's live
deployment. A minimum workspace version alone would not establish compatibility
with a shared rollback or with every existing consumer.

**Code-only:** imports/dry-runs select outputs at worker acquisition, whereas
workspace starts pin them at scheduling. Import/dry-run selection does not check
active-run or UNKNOWN status, and those jobs have no first-class persisted
Foundation assessment anchor. Cached import plans may incidentally retain data
source values. Validation should record which immutable snapshot it assessed.

## What behaved as intended

- Additive output evolution with changed values left existing workspace builds
  and Terraform state untouched. Explicit starts adopted the new outputs.
- Successful historical stop preserved the prior snapshot before any failed
  adoption attempt.
- Numbers, booleans, strings, nested JSON objects/arrays, and nested null survived
  transfer. Sensitive root outputs were excluded from the consumer. Top-level
  null was absent as in ordinary Terraform output behavior.
- Failed Foundation plans, including an apply that failed during pre-apply
  planning, preserved the prior published outputs and allowed subsequent starts.
- Shared rollback did not rebuild workspaces; workspace rollback did not apply
  or mutate the Foundation.
- Destruction used the applied revision rather than the latest saved draft.
- Consumer guards and UNKNOWN quarantine remained fail-closed. No state was
  dropped to manufacture successful cleanup.

## Evidence and residual fixture

Local detailed reports:

- `/tmp/opencode/foundation-contract-shape-results.md`, with actual state snapshots
  and build/run logs under `/tmp/opencode/fcontract-shape/`.
- `/tmp/opencode/foundation-contract-failure-results.md`, with evidence in
  `/tmp/opencode/foundation-contract-failure-evidence.jsonl`.
- `/tmp/opencode/fcontract-snapshot-ledger.json` and
  `/tmp/opencode/fcontract-snapshot-set.log`.

The intentionally partial failure left only:

- Foundation `fcontract-failure-ece894-update`, ID
  `8e76702b-c48b-43e4-903b-3ee213271ef5`.
- Template `fcontract-failure-ece894-tpl`, ID
  `b2d6d5ba-4869-4e70-8853-f0299e15fe05`.
- Two `terraform_data` instances, with no external infrastructure or active
  provisioner process. Its consumer workspace was deleted.

All other fixture Foundations/templates/workspaces were cleaned up through normal
APIs. Existing soft-deleted history remains. Review local evidence before sharing
it wholesale; do not publish unrelated state or credentials.

## Recommended order

1. Resolve state-associated cleanup provenance and supported partial-apply
   recovery before treating the lifecycle as safe.
2. Define immutable applied-snapshot identity and the exact output type contract
   before implementing version compatibility or rollout assessment.
3. Decide execution exclusion semantics and persist validation assessment anchors.
4. Then design compatibility policy and rollback UX around those primitives.

These findings do not require abandoning independently managed Foundations. They
show that source versions, live deployments, state checkpoints, output snapshots,
and compatibility assessments cannot safely be represented by one revision ID.
