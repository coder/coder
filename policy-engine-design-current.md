# Coder AI Gateway — Policy Engine (Current Design)

A concise statement of the policy engine's design as it stands: what it is and
why. Where a piece is designed but not yet implemented, it is marked
*(designed, not built)*. Transitional history and superseded decisions are
omitted; see `policy-engine-design.md` for the full record.

> **POC posture.** This is a proof of concept: BC breaks are fine (rename enum
> values, edit shipped migrations in place, wipe dev DBs); no aliases or
> compatibility shims. Simplicity and clarity of the abstractions is the goal.

## 1. What it is

A policy engine inline in the AI Gateway request path. Requests flowing through
a provider route are subjected to configurable policies that **allow, log,
block, or transform** them. Policies are safe on untrusted end-user input,
fast, validatable, testable, and configurable as code (API / Terraform / UI).

## 2. Substrate: Rego via embedded OPA

Policies are written in **Rego**, evaluated by **OPA embedded as a Go library**
(`rego.PrepareForEval` prepared queries, native topdown interpreter; never the
OPA server, never Wasm, which is 3-4x slower here).

**Why Rego:** Coder already ships OPA, so the engine, team familiarity, and
tooling (`opa test`, `opa fmt`, decision logs) are reused with no new
dependency or language. Rego is expressive (comprehensions, negation,
multi-rule derivation), supports operator-authored reusable modules, handles
whole-object transforms cleanly, and is non-Turing-complete and terminating
with linear-time RE2 regex, so it is safe against untrusted input.

**Accepted trade-offs:** Rego is dynamically typed (a typo'd field is silently
`undefined` at runtime rather than a compile error) and Rego ≈ OPA (engine
lock-in). Compensating controls: schema-checked compilation, per-kind smoke
tests, and Go shape guards (§7, §8).

**Hermeticity is enforced, not assumed.** Policies make no network calls and
share no state. Every policy is compiled *and* evaluated against a restricted
OPA capability set: the base set for the OPA version with every builtin OPA tags
**non-deterministic** removed (`http.send`, `time.now_ns`, `rand.*`,
`uuid.rfc4122`, `net.lookup_ip_addr`, `opa.runtime`, ...). A policy referencing
one is rejected as an undefined function at the validation gate (compile, §8)
and again at load (prepare), so it can never evaluate non-deterministically,
rather than failing only at runtime. Evaluation is therefore pure and
deterministic. Policies compose only through host-threaded annotations (§5),
never by calling each other. External signals enter exclusively via guardrails
(§6).

## 3. Stage model: two axes, one result type

Every pipeline member is a **stage**, classified on two orthogonal axes:

- **Substrate:** hermetic Rego policy vs networked adapter (guardrail).
- **Effects:** which of the engine's effects it may produce (verdict, message,
  annotations, edits, route).

**Every stage yields one `StageResult`** — `{verdict, message, annotations,
edits, headers, route, err}` — through a uniform
`Evaluate(ctx, Input) StageResult`, but stages never construct it freely. Each
kind decodes its Rego output into a **typed per-kind struct** (`Decision`,
`Annotations`, `RouteChanges`, `Transformation`) implementing a **`Projector`**
(`Project() StageResult`); the guardrail adapter result funnels through the same
interface via a `GuardrailOutcome`, and even the failure and undefined-rule
paths are Projectors (`Failure`, `noop`). Effect masks are therefore enforced
**by construction** (a `Decision` has no edit field to populate; an adapter
physically cannot emit LOG), not by runtime checks. Crucially, `Project` has
**no access to the stage's name**: it emits only raw effects. A single host
function, **`Resolve(name, projector)`**, stamps the identity, nesting the raw
annotations under the member's immutable name and labelling the audit-only
failure record, so a stage cannot **choose, omit, or spoof** its namespace.
`Resolve` is the one and only way a `StageResult` acquires a name.

A **kind** is a single-effect Rego stage. Four kinds exist:

- **decide** → `verdict` (+ optional `message`). Verdicts reduce by
  `BLOCK > LOG > ALLOW`; BLOCK short-circuits and returns HTTP 400 with the
  message (or a generic default; a buggy message can never alter the verdict).
  LOG writes to the log stream and passes through.
