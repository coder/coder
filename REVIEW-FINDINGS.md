# Panel Review Findings: gateway-account-prototype

Date: July 14, 2026. Reviewed at commit `313b2d8110` by a 7-persona
panel review (rego policy, roles/migrations, licensing, bridge
enforcement, API/data layer, frontend, lifecycle). This file tracks the
findings so they can be burned down on this branch; remove it before any
merge to main.

## Corroborated findings

### 1. Dormant shared workspaces are broken; comments claim the opposite

Severity: HIGH. Flagged by lifecycle and rego reviewers. The broken
behavior is pre-existing on main: `DormantRBAC()` is unchanged by this
branch, and the list/GET inconsistency for ACL recipients reproduces
there (list filter is prepared against type `workspace` with ACL
clauses; direct access evaluates the ACL-less dormant object). What
this branch introduced is only the contradiction: the dormant
`use_shared` grant and comments claiming recipients keep access through
dormancy, which the pre-existing `DormantRBAC()` makes impossible. The
grant is inert.

`DormantRBAC()` (`coderd/database/modelmethods.go`) attaches no ACLs, so
ACL recipients get 403 on a dormant shared workspace. The workspaces
list SQL filter is prepared against type `workspace` with ACLs intact on
the row, so the dormant workspace still appears in the recipient's list.
Result: users see workspaces they cannot open. The comment at
`coderd/rbac/roles.go` (dormant list in `OrgWorkspaceAccessMemberPerms`)
says `use_shared` exists "so ACL recipients keep access to shared
workspaces that go dormant", which the implementation makes impossible.
A test comment in `roles_test.go` (`WorkspaceDormantUseShared`) has the
same contradiction.

Fix decision needed: attach `WithACLUserList`/`WithGroupACL` in
`DormantRBAC()` (matches stated intent, fixes the list/GET
inconsistency), or drop `use_shared` from dormant and rewrite the
comments to say dormancy intentionally revokes shared access.

### 2. Pre-existing admin ACL entries no longer round-trip in the API

Severity: HIGH (reported as BLOCKER for a main merge). FIXED on this
branch: `use_shared` is now omitted from the admin set in
`WorkspaceRoleActions`, restoring the exact-set match for entries
stored before the action existed. Entries written by intermediate
builds of this branch (admin entries containing `use_shared`) would
still mismatch; none exist on the dev instance. The underlying bug
class (dynamic role definition, materialized storage, exact-equality
read-back) is documented in the scott-misc findings for the RFC.

Adding `ActionUseShared` to `workspaceActions` grew
`WorkspaceRoleActions(WorkspaceRoleAdmin)` via `AvailableActions()`.
Stored admin ACL entries written before the change fail the
`slice.SameElements` comparison in `convertToWorkspaceRole`
(`coderd/workspaces.go`) and fall through to `WorkspaceRoleDeleted` in
API responses. Access is unaffected (rego evaluates the stored action
list), but `GET /workspaces/:id/acl` misrepresents existing admin
shares as role-less.

Cleanest fix: omit `policy.ActionUseShared` from the admin set in
`WorkspaceRoleActions` (`coderd/database/db2sdk/db2sdk.go`); a
capability precondition action is semantically circular inside an ACL
action list. Caveat: entries written by this build already contain
`use_shared` and would then mismatch; needs a normalization decision
(tolerant matching or a data migration) for the RFC.

### 3. Group shares are not validated at share time

Severity: MEDIUM. Flagged by API and lifecycle reviewers. ACCEPTED as
intended behavior: group membership changes after the share, so
eligibility is enforced per-member at access time only; a share to a
group whose members all currently lack the capability is legitimate
and grants nothing until a member gains it. The code comment in
`patchWorkspaceACL` now states this explicitly.

`patchWorkspaceACL` validates `req.UserRoles` against the `use_shared`
capability but writes `req.GroupRoles` unconditionally. Sharing to a
group of gateway-only users succeeds silently and grants nothing.
Access-time enforcement holds, so this is a UX guard gap, not a
security gap. Fix or explicitly document as accepted (the code comment
currently implies all recipients are validated).

### 4. Experiments are snapshotted at bridge server construction

Severity: MEDIUM. Flagged by enforcement and lifecycle reviewers.

The AGPL embedded path (`coderd/aibridged.go`) constructs the aibridged
server once at startup, freezing `s.experiments`; the enterprise path
constructs per WebSocket connection. A runtime experiment change makes
AI-seat recording behavior diverge between the two paths until restart.
Fix: pass a `func() codersdk.Experiments` accessor into `NewServer`
instead of a snapshot.

### 5. Rolling-deploy window can strand new orgs without bridge access

Severity: MEDIUM. Flagged by roles and licensing reviewers.

