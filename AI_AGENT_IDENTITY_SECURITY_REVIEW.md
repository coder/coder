# AI Agent Identity Security Review

Date: August 13, 2026

Status: review artifact. This document reports observed implementation behavior and inferred security consequences. It does not change production code, migrations, or the two prior review documents.

## Executive summary

The sponsor permission ceiling works as designed: authorization requires both the sponsor's live roles or ACLs and the API key scope. The resource allow list is also enforced. The problem is that the current chat profile deliberately puts every workspace, template, organization-member record, and user record in the allow list, and the scope machinery has only one action-independent allow list for the union of all selected scopes. A compromised chat bearer key can therefore read, start, stop, update, SSH into, and connect to every workspace the sponsor can reach, not only a workspace created by that chat. For an ordinary sponsor this includes every workspace owned by that sponsor. For a site owner sponsor it can include other users' workspaces as well. The current chat implementation discards the key secret, so the generic bearer surface is latent unless the secret is exposed or another key is minted for the identity.

The most serious additional invariant failure is outside `MintKey`: the generic user key APIs can create a default `coder:all` plus `*:*` token for an AI-agent user. Middleware then delegates that token to the sponsor. A privileged administrator can therefore create a full sponsor-equivalent agent key, and that key can create a normal human-owned sponsor token that survives AI identity revocation.

`validateProfile` is not complete. There are 236 valid API key scopes. It rejects 30 and accepts 206, including 177 internal-only scopes and 45 `resource:*` wildcards. Dangerous accepted examples include role assignment, crypto keys, OAuth application secrets, AI Gateway keys, provisioner daemons, `system:*`, and `coder:workspaces.delete`. Current production callers use only the built-in profiles, so arbitrary-profile exploitation through `MintKey` is latent, but the exported validator is not a safe boundary for future callers.

Key expiry is enforced on the normal HTTP and AI Gateway authorization paths. Workspace and sandbox keys still have the documented 24-hour availability gap. Chat gateway keys are renewable leases and can even be resurrected after expiry. Workspace-origin creation and key mint/revoke paths also contain reproducible races. Database uniqueness prevents duplicate live origins, but the losing caller gets a hard error. A key mint that passed resolution can commit after the identity is revoked; later authentication rejects the residual key, so this is currently an invariant and cleanup failure rather than an authorization bypass.

## Evidence labels

- **OBSERVED**: read directly from the cited code, query, migration, or a completed test.
- **INFERRED**: a consequence or interleaving reasoned from observed behavior but not exercised end to end.
- **EMPIRICAL**: exercised with a temporary test written for this review. The temporary tests were removed because they assert known vulnerable behavior and are not suitable regression contracts.

## What I examined

- The Vertical 1 design and rejected alternatives in `AI_AGENT_SECURITY_ARCHITECTURE.md`, especially the identity split, scope profiles, revocation model, and fail-closed invariants.
- The schema findings in `AI_AGENT_IDENTITY_SCHEMA_REVIEW.md`.
- Profile construction and validation in `coderd/aiagentidentity/profile.go`.
- All 236 values returned by `database.AllAPIKeyScopeValues()` and all 50 concrete RBAC resource types in `rbac.AllResources()` (`coderd/database/models.go:287-520,563-803`; `coderd/rbac/object_gen.go:11-497`).
- Scope expansion and allow-list intersection in `coderd/database/modelmethods.go:254-383` and `coderd/rbac/scopes.go:137-175,247-267`.
- OPA role, scope, ACL, and allow-list evaluation in `coderd/rbac/policy.rego:14-392,435-461`.
- Generic key creation, HTTP authentication, AI Gateway authentication, chat key renewal, workspace key rotation, sandbox key rotation, origin creation, and revocation paths.
- Existing tests plus four temporary empirical tests described below.

## Findings

### High: A compromised chat bearer key is not bound to the chat's workspace

**OBSERVED.** `ChatAgentProfile` combines workspace create, operate, and access scopes, and uses typed wildcards for workspace, template, organization-member, and user resources (`coderd/aiagentidentity/profile.go:23-50`). The composites expand as follows:

- `coder:workspaces.create`: template read/use; workspace create/read/update/start/stop; organization-member read.
- `coder:workspaces.operate`: template read; workspace read/update/start/stop; organization-member read.
- `coder:workspaces.access`: template read; workspace read/SSH/application-connect; organization-member read.

These expansions are defined at `coderd/rbac/scopes.go:140-162`.

`APIKeyScopes.expandRBACScope` unions every permission and every scope allow list, then `APIKeyScopeSet.Expand` intersects that union with the one database allow list (`coderd/database/modelmethods.go:283-339,355-374`). OPA tests that one list only by resource type and ID, not by action or creation lineage (`coderd/rbac/policy.rego:319-392`). The design therefore cannot express:

