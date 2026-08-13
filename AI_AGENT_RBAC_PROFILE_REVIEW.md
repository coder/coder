# AI Agent RBAC Profile Review

Date: August 13, 2026

Status: design review artifact. Nothing in this document is implemented. The
recommended authorization rule lives in `coderd/rbac`; the other code snippets
only populate inputs consumed by that rule. This review does not propose a
second middleware authorization layer.

## Executive summary

The chat profile is not resource-safe. It combines workspace create, operate,
and access scopes with a `workspace:*` allow-list entry, so the sponsor's live
roles can authorize the AI actor against every workspace the sponsor can reach,
including ordinary workspaces with no AI designation. The direct escape is
`workspace:ssh`: an exfiltrated chat key, or a future HTTP-capable tool using
that key, can enter an ordinary sponsor workspace and read the owner's ambient
credentials. `workspace:application_connect`, `workspace:start`,
`workspace:stop`, and `workspace:update` are also broader than the chat's
lineage. The profile and wildcard are defined at
`coderd/aiagentidentity/profile.go:23-50`; scope expansion is defined at
`coderd/rbac/scopes.go:140-162`.

The creation variant is closed. Commit `97c1e8e147` moved designation into the
shared `createWorkspace` path. That path checks `AIAgentActor` after the
workspace row exists, writes `workspaces.ai_agent_id`, and does so before the
initial build is created, so both REST and in-process callers inherit the same
behavior (`coderd/workspaces.go:543-551,658-666,732-784,795-823`). The connect
variant remains open because authorization does not compare the acting AI
agent with the target workspace's designation.

### Findings summary

| ID | Severity | Finding                                                                                                                                                                          | Disposition                                                              |
|----|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------|
| R1 | Critical | `coder:workspaces.access` plus `workspace:*` authorizes SSH and application connections to undesignated sponsor workspaces.                                                      | Fix in `coderd/rbac` with an object-side designation match.              |
| R2 | High     | `coder:workspaces.operate` plus `workspace:*` authorizes start, stop, and update against undesignated sponsor workspaces. A start build may select a different template version. | Require designation for every non-read, non-create workspace action.     |
| R3 | Medium   | Workspace read and list reach undesignated sponsor workspaces.                                                                                                                   | Keep allowed for chat UX, pending owner sign-off.                        |
| R4 | Info     | `WorkspaceAgentIdentityProfile` has only low-level workspace scopes and one exact workspace ID.                                                                                  | Keep its existing exact-ID bound, then add the policy defense in depth.  |
| R5 | High     | A dynamic allow list would require mutating credential rows and still cannot authorize creation before a workspace ID exists.                                                    | Reject Mechanism B.                                                      |
| R6 | Medium   | Mechanism A changes the Rego input and partial-evaluation contract.                                                                                                              | Add explicit AST and `regosql` mappings; make zero values fail closed.   |
| R7 | High     | A naive object-side gate sees a stale, undesignated in-memory workspace during the initial AI-created start build.                                                               | Copy the setter result into the object before `wsbuilder` authorization. |

## 1. Reachability matrix

### Finding 1, Critical: the chat profile crosses workspace designation boundaries

**Classification rules:**

- **SAFE** means the pair does not provide shell, application, lifecycle, or
  credential-bearing workspace reach in this review's threat model.
- **BOUNDED** means the pair is safe because an exact allow-list ID or the
  creation-time designation chokepoint confines it.
- **OPEN** means the pair reaches an undesignated human workspace. An OPEN row
  can still be an accepted product choice, as with metadata-only read.

`APIKeyScopes.expandRBACScope` unions all selected permissions, and
`APIKeyScopeSet.Expand` intersects that union with one database allow list
(`coderd/database/modelmethods.go:283-339,355-374`). Rego then matches that list
by resource type and ID, independently of action
(`coderd/rbac/policy.rego:319-392`). Therefore `workspace:*` applies equally to
create, read, update, start, stop, SSH, and application connect.

#### `ChatAgentProfile`

The profile selects six scopes and allows one chat ID plus typed wildcards for
workspace, template, organization-member, and user resources
(`coderd/aiagentidentity/profile.go:31-50`). Composite expansion below comes
from `coderd/rbac/scopes.go:140-162`; low-level scopes expand to one site
permission with a wildcard scope allow list before the database overlay is
applied (`coderd/rbac/scopes.go:247-265`;
`coderd/rbac/scopes_test.go:15-41`).