- **transform** → rewritten `body` and/or `headers`. The host applies the
  change and **re-validates** the mutated body against the provider schema
  before forwarding; header transport/auth/hop-by-hop stripping prevents
  credential injection.
- **annotate** (renamed from `classify`: the contract is "emit annotations",
  matching its `annotations` entrypoint rule) → annotations only, threaded
  downstream by the host.
- **route** → within-provider model override (cross-provider routing is
  deferred).

A **guardrail** is a multi-effect networked stage pinned to the head-of-hook
slot. It is deliberately *not* a kind: it differs in substrate (network
adapter, concurrent, its own timeout and secret-bearing config), and its
multi-effect result is a concession to vendor wire formats (one HTTP response
carries score + mask + verdict and cannot be un-bundled). Hermetic kinds stay
single-effect; there is no reason to emulate the bundling in Rego.

**One mutation algebra: edits.** All body mutation is a list of pointer/value
edits; a whole-body rewrite is the degenerate root edit. Transform and
guardrail masking share one representation, one applier (edits applied in
stage order), one re-validate per mutation point, and edit-level audit
granularity. *(Designed, not built: transform currently replaces whole
bodies.)*

**Reducer rule: BLOCK freezes effects, never erases observations.** On BLOCK,
verdict and message are final and no later mutating effect (edits, route)
applies, but annotations from every stage that actually ran are always kept
for audit. Sequential chains stop scheduling at BLOCK; a concurrent guardrail
batch runs to completion and reduces normally (cancelling siblings would make
the merge nondeterministic for little gain).