```text
create any workspace because its ID does not exist yet,
but operate and access only the workspace created by this chat.
```

The `workspace:*` entry applies equally to create, read, update, start, stop, SSH, and application-connect.

**EMPIRICAL.** A temporary RBAC test constructed the exact chat profile and a normal sponsor subject with organization workspace access. It proved that the subject could read, update, start, stop, SSH into, and application-connect to two different sponsor-owned workspaces. It was denied workspace deletion and denied another user's workspace. Repeating the check with a site-owner sponsor proved SSH and user reads against resources owned by other users. The test passed:

```text
go test -tags securityreview ./coderd/aiagentidentity \
  -run '^TestSecurityReview' -count=1
ok github.com/coder/coder/v2/coderd/aiagentidentity
```

**Concrete answers for a compromised chat bearer key:**

| Question                                                            | Result                                                                                                                                                                                                                                                                                                                                                                  |
|---------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Read another workspace of the same sponsor?                         | **Yes.** Workspace ID is wildcarded and the sponsor is the RBAC subject.                                                                                                                                                                                                                                                                                                |
| Start or stop it?                                                   | **Yes.** Both actions are in the composites.                                                                                                                                                                                                                                                                                                                            |
| SSH into it?                                                        | **Yes.** `coder:workspaces.access` grants SSH.                                                                                                                                                                                                                                                                                                                          |
| Connect to workspace applications?                                  | **Yes.** `coder:workspaces.access` grants application-connect.                                                                                                                                                                                                                                                                                                          |
| Reach an unbound human workspace that contains ambient credentials? | **Yes, if the sponsor can access it.** The profile does not require AI designation or an AI-agent binding. SSH provides a shell in that human workspace. Credential-starvation checks on bound workspace agents do not protect an unrelated unbound workspace.                                                                                                          |
| Delete the workspace?                                               | **No with the built-in chat profile.** Delete is not selected.                                                                                                                                                                                                                                                                                                          |
| Read template metadata and use templates?                           | **Yes, subject to sponsor roles or ACLs.**                                                                                                                                                                                                                                                                                                                              |
| Read raw template files?                                            | **No through the file resource.** The profile has no file scope, and the template-file fallback requires template update, which the chat scope does not grant (`coderd/database/dbauthz/dbauthz.go:1308-1327`). The built-in `read_template` tool does expose the active version README and rich-parameter metadata (`coderd/x/chatd/chattool/readtemplate.go:28-100`). |
| Enumerate users?                                                    | **Depends on the sponsor.** A normal member can read their own user object but not another user. A sponsor with site-level `user:read`, such as an owner or auditor, can read other users because the profile allow list is `user:*`.                                                                                                                                   |
| Enumerate organization members?                                     | **Often yes within readable organizations.** The profile includes `organization_member:*`; the sponsor role and organization sharing mode decide which rows pass. Returned rows include email, global roles, status, last-seen time, and login type (`coderd/members.go:567-599`).                                                                                      |

**Current reachability qualification.** Initial chat creation discards the plaintext returned by `MintKey` (`coderd/x/chatd/chatd.go:1351-1376`). Only the hash is stored (`coderd/apikey/apikey.go:44-57,111-134`), and the AI Gateway is given the key ID rather than the secret (`coderd/x/chatd/synthetickey.go:31-75`). Built-in tools also operationally pin start and stop operations to the chat-associated workspace. The broad generic REST bearer surface is therefore latent in the default chat flow. It becomes concrete if the secret is exposed, a generic HTTP-capable tool receives it, or another key is minted for the agent identity, as the next finding demonstrates.

**Recommendation.** Split creation from operation. Use a creation-only subject with workspace wildcarding, then switch to a subject pinned to the created workspace UUID. Do not combine wildcard creation and SSH/application-connect in one reusable credential.

### High: Generic key APIs bypass the AI profile validator and permit escape to a human token

**OBSERVED.** The generic routes `/api/v2/users/{user}/keys` and `/api/v2/users/{user}/keys/tokens` accept an arbitrary user resolved by ID or username (`coderd/coderd.go:1783-1825`; `coderd/httpmw/userparam.go:72-137`). Both handlers reject system users but do not reject `user.Kind == 'ai_agent'` (`coderd/apikey.go:40-66,194-222`). An empty token request defaults to `coder:all`, and an absent allow list defaults to `*:*` (`coderd/apikey.go:68-116`; `coderd/apikey/apikey.go:74-102`).

Database authorization checks only whether the caller may create an API key owned by the target user (`coderd/database/dbauthz/dbauthz.go:6075-6086`). Middleware later sees that the key user is an AI agent, resolves its sponsor, and builds the sponsor subject using the stored key's `ScopeSet()` without re-running `validateProfile` (`coderd/httpmw/apikey.go:477-533`).