| Profile scope              | Low-level resource/action pair  | Effective allow-list match | Class   | Review result                                                                                                                                                                                                           |
|----------------------------|---------------------------------|----------------------------|---------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `coder:workspaces.create`  | `template:read`                 | `template:*`               | SAFE    | Template metadata is needed to select a workspace source. Sponsor roles still apply.                                                                                                                                    |
| `coder:workspaces.create`  | `template:use`                  | `template:*`               | SAFE    | Use is required by workspace preflight and does not itself reach an existing workspace (`coderd/workspaces.go:978-988`).                                                                                                |
| `coder:workspaces.create`  | `workspace:create`              | `workspace:*`              | BOUNDED | The preflight object has no ID, so a wildcard is required. The shared path writes designation before the first build, but Mechanism A must refresh the in-memory build object (`coderd/workspaces.go:763-784,795-823`). |
| `coder:workspaces.create`  | `workspace:read`                | `workspace:*`              | OPEN    | Reaches ordinary sponsor workspace metadata. This is accepted by the proposed default, not security-bounded by designation.                                                                                             |
| `coder:workspaces.create`  | `workspace:update`              | `workspace:*`              | OPEN    | Reaches ordinary sponsor workspace settings.                                                                                                                                                                            |
| `coder:workspaces.create`  | `workspace:start`               | `workspace:*`              | OPEN    | Can start or rebuild an ordinary sponsor workspace.                                                                                                                                                                     |
| `coder:workspaces.create`  | `workspace:stop`                | `workspace:*`              | OPEN    | Can stop an ordinary sponsor workspace.                                                                                                                                                                                 |
| `coder:workspaces.create`  | `organization_member:read`      | `organization_member:*`    | SAFE    | Metadata-only for this takeover analysis; sponsor organization permissions remain the ceiling.                                                                                                                          |
| `coder:workspaces.operate` | `template:read`                 | `template:*`               | SAFE    | Metadata-only for this workspace-boundary analysis.                                                                                                                                                                     |
| `coder:workspaces.operate` | `workspace:read`                | `workspace:*`              | OPEN    | Reaches ordinary sponsor workspace metadata. Accepted only as the current product default.                                                                                                                              |
| `coder:workspaces.operate` | `workspace:update`              | `workspace:*`              | OPEN    | Can mutate an ordinary sponsor workspace. Protect this action even though template-version selection is authorized through start, as qualified below.                                                                   |
| `coder:workspaces.operate` | `workspace:start`               | `workspace:*`              | OPEN    | Can start or rebuild an ordinary sponsor workspace, including a build request that selects another template version.                                                                                                    |
| `coder:workspaces.operate` | `workspace:stop`                | `workspace:*`              | OPEN    | Can stop an ordinary sponsor workspace and terminate its running processes.                                                                                                                                             |
| `coder:workspaces.operate` | `organization_member:read`      | `organization_member:*`    | SAFE    | Metadata-only for this takeover analysis.                                                                                                                                                                               |
| `coder:workspaces.access`  | `template:read`                 | `template:*`               | SAFE    | Metadata-only for this workspace-boundary analysis.                                                                                                                                                                     |
| `coder:workspaces.access`  | `organization_member:read`      | `organization_member:*`    | SAFE    | Metadata-only for this takeover analysis.                                                                                                                                                                               |
| `coder:workspaces.access`  | `workspace:read`                | `workspace:*`              | OPEN    | Reaches ordinary sponsor workspace metadata. Accepted only as the current product default.                                                                                                                              |
| `coder:workspaces.access`  | `workspace:ssh`                 | `workspace:*`              | OPEN    | Headline hole. A shell in an ordinary workspace crosses the credential-starvation boundary.                                                                                                                             |
| `coder:workspaces.access`  | `workspace:application_connect` | `workspace:*`              | OPEN    | Reaches applications in an ordinary workspace and must use the same designation gate as SSH.                                                                                                                            |
| `chat:read`                | `chat:read`                     | Exact profile `chatID`     | BOUNDED | Pinned to the chat passed to the profile.                                                                                                                                                                               |
| `chat:update`              | `chat:update`                   | Exact profile `chatID`     | BOUNDED | Pinned to the chat passed to the profile.                                                                                                                                                                               |
| `user:read`                | `user:read`                     | `user:*`                   | SAFE    | No personal-read or mutation action is present. This label is limited to the workspace takeover analyzed here, not a data-minimization approval (`coderd/rbac/policy/policy.go:96-106`).                                |

There is one important correction to the supplied background. A request can
select another template version when creating a start build:
`postWorkspaceBuildsInternal` passes `TemplateVersionID` to the builder
(`coderd/workspacebuilds.go:402-410,461-472`). The builder authorizes that
transition with `ActionWorkspaceStart`, not `ActionUpdate`, except that waking a
dormant workspace uses update (`coderd/wsbuilder/wsbuilder.go:1253-1279`). The
security conclusion is unchanged: the wildcard operate profile permits
build-time code execution in an ordinary sponsor workspace, but the decisive
low-level permission for the template-version switch is `workspace:start`.
`workspace:update` remains too broad and should be designation-gated because it
covers workspace settings and parameters (`coderd/rbac/policy/policy.go:49-64`).