Migration 000541 backfills `organization-ai-gateway-access` into
existing orgs. In a rolling deploy, an old binary can create an org
after the migration ran; its defaults lack the gateway role, and once
new code serves, members of that org get `ErrNoAIGatewayAccess`. Fix: a
startup sweep that re-asserts the role in defaults, or a runbook note
that org creation must be quiesced during the deploy window.

### 6. Seat count cost at the real entitlement refresh cadence

Severity: HIGH (operational). Corroborates the entitlement-refresh
rate-limit finding in the scott-misc assessment docs.

Entitlements recompute roughly every 5s in practice (replicasync
callback storm, pre-existing bug), so `CountWorkspaceCapableUsers` and
its bulk query run at that cadence. Cache the count with a TTL matched
to the refresh interval or compute it on a slower ticker before this
ships beyond prototype.

## Single-reviewer findings worth tracking

- HIGH (frontend): the Gateway preset card copy "Gateway members do not
  cost license seats" (`DefaultRolesPresetCards.tsx`) is unconditional,
  but the backend only excludes gateway users when the
  `permission-based-licensing` experiment and an AI Governance add-on
  license are both active. Condition or soften the copy.
- MEDIUM (rego, latent): `not input.object.acl_use_gated` in
  `policy.rego` passes when the field is undefined, not just false. All
  current input paths set the field, but one missed future path would
  fail open. Change to `input.object.acl_use_gated == false`.
- MEDIUM (licensing): a `WorkspaceCapableUserCountFn` error silently
  falls back to the legacy (higher) count with only an entitlements
  error string; persistent failure means silent over-counting. Consider
  a distinct, visible warning.
- MEDIUM (lifecycle, pre-existing): AI Governance seats are
  lifetime-cumulative (`GetActiveAISeatCount` has no time window) and
  do not exclude service accounts, unlike `GetActiveUserCount`.
- MEDIUM (frontend): preset cards PATCH immediately with no
  confirmation despite org-wide blast radius; empty
  `default_org_member_roles` renders as a selected-but-empty Custom
  card.
- MEDIUM (enforcement, design question): any custom role granting
  `aibridge_interception.create` passes bridge enforcement, which is
  broader than "the gateway role grants access". Decide whether that is
  intended and document it.
- LOW (licensing, perf): the seat-count dedupe signature includes group
  memberships, which do not affect the workspace-create outcome,
  causing needless cache misses. A roles-only signature would dedupe
  better.
- LOW (roles): the 000541 down migration removes the gateway role from
  all orgs, including any that had added it manually before the
  migration.
- LOW (licensing, latent): add-on dependency validation in
  `LicensesEntitlements` is license-order-sensitive; a spurious
  ordering failure would silently revert to legacy counting.
- LOW (frontend): the new org roles are missing from
  `roleNamesByAccessLevel`, so they sort to the bottom of the role
  selector.
- NIT (api): share-time validation errors all use field `user_roles`
  without identifying which entry failed.
- NIT (frontend): missing stories for the Workspace-card click, the
  empty-defaults state, and the idempotent re-click.

## Prioritized burn-down

Fix on this branch before wider demo or RFC reference:

1. Admin ACL round-trip (finding 2): DONE, `use_shared` omitted from
   the admin ACL action set.
2. Dormant decision (finding 1): pick a direction, implement, fix the
   two contradicting comments.
3. Fail-closed hardening: `acl_use_gated == false` in `policy.rego`.
4. Seat-cost copy conditioning (frontend HIGH).

Document-or-fix next: group-share validation (3): DONE, accepted and
documented in the handler comment; experiments accessor (4);
rolling-deploy sweep (5).

RFC backlog (mostly already tracked in the scott-misc assessment):
refresh cadence caching (6), AI seat reclamation and service-account
filter, count-error visibility, custom-role bypass semantics, preset
confirmation dialog.

## Validated correct by the panel

Enforcement is fail-closed on every traced path (empty allow-lists,
suspended/dormant/deleted/system users, restricted scopes, role
expansion errors). All three rego input paths carry `acl_use_gated`,
and partial evaluation resolves the precondition at prepare time with
no regosql impact. `shared_use_orgs` vote semantics fail closed.
Seat-count exclusions exactly match legacy `GetActiveUserCount`
semantics; the signature dedupe is correct for the current role model;
the `ActiveUserCount` pointer-aliasing overwrite is observed by all
readers. Migration 000541 is idempotent and propagates to existing
members at query time as claimed. Share-time validation uses the
correct object shape, TOCTOU is harmless given access-time enforcement,
and `ScopeAll` is the right scope for a capability check. Frontend
create-CTA gating has no leak paths, and the preset mapping is
order-insensitive with unknown roles correctly falling back to Custom.