Organization members have all API-key actions on API-key resources owned by themselves (`coderd/rbac/roles.go:1189-1212`). Because the delegated subject ID is the sponsor ID, a full-scope agent key can target the sponsor's explicit user ID and create a normal sponsor-owned token.

**EMPIRICAL.** A temporary integration test performed this full chain:

1. A site owner created an AI identity.
2. The owner called the generic token route for the AI-agent user with an empty request.
3. The stored agent key had `coder:all`.
4. The client switched to that agent key and successfully created a token for the sponsor's human user ID.

The test passed:

```text
go test -tags securityreview ./coderd \
  -run '^TestSecurityReviewGenericKeyRoutesBypassAIAgentProfile$' -count=1
ok github.com/coder/coder/v2/coderd
```

**INFERRED impact.** The resulting human-owned token no longer resolves through `ai_agents`, so deleting or revoking the AI identity does not revoke it. This breaks both the no-self-escalation invariant and the intended lifecycle boundary. Creating the initial unsafe key requires a site owner or another caller authorized to create keys for the agent user, so this is also a dangerous administrative footgun rather than a low-privilege unauthenticated path.

**Recommendation.** Reject AI-agent targets in every generic key creation and login/session flow. As defense in depth, authentication should reject any key owned by an AI-agent user whose scopes and allow-list shape do not match a recognized profile.

### Medium: Workspace and sandbox bearer tokens can read raw historical build parameters

**OBSERVED.** Workspace and sandbox profiles include `workspace:read` for one exact workspace (`coderd/aiagentidentity/profile.go:54-89`). Reading a workspace build is authorized by reading its workspace, and `GetWorkspaceBuildParameters` explicitly treats build readability as sufficient to return parameters (`coderd/database/dbauthz/dbauthz.go:5684-5692,5723-5731`). The API returns parameter names and values without redaction or a sensitivity flag (`coderd/workspacebuilds.go:938-960`; `codersdk/workspacebuilds.go:131-135`).

The sandbox session token is returned in plaintext to the parent workspace agent (`coderd/aisandboxes.go:98-124,202-208`). The workspace opt-in token is supplied to the Terraform provisioner environment (`coderd/provisionerdserver/provisionerdserver.go:653-704,3307-3323`; `provisioner/terraform/provision.go:362-385`).

**INFERRED impact.** A sandbox holding its scoped bearer token can enumerate builds for the pinned workspace and read raw historical rich-parameter values. Templates sometimes use parameters for bootstrap credentials, repository tokens, or passwords. The profile description "no user-data scopes" does not protect values classified as workspace build data.

**Recommendation.** Separate build-parameter reads from ordinary workspace reads, or redact values marked or configured as sensitive. Add an end-to-end API test with an intentionally sensitive historical parameter.

### Medium: Sandbox keys survive workspace stop and can race with deletion

**OBSERVED.** Sandbox credentials use deterministic names `ai-sb-<sandbox>` and inherit workspace read, update, start, stop, SSH, and application-connect (`coderd/aiagentidentity/profile.go:54-89`). Workspace stop and delete transitions revoke only the deterministic workspace token `ai-ws-<workspace>` (`coderd/provisionerdserver/provisionerdserver.go:699-703,3361-3388`). Sandbox keys are deleted only by the explicit sandbox deletion path (`coderd/aisandboxes.go:251-324`).

**INFERRED impact.** A sandbox that copies its token outside the workspace can continue using it after workspace stop, for up to 24 hours. It can restart the stopped workspace and reconnect if the sponsor's live roles still permit those actions.

Sandbox reconcile and delete are also uncoordinated. Reconcile rotates the token outside a transaction or advisory lock (`coderd/aisandboxes.go:98-125,294-306`). Delete independently deletes the currently observed token and then soft-deletes the sandbox (`coderd/aisandboxes.go:251-288,309-324`). A replacement can be minted after delete has looked up or removed the old key, leaving a valid token for a deleted sandbox.

**Recommendation.** On workspace stop/delete, revoke every sandbox token associated with the workspace. Serialize reconcile, rotate, and delete by sandbox ID, re-read state under the lock, and make the final key replacement or revocation atomic.

### Medium: `validateProfile` is an allow-by-default denylist that has already drifted far from the catalog

**OBSERVED.** The generated API key enum has 236 values (`coderd/database/models.go:287-520,563-803`). The RBAC catalog has 50 concrete resource types plus the wildcard resource (`coderd/rbac/object_gen.go:11-497`). `validateProfile` rejects only four named composite/special scopes and applies semantic rules to `api_key`, `user_secret`, `user_skill`, `user`, and `template`; every other valid scope is accepted (`coderd/aiagentidentity/profile.go:92-133`).