Read and list are different from connect and operate. Workspace read exposes
metadata and build-related information, but it does not itself put the actor in
the ordinary workspace's process or credential boundary. The proposed default
keeps it OPEN and allowed so a chat can answer questions such as "what
workspaces do I have?" This is a product tradeoff, not a claim that the row is
resource-bounded.

### Finding 2, Informational: the workspace identity profile is pinned to one workspace

`WorkspaceAgentIdentityProfile` contains six low-level workspace scopes and an
allow list with exactly one `workspace:<workspaceID>` entry. It contains no
wildcard (`coderd/aiagentidentity/profile.go:54-74`).

| Profile scope                   | Low-level resource/action pair  | Effective allow-list match | Class   | Review result                               |
|---------------------------------|---------------------------------|----------------------------|---------|---------------------------------------------|
| `workspace:read`                | `workspace:read`                | Exact `workspaceID`        | BOUNDED | Cannot reach a different workspace ID.      |
| `workspace:update`              | `workspace:update`              | Exact `workspaceID`        | BOUNDED | Cannot mutate a different workspace ID.     |
| `workspace:start`               | `workspace:start`               | Exact `workspaceID`        | BOUNDED | Cannot start a different workspace ID.      |
| `workspace:stop`                | `workspace:stop`                | Exact `workspaceID`        | BOUNDED | Cannot stop a different workspace ID.       |
| `workspace:ssh`                 | `workspace:ssh`                 | Exact `workspaceID`        | BOUNDED | Cannot SSH to a different workspace ID.     |
| `workspace:application_connect` | `workspace:application_connect` | Exact `workspaceID`        | BOUNDED | Cannot connect to a different workspace ID. |

This profile is genuinely ID-pinned. The recommended object-side rule still
adds useful defense in depth: if a key is minted with the wrong workspace ID,
or a workspace is later associated with a different AI identity, protected
actions require the designation to match the acting agent.

## 2. Threat analysis of the headline hole

### Finding 3, Critical: wildcard SSH turns an ordinary workspace into a credential bridge

**OBSERVED.** HTTP authentication builds the authorization subject from the
sponsor's live roles and the agent key's `ScopeSet`, while keeping
`Subject.ID` equal to the sponsor. It marks the subject as an AI agent and
stores the separate actor for attribution (`coderd/httpmw/apikey.go:477-549`).
The profile grants wildcard workspace SSH
(`coderd/aiagentidentity/profile.go:31-49`;
`coderd/rbac/scopes.go:158-162`).

**OBSERVED.** Ordinary start builds mint an owner API key and send the plaintext
as `WorkspaceOwnerSessionToken`; the generation call supplies no narrowed
scope, so `apikey.Generate` defaults to `coder:all`
(`coderd/provisionerdserver/provisionerdserver.go:3212-3252`;
`coderd/apikey/apikey.go:85-103`). The same provisioner job also carries owner
external-auth access tokens and the owner's SSH key
(`coderd/provisionerdserver/provisionerdserver.go:711-758,809-840`). In contrast,
an AI-designated build deletes or withholds the owner session token
(`coderd/provisionerdserver/provisionerdserver.go:586-607`), binds every
workspace agent to the designation (`coderd/provisionerdserver/provisionerdserver.go:2229-2249`),
and rejects owner external-auth retrieval from a bound agent
(`coderd/workspaceagents.go:2087-2097`).

**INFERRED attack narrative.** A chat creates an AI identity and mints its
scoped key. If the key plaintext is exfiltrated, or a future tool surface makes
equivalent HTTP calls, the attacker uses `coder ssh` against another workspace
owned by the same sponsor. That workspace is ordinary, so its environment or template
may expose the owner's `CODER_SESSION_TOKEN`, and its build receives the owner
session token, external-auth tokens, and SSH key described above. Reading the
owner token yields a human-owned `coder:all` credential, which no longer passes
through AI-agent scope restrictions. The result is full account takeover.

Current non-exploitability is accidental. Chat creation ignores the plaintext
returned by `MintKey` after the transaction call
(`coderd/x/chatd/chatd.go:1351-1376`). The built-in start and stop tools load the
workspace ID from the chat binding instead of accepting an arbitrary target
(`coderd/x/chatd/chattool/startworkspace.go:65-89,143-160`;
`coderd/x/chatd/chattool/stopworkspace.go:66-90,142-149`). Neither fact is an
RBAC guarantee. A leaked bearer, a new generic workspace tool, or a future
call path can exercise the permissions already granted by policy.

The cross-agent variant must also fail. If agent A and agent B share a sponsor,
owner equality is identical for both. A designation check based only on
"non-null AI workspace" would therefore let A enter B's workspace. The rule
must require exact equality between `subject.ai_agent_id` and
`object.ai_agent_id`.

## 3. Two candidate mechanisms and recommendation

### Finding 4, Medium: Mechanism A models the dynamic boundary on the object