**Failures are just another projection, not a parallel error path.** A failed
invocation (eval error, network error, the global 1s eval timeout, decode
failure) is replaced at the stage boundary with a **`Failure` Projector** that
projects under the stage's fail mode exactly as a success would: `fail_closed →
BLOCK` with a generic message; `fail_open → LOG` (LOG, not ALLOW: a fail-open
outage must be visible, not silent). The error rides an audit-only field on
`StageResult`; the failing stage's identity never reaches the client message
(telling an adversary the DLP scanner is down invites retries until it stays
down). Because success and failure share the one `Project` path, the reducer is
total over `StageResult`s — no error branches, nothing to drift. No failure
class bypasses `fail_mode`.

## 4. Hooks and pipelines

Each **provider** has at most one **pipeline**: a single versioned unit
spanning the hooks, with each member stage pinned to a hook. The whole
pipeline is the atomic swap unit (§8). No pipeline = pass-through (matches
pre-engine behavior; visibility comes from metrics, not a default-deny).

v1 hooks (post-resp and output inspection are deferred):

| Hook | Envelope | Valid kinds |
|---|---|---|
| **pre-auth** | raw request, headers, credentials; no identity | annotate, decide |
| **pre-req** | + resolved identity/groups/roles, body, annotations | annotate, route, decide, transform |
| **pre-tool** | `identity` + `tool_call` {id, name, arguments, index} + annotations, per call (no request body/headers) | annotate, decide |

Pre-req is the richest hook and deliberately drops the raw credential (it is
resolved into identity by then; re-exposing the secret is needless attack
surface). The two decision-only hooks reject request-mutating kinds both at
registration and defensively at load via constrained pipeline constructors, so
a smuggled route/transform cannot mutate anything.

**Per-hook ordering:** `guardrails → annotate → route → decide → transform`.

- **Guardrails run first as a security invariant**, not a scheduling default:
  a masking guardrail must precede every Rego stage that reads the body,
  otherwise an annotate policy could copy unmasked PII into annotations and
  thence into audit/telemetry. Placement is not operator-choosable.
- **Annotate before the other policy stages** so its annotations are visible to
  later stages and later hooks. This is how policies compose while staying
  hermetic.
- **Decides run sequentially** (ordered by name) and reduce with a BLOCK
  short-circuit; per-policy attribution after a block is best-effort.
- **Transform runs last**, validate-after-mutate.

## 5. Envelope and annotations

The host-built envelope is typed and frozen by shape guards (§7):

- `input.request` = `{method, path, body}`; `body` is the provider-native
  request body, opaque to the shape guard (its shape is the provider's
  contract, not ours).
- `input.identity` = `{id, username, groups[], roles[]}`, decoupled from
  upstream-forwarded actor metadata so it cannot leak to the provider; arrays
  always materialized, never undefined.
- `input.headers` = lowercase header → first value (+ `x-remote-addr`).
- `input.annotations` = threaded annotate/guardrail outputs, seeded `{}` at
  every hook so reads are defined-but-empty.

**Annotation namespacing.** The host owns the first level of the annotations
map: every producer writes under its own stage name
(`input.annotations.<stage_name>.<keys>`), with name collisions rejected at
pipeline-version create. A producer attached at multiple hooks replaces its
namespace wholesale at the later hook (no deep-merge: merged documents nobody
authored are unpredictable). Stage **names are immutable at create**
(`display_name` is the mutable label; a true rename is a fork) because names
key annotation paths consumed by downstream policies, and a rename would
silently turn those reads `undefined`, Rego's worst silent-failure mode.
Enforcement is structural: `Resolve(name, projector)` (§3) is the sole site that
nests a stage's raw annotations under its name, so a stage cannot write outside,
omit, or spoof its namespace, the `Project` step never sees the name.

## 6. Guardrails (networked head-of-hook stage)

Guardrails integrate external safety/DLP vendors (Presidio, Bedrock, Lakera,
OpenAI moderation, ...). There is no industry-standard guardrail I/O format, so
they integrate via per-vendor **adapters composed over a base** modelled on
litellm's Generic Guardrail API (extract texts → POST → block / pass /
intervene-with-modification); any generic-API-compatible vendor works with
zero per-vendor code, and materially-better native APIs get first-class
adapters.

**Authority is intrinsic, not a mode.** A guardrail's effects are whatever its
adapter returns; there is no advisory/enforcing toggle. A scanner that only
emits annotations (e.g. a moderation score) leaves the block and edits empty,
and a Rego `decide` turns the signal into a verdict (e.g. "block contractors
over moderation score X, log admins") — this is also the engine's *only*
enrichment mechanism: policies never fetch. A guardrail whose adapter returns a
block (HTTP 400, same as a policy block) and/or edits is the zero-Rego
"block/mask on detect" path; annotations always thread regardless. Observe-only
rollout of a blocking adapter is handled by version-targeted rehearsal (§9) and
the membership `enabled` flag, not a per-membership mode. *(A later
concurrent-with-dispatch / output lane will be advisory by a structural
invariant — it cannot influence a request that has already happened — which is a
lane property, not this removed mode.)*

**Execution.** A hook's guardrails run **concurrently** (network-bound, unlike
CPU-bound Rego), and the batch runs to completion even when one blocks, then
reduces under the shared rule (§3): annotations union under per-guardrail
namespaces (kept even on BLOCK); a BLOCK freezes mutating effects; edits apply
as a deterministic ordered chain (by guardrail name) with one re-validate
after the guardrail stage (the policy transform then re-validates its own
mutation: two mutation points, each re-validated). Each guardrail carries its
own network timeout and `fail_mode` (default fail-closed); failures project
through the `Failure` Projector into ordinary StageResults (§3). No retries, no
response caching in v1.

**What is scanned:** the **latest user prompt**, selected cross-provider as the
most recent role-`user` item carrying a text block (trailing system/tool-result
turns are skipped, since agentic clients append them after the user's prompt).
Only the latest user turn is scanned; earlier-turn PII in resent history is not
re-masked.

v1 ships serial **pre-req input guardrails** only (injection detection, PII /
secret masking, tool gating, moderation→annotations→decide). Output guardrails
ride the deferred post-resp machinery: a post-call guardrail on a stream *is*
response buffering, since a flushed chunk cannot be un-sent.

## 7. Pre-tool gating

The **pre-tool** hook is the last control point before a model-requested tool
call reaches the client, where agentic clients execute it. It fires once per
assembled client-bound tool call (annotate + decide only). Its envelope is
deliberately narrow: `input.tool_call.*` (the subject of the decision),
`input.identity.*` (RBAC inputs), and threaded `input.annotations.*`, but **not**
the request body or headers, since the gate's subject is the individual tool
call, not the original request. `index` makes "at most N calls per turn"
expressible without state.

Tool calls are splayed across many SSE events, so the interceptor **holds** a
client-bound tool block's events from start to completion while text outside
the block streams through untouched. On ALLOW the held events flush in order;
on BLOCK they are discarded and the **whole turn terminates** with a
provider-shaped error naming the tool and policy (per-call suppression would
require fabricating stop-reason/index consistency, so it is deliberately not
done). Non-streaming responses gate identically and return HTTP 400 on BLOCK,
so a client cannot bypass the gate by disabling streaming. Holding engages
only when a pre-tool pipeline is configured; otherwise streaming is
byte-for-byte unchanged.

The hold is bounded: per-stage 1s eval timeout (normalized through fail mode),
a 4 MiB byte cap, a 5 min wall-clock deadline (hardcoded, generous). A cap
breach honors the gate's **aggregate fail mode**: fail-open only when *every*
pre-tool member is fail-open. Operators should roll out in LOG before BLOCK,
since a false positive terminates a live agent turn.

## 8. Validation

Rego's dynamic typing means correctness is enforced at the gate, not by the
language. Validation is synchronous, in-process Go (no `opa` CLI), runs before
the write transaction, and only valid rows ever persist:

- **Compile with the hermetic capability set** (`ast.Compiler.WithCapabilities`):
  rejects any reference to a non-deterministic builtin (`http.send`,
  `time.now_ns`, `rand.*`, ...) as an undefined function, enforcing §2's
  hermeticity at the gate. The same set is reused at load (prepare).
- **Compile with schemas** (`ast.Compiler.WithSchemas`): catches typos and
  hook-inappropriate field references against per-hook input schemas
  *(designed, not built)*.
- **Kind/output binding:** assert the declared kind's entrypoint rule exists
  (`verdict`/`model`/`annotations`/`body`), `decide` has a `default verdict`,
  and the output conforms to the kind's contract on standard smoke inputs.
  Closes the "wrong-kind / typo'd rule silently no-ops" hole.
- **Per-kind smoke tests** via OPA's `tester` package.
- **Pipeline structure in Go:** kind-cardinality per hook, kind-validity by
  hook, referenced versions exist; the loader re-checks defensively so a
  direct DB write cannot smuggle an invalid posture into the runtime.
- **Guardrail config** validates by parsing through the concrete adapter
  constructor (structural validation by construction, not a schema registry).
- *(Designed, not built:)* cross-table member-name collision rejection, and
  best-effort **annotation-flow warnings**: an advisory guardrail nobody reads
  (a silently dead DLP paying vendor latency for nothing) and a `decide`
  reading a namespace nobody produces (the typo case Rego fails silent-open
  on). Warnings, never rejections: dynamic addressing defeats the static walk,
  and "guardrail today, consuming decide tomorrow" is legitimate staging.

**Backward compatibility is structural, not versioned-at-runtime.** The host
always builds the *current* envelope shape; the contract is **never remove,
rename, or retype an input field** (additions only) and **widen, never narrow,
output contracts**. Old policies therefore always find the paths they read. Go
shape-guard tests pin the envelope skeleton and each kind's behavioral output
contract against golden files, and assert any shape change bumps the schema
version stamp, so a contract change cannot land silently. The stamps
(`input_schema_version`/`output_schema_version` on each policy version) are
forensic, read by audits, not the runtime. Accepted residual risk: *adding* a
field can flip a policy that probed the previously-undefined path; additions
are rare, reviewed, and caught by LOG-first rollout.

## 9. Persistence and versioning

Postgres, deployment-global (no `organization_id`; the attach target
`ai_providers` is global). Three first-class versioned entities, all on the
`templates`/`template_versions` pattern: a parent row with an
`active_version_id` pointer plus immutable version rows
(`UNIQUE(parent_id, version_number)`; edits insert, never mutate; composite
FKs guarantee a parent can only activate its *own* versions):

- **Policies** (`ai_gateway_policies` / `_versions`): reusable library
  content; versions store **raw Rego text** (prepared queries are not
  serializable; `aibridged` recompiles on load). `kind` is intrinsic and
  immutable: a kind is a semantic role, so a kind change is a new policy.
- **Pipelines** (`ai_gateway_pipelines` / `_versions`): one per provider; the
  atomic whole-posture swap unit. Membership rows pin exact
  `policy_version_id`s (composition history is exact; rollback is possible)
  and carry per-membership `hook`, `fail_mode`, and `enabled`, since one
  reusable policy can run differently in different pipelines. Per-hook
  cardinality (one annotate/route/transform, many decides) is enforced by
  partial unique indexes over a denormalized `kind` column.
- **Guardrails** (`ai_gateway_guardrails` / `_versions`): same pattern but
  storing adapter config, not Rego. The credential column is the **one
  dbcrypt-encrypted field** in the system; Rego itself is code, meant to be
  readable, diffable, and decision-logged, so policies are never encrypted
  and embedded secrets are documented-unsupported. Guardrails join pipeline
  versions through a dedicated membership table (no kind column, no
  cardinality cap; many concurrent guardrails per hook).

Enable/disable exists at exactly two levels: the **pipeline** (whole posture
off) and the **membership** (one policy/guardrail off within one pipeline,
which mints a new version since rows are immutable). A policy has no global
on/off flag because a reusable definition has no standalone on/off meaning.
Parents soft-delete only; version rows are retained indefinitely (FKs, history,
and audit depend on them). Deleting a policy is blocked while an active
pipeline version references it.

**RBAC and audit:** one owner-only `ai_gateway_policy` RBAC resource covers
everything. Policies, pipelines, and guardrails are audited; the
`active_version_id` repoint is the most security-relevant action, and pipeline
audit diffs render the full member posture that went live.

### Rollout: mint, then promote

Changing what runs is explicitly two-stage *(designed, not built; current code
auto-activates)*:

- **Version creation defaults to `activate=false`**: it mints an immutable
  version and changes nothing anywhere.
- **Activating a policy version** re-pins every referencing pipeline by
  minting a new pipeline version on each pipeline's **tip** (so staged changes
  accumulate as one linear draft lineage), but does **not** promote. Every
  activation returns a **propagation report** so "editing didn't change what
  runs" is loud, not surprising.
- **Promotion** (repointing `active_version_id`, same path as revert) is the
  only action that changes live posture. An opt-in `promote: true` collapses
  mint+promote for urgent hole-patches. The promote-time live-vs-candidate
  diff is the safety net showing everything that would go live.
- The drift gauge (pipeline pins ≠ policy's active version) is the
  "unpromoted changes exist" workqueue indicator.

### Version-targeted evaluation

*(Designed, not built.)* Operators rehearse an unpromoted version against real
traffic with an owner-only header (`X-Coder-AI-Gateway-Pipeline-Version: 3`),
addressing the logical version number, provider-scoped. The engine's behavior
is never forked: no shadow or dry-run mode; what you test is byte-for-byte
what you promote, including real guardrail calls, real failure synthesis, and
real blocks. Only the audience differs. Non-owners get 403, never a silent
fallthrough (a typo'd staging test must fail loudly, not quietly exercise
production). A header rather than a route/query param because SDKs mangle base
URLs but trivially support `default_headers`. Rollback rehearsal falls out for
free by targeting an older version. The same validation gate is also exposed
as a headless, side-effect-free dry-run for external CI *(designed, not
built)*: CI checks the artifact pre-merge; the header rehearses it pre-promote.

## 10. Runtime

- **Topology:** `coderd` validates + writes + publishes; `aibridged`
  subscribes + reloads (mirrors the `ai_providers` reload pattern). Publish is
  strictly post-commit so a reload cannot race a half-written change;
  pipeline + memberships + pinned versions load in one consistent read.
- **Rebuild-all on any change** (compilation is cheap; expected scale is ~3-5
  pipelines), with a ~5m periodic safety reload to converge after a lost
  NOTIFY or read-replica lag. On a reload compile error the **last-good
  snapshot is kept** (alerted via metric; near-impossible given the gate).
- **Per-provider snapshot:** provider → compiled per-hook pipelines, each
  holding its Rego stages and guardrail adapters (constructed once at snapshot
  build, decrypting credentials then, so per-request cost is the network call
  only). The snapshot query excludes disabled/soft-deleted members, so any of
  those removes the member from what runs.
- **Compile-once, eval-many:** prepared queries cached by immutable
  `policy_version_id`, shared across reloads and across pipelines. Per-request
  cost is evaluation only: tens of microseconds for indexed decisions. OPA
  eval is CPU-bound and single-threaded per query, so concurrency is at the
  goroutine level.
- **Atomic swap:** repointing `active_version_id` swaps the live snapshot in
  one step; in-flight requests keep their version; retired snapshots are
  reclaimed by Go GC. DB rows are never deleted.
- **Bounds:** 1s per-stage eval timeout (honoring `fail_mode`), OPA eval
  limits, and a host-side size gate on large bodies.

## 11. API, UI, observability

- **HTTP:** enterprise CRUD at `/api/v2/aibridge/{policies,pipelines,guardrails}`
  (+ `POST .../{id}/versions`), gated by the AI Governance feature; `codersdk`
  client and Terraform (via the `coderd` provider) use the same
  ingest/compile/store path as the UI.
- **UI:** `/ai/settings/policies`: policy list with a Monaco Rego editor and
  version-history revert; pipeline list with membership management (policies
  and guardrails, per-membership mode/fail-mode/enable); guardrail management
  with write-only credentials. Membership edits mint unpromoted drafts; an
  "Unpromoted vN" badge and a prominent "Promote vN" button make promotion
  volitional.
- **Authoring tiers:** canned registry policies (taxonomy-tagged to OWASP LLM
  Top 10 / NIST AI RMF) → form builder → raw Rego → LLM-assisted.
- **Observability:** reload result/duration, per-provider live-version gauge,
  drift gauge, eval-duration histogram aligned to the 1s timeout, pre-tool
  verdict/hold metrics, and `pipeline_version` stamped on every decision log
  line so audits can reconstruct exactly what evaluated a past request.
  Execution records carry the evaluating `pipeline_version_id` and member
  `policy_version_id`s plus actor attribution. All-hook verdict/block-rate
  counters and guardrail metrics are designed, not built.

## 12. Known limitations (accepted)

- **Dynamic typing:** schema-checked compile + smoke tests + shape guards are
  compensating controls, weaker than compile-time typing; schema discipline is
  required. Overlapping `verdict` rules with different values are a runtime
  conflict error (use `else`); `default` rules are mandatory.
- **No Unicode normalization in matching:** homoglyph/zero-width evasion needs
  a host normalization pre-pass (not yet designed). RE2 has no
  lookahead/lookbehind.
- **No cross-request state:** rate limits, quotas, dedupe are host-side.
- **Multi-tenant blast radius:** a pathological policy degrades latency for
  all tenants; timeouts, eval limits, and size gates apply even for trusted
  authors.
- **Decide attribution is best-effort** after a BLOCK short-circuit.
- **The tip is shared mutable staging:** abandoned experiments sit in the
  lineage; the promote-time diff is the safety net. Acceptable at current
  scale with owner-only access.

## 13. Deferred (priority order)

1. Guardrail extensions: output guardrails, the `during_call` lane
   (concurrent-with-dispatch, block-only), caching/retries.
2. post-resp hook + output inspection (buffered with byte cap, then windowed
   streaming, annotate/decide only; you cannot un-send a flushed chunk).
3. Operator-authored reusable Rego modules.
4. Operator-supplied stored tests.
5. FLAG verdict (`BLOCK > FLAG > LOG > ALLOW`) with a defined sink.
6. Cross-provider routing.
7. Annotation value-flow validation (beyond the produced/consumed warnings).
8. Conditional guardrail invocation (skip the vendor call when not needed).
9. Traffic mirroring for staged versions (log-only evaluation against live
   traffic, the gap the evaluation header deliberately does not cover).