The exact count is:

- 236 valid scopes.
- 30 rejected.
- 206 accepted.
- 177 accepted scopes are internal-only according to the public scope catalog (`coderd/rbac/scopes_catalog.go:8-10,77-105`).
- 45 accepted scopes are `resource:*` wildcards.
- Accepted scopes span 47 of 50 concrete resource families.

Dangerous accepted examples include:

- `assign_role:*` and `assign_org_role:*`.
- `crypto_key:*` and `oauth2_app_secret:*`.
- `ai_gateway_key:*`.
- `provisioner_daemon:*` and `system:*`.
- `workspace:*`, `coder:workspaces.delete`, and `coder:templates.build`.
- `audit_log:read`, `connection_log:read`, and deployment configuration scopes.

Composite `coder:*` scopes are validated by their string prefix as if `coder` were a resource. They are not expanded and checked permission by permission before minting (`coderd/rbac/scopes.go:137-175,247-267`). `MintKey` then inserts the result under restricted system authorization, so caller authorization is not a second boundary (`coderd/aiagentidentity/aiagentidentity.go:152-184`).

Current production `MintKey` call sites use only `ChatAgentProfile`, `WorkspaceAgentIdentityProfile`, and `SandboxIdentityProfile`. Arbitrary custom-profile exploitation through `MintKey` is therefore latent today. The exported `Profile` and `MintKey` APIs nevertheless present the validator as a security boundary, and a new caller can silently mint an administrative credential.

**Recommendation.** Make profile construction closed and typed, or use an explicit allowlist of exact expanded resource/action pairs. Add an exhaustive test over `database.AllAPIKeyScopeValues()` that requires every new scope to be deliberately classified.

### Medium: `user:read` and organization-member reads expose more data than the design states

**OBSERVED.** The chat profile grants `user:read` with `user:*`, not sponsor-ID pinning (`coderd/aiagentidentity/profile.go:31-49`). `GET /api/v2/users/{user}` returns the full `codersdk.User`, including email, last-seen time, status, login type, organization IDs, and roles (`coderd/users.go:756-781`; `codersdk/users.go:75-104`). Organization member responses expose email, global roles, last-seen time, status, and login type (`coderd/members.go:567-599`).

A normal sponsor's role ceiling restricts generic `user:read` to the sponsor's own user object. Elevated sponsors with site-level user read can reach other users. Organization-member enumeration depends on organization roles and sharing settings, but the wildcard does not narrow it further.

The architecture says `user:read` is needed to resolve the owner (`AI_AGENT_SECURITY_ARCHITECTURE.md:480-487`). That rationale is inaccurate for authentication: identity and owner resolution use restricted system access before subject construction (`coderd/aiagentidentity/aiagentidentity.go:193-219`; `coderd/httpmw/apikey.go:500-529`). A REST workspace-creation path may still require a user read, but that is a narrower reason and can be pinned to the sponsor UUID.

**Recommendation.** Pin user access to the sponsor ID. Review whether plain `user:read` should return email and account metadata, and avoid using the full user representation where a reduced identity record is sufficient.

### Low: Allow-list validation rejects only one representation of global access

**OBSERVED.** `validateProfile` rejects only the exact `*:*` element (`coderd/aiagentidentity/profile.go:135-151`). `NewAllowListElement` accepts `*:<uuid>` (`coderd/rbac/allowlist.go:32-44`), and policy applies that UUID across all resource types (`coderd/rbac/policy.rego:375-391`). Existing RBAC tests prove that one `*:<uuid>` entry can authorize workspace, group, and template objects sharing that ID (`coderd/rbac/authz_internal_test.go:1574-1607`).

The normalizer permits 128 entries (`coderd/rbac/allowlist.go:14-17,79-85`). There are only 50 concrete resource types, so a profile can enumerate `<type>:*` for every type and semantically cover the entire resource catalog despite the error text saying AI profiles cannot grant every resource.

Specific-ID validation also assumes UUID IDs. Some RBAC resources use other shapes, such as string API key IDs or resources with no object ID, so arbitrary profiles often cannot be safely pinned and will fall back to typed wildcards.

**Recommendation.** Reject wildcard resource types in AI profiles, validate the complete normalized allow-list shape, and detect semantic all-resource coverage rather than only the literal `*:*` spelling.

### Low: Workspace and sandbox keys have a 24-hour availability cliff; chat keys can be resurrected after expiry