Mechanism A adds the workspace's designation to `rbac.Object` and the acting
agent's ID to `rbac.Subject`. The Rego policy adds one mandatory allow gate for
workspace actions other than read and create. Human subjects are unaffected.
AI subjects need an exact designation match for SSH, application connect,
start, stop, update, and any future non-read workspace action.

This fits the domain. Designation is stored on the workspace and changes from
NULL to an agent ID when creation or opt-in designates it
(`coderd/database/migrations/000567_workspace_ai_designation.up.sql:1-7`;
`coderd/workspaces.go:768-784`). `WorkspaceTable.RBACObject` is the natural
central point for the owner, organization, ID, ACL, and designation attributes,
but the `Workspace` to `WorkspaceTable` converter currently omits `AIAgentID`
(`coderd/database/modelmethods.go:488-518,538-555`). The actor identity already
exists separately from the sponsor subject in `AIAgentActor`
(`coderd/aiagentidentity/aiagentidentity.go:36-62`) and is resolved in both HTTP
and in-process chat paths (`coderd/httpmw/apikey.go:500-529`;
`coderd/x/chatd/chattool/subject.go:41-70`).

The cost is real. `Subject.regoValue` and `Object.regoValue` manually enumerate
input fields (`coderd/rbac/astvalue.go:68-143`). `UserRBACSubject` also caches the
AST before the AI-specific fields are currently applied
(`coderd/httpmw/apikey.go:971-1000`), so direct field mutation would be a silent
bug unless a helper invalidates and rebuilds the cache. Partial evaluation
currently marks five object fields as unknown and supplies only object type in
the partial input (`coderd/rbac/authz.go:349-370`;
`coderd/rbac/astvalue.go:37-66`). Workspace SQL conversion maps only ID, owner,
organization, and ACLs (`coderd/rbac/regosql/configs.go:43-55`). Mechanism A
must update all three contracts.

The policy should exempt only `ActionRead` and `ActionCreate`; every other
workspace action should require designation. The current workspace action
catalog also contains delete, share, and agent-management actions, so naming
only today's five profile actions would fail open when a future profile grows
(`coderd/rbac/policy/policy.go:49-71`). For `ActionRead`, the exemption resolves
from the known action and object type before an object designation is needed,
so existing list filtering does not gain an AI predicate. Workspace listing
compiles partial Rego through `ConfigWorkspaces`
(`coderd/database/modelqueries.go:239-255,358-366`). A mapping for
`input.object.ai_agent_id` should still be added so prepared or SQL checks for
protected actions remain correct.

Zero values must fail closed. `workspaces.ai_agent_id` is nullable
(`coderd/database/migrations/000567_workspace_ai_designation.up.sql:1-7`). The
RBAC object should encode NULL as `""`; an AI subject has a non-empty acting
agent ID, so equality fails for every undesignated workspace. Rego must also
consume `Subject.Type`: `Type = ai_agent` with an accidentally empty acting ID
must deny protected actions, not fall through to the human branch. `Type` is
currently documented as logging-only, so making it functional is an explicit
contract change (`coderd/rbac/authz.go:99-123`).

### Finding 5, High: Mechanism B makes credential state dynamic and breaks creation

Mechanism B removes `workspace:*` from `ChatAgentProfile`, starts the key with
the chat workspace ID, and appends each created workspace ID to the key's
stored allow list. The allow list is a `text[]` column on `api_keys` and is
materialized as `APIKey.AllowList`
(`coderd/database/migrations/000371_api_key_scopes_array_allow_list.up.sql:149-160`;
`coderd/database/models.go:5035-5048`). There is no current query that updates
that column. `UpdateAPIKeyByID` changes only `last_used`, `expires_at`, and
`ip_address` (`coderd/database/queries/apikeys.sql:79-87`). Mechanism B would
therefore require a new credential-row mutation query.

The current profile constructor receives only `chatID`, and the chat workflow
supports a chat with no workspace until `create_workspace` runs
(`coderd/aiagentidentity/profile.go:23-50`;
`coderd/x/chatd/chattool/createworkspace.go:83-103,128-154`). B would also have
to add an optional workspace binding to mint-time inputs and define the empty
initial workspace allow list.

The fatal problem is create authorization. Preflight authorizes
`ActionCreate` on a workspace object with organization and owner but no ID
(`coderd/workspaces.go:944-976`). The policy's exact-ID allow-list branch
requires `input.object.id` to match an allowed ID
(`coderd/rbac/policy.rego:375-392`). A chat key with only existing workspace IDs
cannot authorize a not-yet-created object. Keeping a create wildcard while
removing it for access is also impossible in the current model because the
allow list is action-independent and is applied after all selected scopes are
unioned (`coderd/database/modelmethods.go:302-339,355-374`). Mechanism B cannot
solve the hole without redesigning scope storage into per-action allow lists,
which is a larger authorization model change than Mechanism A.

Even if creation were special-cased in Rego, B would introduce request-time
mutation of a credential row. The create transaction would need to append the
workspace ID without losing concurrent appends, coordinate with key rotation
and revocation, and decide which of multiple keys or sessions for the same
agent receive the new ID. A successful workspace insert followed by a failed
allow-list update would strand a designated workspace that its creating key
cannot operate; an update that races with revocation can resurrect or mutate a
credential being removed. These are avoidable consistency problems because
the authoritative relationship already exists on the workspace object.

Removing the wildcard also collapses list UX. An exact-ID allow list causes
workspace read filtering to return only IDs already copied into the key, so the
chat cannot enumerate the sponsor's ordinary workspaces. That conflicts with
the proposed default of retaining read/list while protecting connect and
operate.

### Tradeoff table

| Criterion                         | Mechanism A, object-side attribute                           | Mechanism B, dynamic allow list                                                                |
|-----------------------------------|--------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| Models current truth              | Yes. Reads `workspaces.ai_agent_id` at authorization time.   | No. Copies workspace IDs into mutable credential state.                                        |
| Workspace create before ID exists | Works. `create` is not a protected action.                   | Fails with an exact-ID list unless creation gets a special case.                               |
| SSH and app connect               | Exact agent-to-workspace designation match.                  | Exact IDs can work after successful mutation.                                                  |
| Start, stop, update               | Exact designation match can protect all three.               | Exact IDs can work after successful mutation.                                                  |
| Read/list UX                      | Can remain sponsor-wide by action.                           | Collapses to IDs copied into the key.                                                          |
| Cross-agent isolation             | Exact subject/object AI agent equality.                      | Possible only if every key's ID set stays correct.                                             |
| Races and lifecycle               | Reads one authoritative workspace field.                     | Adds append, rotate, revoke, and multi-key races.                                              |
| RBAC implementation cost          | Rego input, AST, partial evaluation, SQL mapping, and tests. | New query plus credential mutation protocol, and still needs policy special-casing for create. |

**Recommendation: Mechanism A.** The strongest reason is that designation is a
dynamic property of the resource being authorized, while an API key allow list
is static subject state. Copying resource lineage into credentials creates
staleness and race conditions, and it still cannot represent a create target
that has no ID.

## 4. Concrete design for Mechanism A

### Finding 6, Medium: add one functional subject attribute and one object attribute

The following snippets are implementation sketches only.

#### 4.1 `coderd/rbac/object.go` and `coderd/rbac/authz.go`

Current `Object` fields are declared at `coderd/rbac/object.go:20-43`; current
`Subject` fields and cached AST are at `coderd/rbac/authz.go:99-123`.

```go
// coderd/rbac/object.go

type Object struct {
    ID          string `json:"id"`
    Owner       string `json:"owner"`
    OrgID       string `json:"org_owner"`
    AnyOrgOwner bool   `json:"any_org"`
    Type        string `json:"type"`

    // AIAgentID is the workspace designation. Empty means undesignated.
    AIAgentID string `json:"ai_agent_id"`

    ACLUserList  map[string][]policy.Action `json:"acl_user_list"`
    ACLGroupList map[string][]policy.Action `json:"acl_group_list"`
}

func (z Object) WithAIAgentID(id string) Object {
    z.AIAgentID = id
    return z
}
```

Every value-copying `Object` helper must preserve `AIAgentID`, and `Equal` must
compare it. `All` should deliberately clear it because an all-resources object
cannot claim one designation; that makes a protected AI authorization against
an aggregate object fail closed. These helpers currently reconstruct the struct
field by field (`coderd/rbac/object.go:93-115,140-239`).

```go
// coderd/rbac/authz.go

type Subject struct {
    FriendlyName string
    Email        string
    // Type becomes a policy input. It remains useful for logging too.
    Type SubjectType

    ID string
    // AIAgentID is the acting AI principal. ID remains the sponsoring human.
    // Empty is valid for non-AI subjects. An AI Type with an empty ID fails
    // closed for protected actions.
    AIAgentID string
    Roles     ExpandableRoles
    Groups    []string
    Scope     ExpandableScope

    cachedASTValue ast.Value
}

func (s Subject) AsAIAgent(id uuid.UUID, friendlyName string) Subject {
    s.Type = SubjectTypeAIAgent
    s.FriendlyName = friendlyName
    s.AIAgentID = id.String()
    // UserRBACSubject returns a cached AST. Rebuild it after changing
    // functional authorization fields.
    s.cachedASTValue = nil
    return s.WithCachedASTValue()
}
```

`Subject.Equal` must compare both `Type` and `AIAgentID`, because those fields
become functional. It currently compares ID, groups, roles, and scope only
(`coderd/rbac/authz.go:147-163`). Authorization cache hashing already
JSON-encodes the whole subject and object, so the exported fields naturally
enter the cache key (`coderd/rbac/authz.go:37-59`).