**OBSERVED.** `MintKey` creates fixed 24-hour `LoginTypeToken` credentials (`coderd/aiagentidentity/profile.go:14`; `coderd/aiagentidentity/aiagentidentity.go:166-180`). Normal HTTP authentication rejects expired keys and does not sliding-refresh token-login keys (`coderd/httpmw/apikey.go:252-274,429-439`). AI Gateway authorization also rejects expiry, including delegated key-ID requests (`coderd/aibridgedserver/aibridgedserver.go:786-800`). I found no authorization path for AI bearer credentials that omitted expiry enforcement. `APIKeyFromRequest` does not check expiry, but its only caller uses it to identify and delete an old key during login conversion, not to authorize a request (`coderd/httpmw/apikey.go:563-635`; `coderd/userauth.go:2052-2058`).

Workspace keys rotate only on a start build, and sandbox keys rotate only on sandbox create/reconcile. Long-running workspaces and sandboxes therefore lose API access after 24 hours. This gap is already documented as an open implementation item (`AI_AGENT_SECURITY_ARCHITECTURE.md:691-694`).

Chat gateway keys differ. `ensureChatGatewayKeyID` fetches by name without an expiry predicate. Any key with less than 12 hours remaining, including an already-expired key, is extended in place to `now+24h` (`coderd/x/chatd/synthetickey.go:23-29,49-65`). Expiry is therefore not terminal for chat gateway key IDs.

**Recommendation.** Implement renewal for workspace and sandbox credentials without silently making them long-lived. For chat keys, refuse to renew an already-expired row and mint a replacement, or explicitly document stable renewable key IDs as a deliberate exception.

### Low: Concurrent origin creation fails one caller instead of returning the winning identity

**OBSERVED.** `ResolveWorkspaceOrigin` performs a read followed by `Create`, with no origin-scoped lock and no recovery for the partial unique index (`coderd/aiagentidentity/workspace.go:22-39`). `Create` retries only username collisions (`coderd/aiagentidentity/aiagentidentity.go:88-146`). The partial index prevents two live identities for one origin (`coderd/database/migrations/000565_ai_agent_identity.up.sql:30-32`).

**EMPIRICAL.** A barrier-controlled temporary test forced two callers to observe no identity before either created one. Exactly one succeeded and the other returned `idx_ai_agents_origin` unique violation. No crash occurred and no duplicate row was created, but the API/build path receives a hard error.

Ownership-transfer resolution has a related read/revoke/create race and can fail or perform redundant revocation work.

**Recommendation.** Acquire an origin-scoped advisory lock, or recover the origin unique violation by re-reading the winning live identity and validating its sponsor before returning it.

### Low: Concurrent revoke and mint can leave a key row owned by a revoked identity

**OBSERVED.** `MintKey` resolves identity state, then generates and inserts the key separately (`coderd/aiagentidentity/aiagentidentity.go:152-184`). Workspace-origin revocation deletes a currently named key and marks the identity deleted in separate operations (`coderd/aiagentidentity/workspace.go:56-82`).

**EMPIRICAL.** A temporary test paused `MintKey` immediately before `InsertAPIKey`, marked the identity deleted, then released the insert. The mint succeeded and the key row existed while the identity was deleted.

Normal HTTP and AI Gateway authentication re-check identity state and reject this residual key (`coderd/httpmw/apikey.go:500-521`; `coderd/aibridgedserver/aibridgedserver.go:822-847`). This is currently an invariant, cleanup, and future-maintenance hazard, not a demonstrated authorization bypass. It becomes more dangerous if any future consumer validates only key existence and expiry.

**Recommendation.** Serialize mint and revoke per identity, or perform final identity validation and insertion in one transaction under a per-identity lock. Revocation should atomically mark the identity and delete all of its keys, not only one expected token name.

### Low: Key rotation is delete-before-mint and uncoordinated

**OBSERVED.** Workspace rotation deletes the old key and then mints the replacement without a transaction or advisory lock (`coderd/provisionerdserver/provisionerdserver.go:3312-3323`). Sandbox rotation does the same (`coderd/aisandboxes.go:294-306`). Chat replacement after a missing key is also an unlocked check-then-mint (`coderd/x/chatd/synthetickey.go:49-73`). Token names are uniquely constrained for token-login keys (`coderd/database/dump.sql:4880`).

**INFERRED outcomes.** Concurrent callers can produce a unique-violation error, delete a key another caller just returned, or leave a temporary no-key interval when minting fails after deletion. The synthetic legacy chat key path already uses an advisory lock and recheck for this class of race (`coderd/x/chatd/synthetickey.go:107-167`).

**Recommendation.** Apply the same lock-and-recheck pattern to deterministic AI key names.

### Low: Resolution can become stale before the request's authorization checks run

**OBSERVED.** HTTP middleware resolves the identity, verifies agent and sponsor state, loads sponsor roles, and returns a subject (`coderd/httpmw/apikey.go:477-549`). `ExtractAPIKeyMW` then stores that subject in the request context and the handler authorizes later operations with it (`coderd/httpmw/apikey.go:176-200`). There is no transaction or state-version check spanning resolution and all handler authorization checks.

**INFERRED.** A concurrent sponsor suspension, role removal, or identity revocation after subject construction can allow the already-authenticated request to finish under stale state. This is a smaller window than the previously documented long-lived WebSocket/DRPC session issue, but it means "next request" is not strictly atomic at the revocation commit boundary.

**Recommendation.** Treat one already-authenticated request as the explicit revocation granularity, or add state/version checks at selected high-impact mutations. Full per-authorization database revalidation would be expensive and should not be implied without a deliberate design decision.

## Correct behavior confirmed

- The sponsor ceiling is real. OPA requires both role or ACL permission and scope permission (`coderd/rbac/policy.rego:435-461`).
- The database allow list is intersected with expanded scopes (`coderd/database/modelmethods.go:355-374`). I found no direct allow-list bypass.
- Normal HTTP and AI Gateway bearer authorization enforce key expiry.
- AI authentication re-checks agent identity and sponsor liveness.
- Chat creation, identity creation, and initial key mint are transactionally coupled (`coderd/x/chatd/chatd.go:1351-1380`).
- The partial origin index prevents two simultaneously live identities for one origin. The concurrency problem is error handling, not duplicate rows.
- Residual keys minted after revocation are rejected by the current normal authentication paths.

## Corrections and qualifications to the prior reviews

I did not disprove the central schema review headline or the eight listed conformance findings. The following statements need correction or qualification:

1. **No-self-escalation is not an invariant for all keys owned by AI users.** `AI_AGENT_SECURITY_ARCHITECTURE.md:507-508` is true only for keys minted through the current built-in profiles. The generic key APIs bypass `validateProfile`, and the empirical test proved escape to a human sponsor token.
2. **Review 1 item 8 understated the risk of generic human-user APIs.** Admin role and membership mutation is not the only issue. Generic API-key creation can alter the delegated scope itself and create a persistent human credential chain.
3. **The `user:read` rationale is wrong as written.** Authentication does not need it to resolve the sponsor; that resolution is performed under restricted system access. If workspace creation needs the read, it can be sponsor-pinned.
4. **A statement that AI keys simply expire after 24 hours is too broad.** Workspace and sandbox bearer keys do; chat gateway key IDs are renewed and can be resurrected after expiry.
5. **"Loses access the moment the sponsor is suspended" is only true at an authentication boundary.** Review 1 correctly documented long-lived sessions. This review adds the smaller per-request resolution-to-authorization window.
6. **The workspace-deletion finding remains correct despite misleading code comments.** `revokeAIAgentIdentity` says it is used for workspace deletion (`coderd/provisionerdserver/provisionerdserver.go:3405-3407`), but the only observed caller of `revokeAIAgentIdentityForWorkspace` is the prebuild-claim path (`coderd/provisionerdserver/provisionerdserver.go:659-665`). Delete transitions revoke the workspace key but do not call the identity revoker (`coderd/provisionerdserver/provisionerdserver.go:699-703`).

## Empirical tests written for this review

The temporary tests were build-tagged, run, and then removed. They were not kept because their assertions encode vulnerable current behavior rather than a desired security contract.

| Temporary test                                           | What it proved                                                                                                                                                                                                |
|----------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `TestSecurityReviewChatProfileReachability`              | The exact chat profile authorizes read/update/start/stop/SSH/application-connect on multiple sponsor-owned workspaces; denies file read and workspace delete; elevated sponsor roles expand cross-user reach. |
| `TestSecurityReviewGenericKeyRoutesBypassAIAgentProfile` | A site owner can create a default full-scope token for an AI user, and that delegated token can create a normal token for the human sponsor.                                                                  |
| `TestSecurityReviewConcurrentWorkspaceOriginCreation`    | Two first-use callers produce one success and one origin unique-violation error, not idempotent reuse.                                                                                                        |
| `TestSecurityReviewMintCanCommitAfterRevocation`         | A mint that resolved first can insert its key after the identity is marked deleted.                                                                                                                           |

Commands and results:

```text
go test -tags securityreview ./coderd/aiagentidentity \
  -run '^TestSecurityReview' -count=1
ok github.com/coder/coder/v2/coderd/aiagentidentity

go test -tags securityreview ./coderd \
  -run '^TestSecurityReviewGenericKeyRoutesBypassAIAgentProfile$' -count=1
ok github.com/coder/coder/v2/coderd
```

## Could not verify