Keep `AIAgentID` exported. The authorization hash JSON-encodes `Subject`, and
the resulting key feeds a global cross-request cache
(`coderd/rbac/authz.go:37-59,775-812`). An unexported acting ID would be omitted
from the hash and could make two agents with otherwise identical subjects share
a cached authorization result.

#### 4.2 `coderd/rbac/astvalue.go`

The subject type and both agent ID attributes must be present in full Rego
input. Empty ID strings are intentional and support fail-closed equality.

```go
// coderd/rbac/astvalue.go, inside Subject.regoValue
[2]*ast.Term{
    ast.StringTerm("type"),
    ast.StringTerm(string(s.Type)),
},
[2]*ast.Term{
    ast.StringTerm("ai_agent_id"),
    ast.StringTerm(s.AIAgentID),
},

// coderd/rbac/astvalue.go, inside Object.regoValue
[2]*ast.Term{
    ast.StringTerm("ai_agent_id"),
    ast.StringTerm(z.AIAgentID),
},
```

These additions are anchored to the existing manual subject and object AST
construction at `coderd/rbac/astvalue.go:83-100,114-143`.

#### 4.3 `coderd/database/modelmethods.go`

Current code builds a workspace object without the designation
(`coderd/database/modelmethods.go:538-555`). Populate it at this converter, not
at individual handlers.

First preserve the field when a full `Workspace` is reduced. The current
converter copies ACLs but omits `AIAgentID`
(`coderd/database/modelmethods.go:488-513`).

```go
// coderd/database/modelmethods.go, inside Workspace.WorkspaceTable
return WorkspaceTable{
    // Existing fields remain unchanged.
    GroupACL:  w.GroupACL,
    UserACL:   w.UserACL,
    AIAgentID: w.AIAgentID,
}
```

Then populate the RBAC object:

```go
// Before, coderd/database/modelmethods.go
obj := rbac.ResourceWorkspace.
    WithID(w.ID).
    InOrg(w.OrganizationID).
    WithOwner(w.OwnerID.String())
```

```go
// After, coderd/database/modelmethods.go
obj := rbac.ResourceWorkspace.
    WithID(w.ID).
    InOrg(w.OrganizationID).
    WithOwner(w.OwnerID.String())

if w.AIAgentID.Valid {
    obj = obj.WithAIAgentID(w.AIAgentID.UUID.String())
}
```

Apply the same population to `WorkspaceTable.DormantRBAC`, which currently
builds a separate `workspace_dormant` object (`coderd/database/modelmethods.go:557-562`).
NULL remains the zero value `""`, which is required for fail-closed AI checks.

Audit the other workspace-typed constructors too. `Workspace.AsPrebuild` and
`WorkspaceTable.DormantRBAC` build separate object types, while
`WorkspaceIdentity` synthesizes a reduced workspace and currently has no
`AIAgentID` field (`coderd/database/modelmethods.go:526-562,955-1009`). They
must preserve designation. Row wrappers that delegate to
`WorkspaceTable.RBACObject` inherit the central fix
(`coderd/database/modelmethods.go:1012-1049`). A table-driven converter test
should assert that every workspace, dormant-workspace, and prebuilt-workspace
object carries the expected designation.

#### 4.4 Finding 7, High: refresh the in-memory workspace before its first build

There is a fail-closed happy-path trap. `createWorkspace` fetches `workspace`
before designation, discards the `WorkspaceTable` returned by
`SetWorkspaceAIAgentID`, and then passes the earlier `workspace` value to
`wsbuilder.New` (`coderd/workspaces.go:760-784,795-823`). Once start requires a
designation match, the initial AI-created build would be denied unless the
local object is updated.

```go
// coderd/workspaces.go
if aiActor, ok := aiagentidentity.ActorFromContext(ctx); ok {
    designated, err := db.SetWorkspaceAIAgentID(
        dbauthz.AsSystemRestricted(ctx),
        database.SetWorkspaceAIAgentIDParams{
            ID:        workspace.ID,
            AIAgentID: uuid.NullUUID{
                UUID: aiActor.AgentUserID,
                Valid: true,
            },
        },
    )
    if err != nil {
        return xerrors.Errorf("designate AI-created workspace: %w", err)
    }
    // Keep the object passed to wsbuilder authorization in sync with the row.
    workspace.AIAgentID = designated.AIAgentID
}
```

This remains input construction, not a second authorization decision. The
policy still decides whether the subsequent start is allowed.

#### 4.5 Subject population in HTTP, chat tools, and workspace-agent middleware

HTTP authentication currently sets only the logging `Type` and friendly name
after `UserRBACSubject` returns (`coderd/httpmw/apikey.go:524-530`). The acting
ID must be set with the cache-safe helper.

```go
// coderd/httpmw/apikey.go
actor, userStatus, err = UserRBACSubject(
    ctx, cfg.DB, identity.OwnerUser.ID, key.ScopeSet(),
)
if err == nil {
    actor = actor.AsAIAgent(
        identity.Actor.AgentUserID,
        identity.AgentUser.Username,
    )
    resolvedActor := identity.Actor
    agentActor = &resolvedActor
}
```