- A full end-to-end SSH session from a chat bearer token into a running unrelated workspace. RBAC authorization was proven, but no workspace runtime was started.
- Whether any out-of-repository Terraform provider, template, plugin, or deployment integration exposes `CODER_WORKSPACE_AI_AGENT_SESSION_TOKEN` inside the final workspace.
- How frequently real deployments store secrets in rich workspace parameters.
- Whether provisioner job acquisition absolutely prevents overlapping lifecycle operations for one workspace.
- Production expired-key purge cadence and whether deployed databases already contain non-profile-conforming keys owned by AI users.
- Out-of-repository callers of exported `aiagentidentity.Profile` and `MintKey`.
- A currently reachable descendant-chat operation that fails because the reused root identity's chat allow-list contains only the root chat UUID.
- Every endpoint that shares the coarse `workspace:update` permission. Rename is clearly reachable, but a complete endpoint-by-endpoint mutation matrix was not run.
- Race behavior under real multi-replica load. The empirical races used deterministic test barriers against PostgreSQL.

## Exhaustive scope and resource inventory

`database.AllAPIKeyScopeValues()` contains 236 exact values. The following classification applies the current `validateProfile` logic, not a judgment that every accepted scope is exploitable through a current request path.

<details>
<summary>Rejected by `validateProfile` (30)</summary>

- **`coder`**: `coder:all`, `coder:application_connect`, `coder:templates.author`, `coder:apikeys.manage_self`
- **`api_key`**: `api_key:create`, `api_key:delete`, `api_key:read`, `api_key:update`, `api_key:*`
- **`template`**: `template:create`, `template:delete`, `template:update`, `template:view_insights`, `template:*`
- **`user`**: `user:create`, `user:delete`, `user:read_personal`, `user:update`, `user:update_personal`, `user:*`
- **`user_secret`**: `user_secret:create`, `user_secret:delete`, `user_secret:read`, `user_secret:update`, `user_secret:*`
- **`user_skill`**: `user_skill:create`, `user_skill:read`, `user_skill:update`, `user_skill:delete`, `user_skill:*`

</details>

<details>
<summary>Accepted by `validateProfile` (206)</summary>