The same helper is required for in-process chat tools after their profile-scoped
subject is built (`coderd/x/chatd/chattool/subject.go:56-70`):

```go
actor = actor.AsAIAgent(
    identity.Actor.AgentUserID,
    identity.AgentUser.Username,
)
```

Workspace-agent middleware must do the same when a bound agent resolves its AI
identity (`coderd/httpmw/workspaceagent.go:156-191`):

```go
subject = subject.AsAIAgent(
    identity.Actor.AgentUserID,
    identity.AgentUser.Username,
)
```

These call sites only populate input. They do not make an authorization
decision; the decision remains in `coderd/rbac/policy.rego`.

#### 4.6 `coderd/rbac/policy.rego`

The existing policy ends with one `allow` rule requiring both permission and
scope (`coderd/rbac/policy.rego:435-461`). Match that positive-gate style rather
than adding a middleware veto.

```rego
# coderd/rbac/policy.rego

# Treat either marker as AI-delegated. This fails closed if Type says AI but the
# acting ID was not populated, and also if a non-empty acting ID is paired with
# an unexpected Type.
subject_is_ai_agent if {
    input.subject.type = "ai_agent"
}

subject_is_ai_agent if {
    not input.subject.ai_agent_id = ""
}

# AI agents may read workspace metadata and create a workspace before its ID
# exists. Every other workspace action requires exact designation. Keeping the
# exemption list short makes future workspace actions protected by default.
ai_workspace_action_requires_designation if {
    input.object.type in {"workspace", "workspace_dormant", "prebuilt_workspace"}
    not input.action in {"read", "create"}
}

# Human and system subjects are unaffected.
ai_workspace_designation_allow if {
    not subject_is_ai_agent
}

# AI subjects may perform non-protected actions, including read and create.
ai_workspace_designation_allow if {
    subject_is_ai_agent
    not ai_workspace_action_requires_designation
}

# Protected actions require a non-empty acting ID and exact lineage.
ai_workspace_designation_allow if {
    subject_is_ai_agent
    not input.subject.ai_agent_id = ""
    input.object.ai_agent_id = input.subject.ai_agent_id
}

allow if {
    permission_allow
    scope_allow
    ai_workspace_designation_allow
}
```

This rule denies agent A against agent B's workspace even when both share a
sponsor, because only exact agent ID equality satisfies the protected branch.
It denies an AI subject against an undesignated object because `""` does not
equal the non-empty subject agent ID. It also denies protected actions when an
AI subject's acting ID was accidentally left empty, instead of treating that
subject as human.

#### 4.7 Partial evaluation and SQL conversion

The partial query currently declares ID, owner, organization, and ACL fields as
unknown (`coderd/rbac/authz.go:349-370`). Add the designation:

```go
// coderd/rbac/authz.go, rego.Unknowns
rego.Unknowns([]string{
    "input.object.id",
    "input.object.owner",
    "input.object.org_owner",
    "input.object.ai_agent_id",
    "input.object.acl_user_list",
    "input.object.acl_group_list",
})
```

Map it for workspace SQL filters alongside the existing ID, organization, and
owner mappings (`coderd/rbac/regosql/configs.go:43-55`):

```go
// coderd/rbac/regosql/configs.go
func WorkspaceConverter() *sqltypes.VariableConverter {
    matcher := sqltypes.NewVariableConverter().RegisterMatcher(
        resourceIDMatcher(),
        sqltypes.StringVarMatcher(
            "workspaces.organization_id :: text",
            []string{"input", "object", "org_owner"},
        ),
        userOwnerMatcher(),
        sqltypes.StringVarMatcher(
            "COALESCE(workspaces.ai_agent_id::text, '')",
            []string{"input", "object", "ai_agent_id"},
        ),
    )
    // Existing ACL matchers remain unchanged.
    return matcher
}
```

`regosql` rejects variables without a registered converter
(`coderd/rbac/regosql/compile.go:171-199`). The mapping is therefore required
for any protected-action SQL compilation. The `workspaces` qualifier is valid
in both queries that use this converter
(`coderd/database/queries/workspaces.sql:110-123,1062-1101`). Read/list remains
cheap: action and object type are known in the partial input
(`coderd/rbac/astvalue.go:37-66`), so the read exemption is ground and produces
no residual `ai_agent_id` predicate. The ordinary workspace list continues to
compile through `ConfigWorkspaces` (`coderd/rbac/authz.go:742-746`;
`coderd/database/modelqueries.go:244-255`). Add a converter case beside the
existing workspace ACL cases in `coderd/rbac/regosql/compile_test.go:200-218`.

#### 4.8 Test sketch

The organization workspace-access role grants member-level workspace actions
(`coderd/rbac/roles.go:225-247,716-728`). A focused policy test can therefore
use the real role and a low-level SSH scope.

```go
// coderd/rbac/authz_aiagent_test.go
func TestAIAgentWorkspaceDesignation(t *testing.T) {
    t.Parallel()

    ctx := testutil.Context(t, testutil.WaitShort)
    authz := rbac.NewStrictAuthorizer(prometheus.NewRegistry())
    ownerID := uuid.New()
    orgID := uuid.New()
    agentA := uuid.New()
    agentB := uuid.New()

    sshScope, err := rbac.ScopeName("workspace:ssh").Expand()
    require.NoError(t, err)

    aiSubject := rbac.Subject{
        Type:      rbac.SubjectTypeAIAgent,
        ID:        ownerID.String(),
        AIAgentID: agentA.String(),
        Roles: rbac.RoleIdentifiers{
            rbac.RoleMember(),
            rbac.ScopedRoleOrgWorkspaceAccess(orgID),
        },
        Scope: sshScope,
    }

    workspace := func(designation string) rbac.Object {
        return rbac.ResourceWorkspace.
            WithID(uuid.New()).
            InOrg(orgID).
            WithOwner(ownerID.String()).
            WithAIAgentID(designation)
    }

    t.Run("AI subject denied on undesignated workspace", func(t *testing.T) {
        err := authz.Authorize(ctx, aiSubject, policy.ActionSSH, workspace(""))
        require.Error(t, err)
    })

    t.Run("AI subject allowed on own designated workspace", func(t *testing.T) {
        err := authz.Authorize(ctx, aiSubject, policy.ActionSSH, workspace(agentA.String()))
        require.NoError(t, err)
    })

    t.Run("human subject unaffected", func(t *testing.T) {
        human := aiSubject
        human.Type = rbac.SubjectTypeUser
        human.AIAgentID = ""
        err := authz.Authorize(ctx, human, policy.ActionSSH, workspace(""))
        require.NoError(t, err)
    })

    t.Run("AI type with missing acting ID fails closed", func(t *testing.T) {
        missingID := aiSubject
        missingID.AIAgentID = ""
        err := authz.Authorize(ctx, missingID, policy.ActionSSH, workspace(agentA.String()))
        require.Error(t, err)
    })

    t.Run("agent A denied on agent B workspace", func(t *testing.T) {
        err := authz.Authorize(ctx, aiSubject, policy.ActionSSH, workspace(agentB.String()))
        require.Error(t, err)
    })
}
```

Add parallel cases for `ActionApplicationConnect`, `ActionWorkspaceStart`,
`ActionWorkspaceStop`, and `ActionUpdate`, plus positive tests showing
`ActionRead` remains allowed and `ActionCreate` still works on an ID-less,
undesignated object. Iterate `ResourceWorkspace.AvailableActions()` and require
designation for every action except read and create, so new actions cannot drift
open. Add a `Prepare` plus `CompileToSQL(ConfigWorkspaces())` test for both read
and SSH so the partial-evaluation contract cannot regress. Finally, add a
`createWorkspace` regression test proving the initial AI-created start build
receives the designation on its in-memory RBAC object.

### Could not verify in this design-only pass

- No end-to-end SSH session was started against a running ordinary workspace.
  The RBAC reach and credential delivery paths were verified from code, but the
  complete takeover chain was not executed.
- The proposed Rego and SQL residuals were not compiled because this artifact
  intentionally changes no source. The design therefore requires the
  `Authorize`, `Prepare`, and `CompileToSQL` tests described above.
- Out-of-repository templates and integrations that expose
  `WorkspaceOwnerSessionToken` as `CODER_SESSION_TOKEN` were not inspected.

### Open product decisions, pending owner sign-off

| Decision                                    | Proposed default                       | Rationale                                                                                                                           |
|---------------------------------------------|----------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| Read/list sponsor's undesignated workspaces | **Allowed.**                           | Preserves chat inventory and selection UX. This is intentionally OPEN metadata reach.                                               |
| SSH and application connect                 | **Require exact designation match.**   | Both cross into the workspace runtime and its credentials.                                                                          |
| Start, stop, and update                     | **Require exact designation match.**   | Prevents disruption and mutation of ordinary workspaces. Start also covers rebuilds and template-version selection in current code. |
| Cross-agent access with the same sponsor    | **Denied.**                            | Require `object.ai_agent_id == subject.ai_agent_id`, not merely non-null designation.                                               |
| Other non-read workspace actions            | **Require exact designation match.**   | Protect delete, share, and agent-management actions by default if future profiles add them.                                         |
| Workspace create before an ID exists        | **Allowed without designation match.** | The shared chokepoint writes designation, then must refresh the build object before start authorization.                            |

Owner sign-off should explicitly confirm the read/list exception and the choice
to protect all three operate-class actions. The security recommendation is to
protect start, stop, and update together. Allowing any one of them on an
undesignated workspace preserves a separate integrity or availability attack
surface even after SSH is closed.