- **`aibridge_interception`**: `aibridge_interception:create`, `aibridge_interception:read`, `aibridge_interception:update`, `aibridge_interception:*`
- **`assign_org_role`**: `assign_org_role:assign`, `assign_org_role:create`, `assign_org_role:delete`, `assign_org_role:read`, `assign_org_role:unassign`, `assign_org_role:update`, `assign_org_role:*`
- **`assign_role`**: `assign_role:assign`, `assign_role:read`, `assign_role:unassign`, `assign_role:*`
- **`audit_log`**: `audit_log:create`, `audit_log:read`, `audit_log:*`
- **`connection_log`**: `connection_log:read`, `connection_log:update`, `connection_log:*`
- **`crypto_key`**: `crypto_key:create`, `crypto_key:delete`, `crypto_key:read`, `crypto_key:update`, `crypto_key:*`
- **`debug_info`**: `debug_info:read`, `debug_info:*`
- **`deployment_config`**: `deployment_config:read`, `deployment_config:update`, `deployment_config:*`
- **`deployment_stats`**: `deployment_stats:read`, `deployment_stats:*`
- **`file`**: `file:create`, `file:read`, `file:*`
- **`group`**: `group:create`, `group:delete`, `group:read`, `group:update`, `group:*`
- **`group_member`**: `group_member:read`, `group_member:*`
- **`idpsync_settings`**: `idpsync_settings:read`, `idpsync_settings:update`, `idpsync_settings:*`
- **`inbox_notification`**: `inbox_notification:create`, `inbox_notification:read`, `inbox_notification:update`, `inbox_notification:*`
- **`license`**: `license:create`, `license:delete`, `license:read`, `license:*`
- **`notification_message`**: `notification_message:create`, `notification_message:delete`, `notification_message:read`, `notification_message:update`, `notification_message:*`
- **`notification_preference`**: `notification_preference:read`, `notification_preference:update`, `notification_preference:*`
- **`notification_template`**: `notification_template:read`, `notification_template:update`, `notification_template:*`
- **`oauth2_app`**: `oauth2_app:create`, `oauth2_app:delete`, `oauth2_app:read`, `oauth2_app:update`, `oauth2_app:*`
- **`oauth2_app_code_token`**: `oauth2_app_code_token:create`, `oauth2_app_code_token:delete`, `oauth2_app_code_token:read`, `oauth2_app_code_token:*`
- **`oauth2_app_secret`**: `oauth2_app_secret:create`, `oauth2_app_secret:delete`, `oauth2_app_secret:read`, `oauth2_app_secret:update`, `oauth2_app_secret:*`
- **`organization`**: `organization:create`, `organization:delete`, `organization:read`, `organization:update`, `organization:*`
- **`organization_member`**: `organization_member:create`, `organization_member:delete`, `organization_member:read`, `organization_member:update`, `organization_member:*`
- **`prebuilt_workspace`**: `prebuilt_workspace:delete`, `prebuilt_workspace:update`, `prebuilt_workspace:*`
- **`provisioner_daemon`**: `provisioner_daemon:create`, `provisioner_daemon:delete`, `provisioner_daemon:read`, `provisioner_daemon:update`, `provisioner_daemon:*`
- **`provisioner_jobs`**: `provisioner_jobs:create`, `provisioner_jobs:read`, `provisioner_jobs:update`, `provisioner_jobs:*`
- **`replicas`**: `replicas:read`, `replicas:*`
- **`system`**: `system:create`, `system:delete`, `system:read`, `system:update`, `system:*`
- **`tailnet_coordinator`**: `tailnet_coordinator:create`, `tailnet_coordinator:delete`, `tailnet_coordinator:read`, `tailnet_coordinator:update`, `tailnet_coordinator:*`
- **`template`**: `template:read`, `template:use`
- **`usage_event`**: `usage_event:create`, `usage_event:read`, `usage_event:update`, `usage_event:*`
- **`user`**: `user:read`
- **`webpush_subscription`**: `webpush_subscription:create`, `webpush_subscription:delete`, `webpush_subscription:read`, `webpush_subscription:*`
- **`workspace`**: `workspace:application_connect`, `workspace:create`, `workspace:create_agent`, `workspace:delete`, `workspace:delete_agent`, `workspace:read`, `workspace:ssh`, `workspace:start`, `workspace:stop`, `workspace:update`, `workspace:*`, `workspace:share`, `workspace:update_agent`
- **`workspace_agent_devcontainers`**: `workspace_agent_devcontainers:create`, `workspace_agent_devcontainers:*`
- **`workspace_agent_resource_monitor`**: `workspace_agent_resource_monitor:create`, `workspace_agent_resource_monitor:read`, `workspace_agent_resource_monitor:update`, `workspace_agent_resource_monitor:*`
- **`workspace_dormant`**: `workspace_dormant:application_connect`, `workspace_dormant:create`, `workspace_dormant:create_agent`, `workspace_dormant:delete`, `workspace_dormant:delete_agent`, `workspace_dormant:read`, `workspace_dormant:ssh`, `workspace_dormant:start`, `workspace_dormant:stop`, `workspace_dormant:update`, `workspace_dormant:*`, `workspace_dormant:share`, `workspace_dormant:update_agent`
- **`workspace_proxy`**: `workspace_proxy:create`, `workspace_proxy:delete`, `workspace_proxy:read`, `workspace_proxy:update`, `workspace_proxy:*`
- **`coder`**: `coder:workspaces.create`, `coder:workspaces.operate`, `coder:workspaces.delete`, `coder:workspaces.access`, `coder:templates.build`
- **`task`**: `task:create`, `task:read`, `task:update`, `task:delete`, `task:*`
- **`boundary_usage`**: `boundary_usage:*`, `boundary_usage:delete`, `boundary_usage:read`, `boundary_usage:update`
- **`chat`**: `chat:create`, `chat:read`, `chat:update`, `chat:delete`, `chat:*`, `chat:share`
- **`ai_seat`**: `ai_seat:*`, `ai_seat:create`, `ai_seat:read`
- **`ai_model_price`**: `ai_model_price:*`, `ai_model_price:read`, `ai_model_price:update`
- **`ai_provider`**: `ai_provider:*`, `ai_provider:create`, `ai_provider:delete`, `ai_provider:read`, `ai_provider:update`
- **`boundary_log`**: `boundary_log:*`, `boundary_log:create`, `boundary_log:delete`, `boundary_log:read`
- **`ai_gateway_key`**: `ai_gateway_key:*`, `ai_gateway_key:create`, `ai_gateway_key:delete`, `ai_gateway_key:read`, `ai_gateway_key:update`
- **`workspace_build_orchestration`**: `workspace_build_orchestration:*`, `workspace_build_orchestration:create`, `workspace_build_orchestration:delete`, `workspace_build_orchestration:read`, `workspace_build_orchestration:update`

</details>

The 50 concrete RBAC resource types are:

```text
ai_gateway_key, ai_model_price, ai_provider, ai_seat,
aibridge_interception, api_key, assign_org_role, assign_role, audit_log,
boundary_log, boundary_usage, chat, connection_log, crypto_key,
debug_info, deployment_config, deployment_stats, file, group,
group_member, idpsync_settings, inbox_notification, license,
notification_message, notification_preference, notification_template,
oauth2_app, oauth2_app_code_token, oauth2_app_secret, organization,
organization_member, prebuilt_workspace, provisioner_daemon,
provisioner_jobs, replicas, system, tailnet_coordinator, task, template,
usage_event, user, user_secret, user_skill, webpush_subscription,
workspace, workspace_agent_devcontainers, workspace_agent_resource_monitor,
workspace_build_orchestration, workspace_dormant, workspace_proxy
```
