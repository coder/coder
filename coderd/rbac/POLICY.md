# Rego authorization policy

## Code style

It's a good idea to consult the [Rego style guide](https://docs.styra.com/opa/rego-style-guide). The "Variables and Data Types" section in particular has some helpful and non-obvious advice in it.

## Debugging

Open Policy Agent provides a CLI and a playground that can be used for evaluating, formatting, testing, and linting policies.

### CLI

Below are some helpful commands you can use for debugging.

For full evaluation, run:

```sh
opa eval --format=pretty 'data.authz.allow' -d policy.rego  -i input.json
```

For partial evaluation, run:

```sh
opa eval --partial --format=pretty 'data.authz.allow' -d policy.rego \
	--unknowns input.object.owner --unknowns input.object.org_owner \
	--unknowns input.object.acl_user_list --unknowns input.object.acl_group_list \
	-i input.json
```

### Playground

Use the [Open Policy Agent Playground](https://play.openpolicyagent.org/) while editing to getting linting, code formatting, and help debugging!

You can use the contents of input.json as a starting point for your own testing input. Paste the contents of policy.rego into the left-hand side of the playground, and the contents of input.json into the "Input" section. Click "Evaluate" and you should see something like the following in the output.

```json
{
	"allow": true,
	"check_scope_allow_list": true,
	"org": 0,
	"org_member": 0,
	"org_memberships": [],
	"permission_allow": true,
	"role_allow": true,
	"scope_allow": true,
	"scope_org": 0,
	"scope_org_member": 0,
	"scope_site": 1,
	"scope_user": 0,
	"site": 1,
	"user": 0
}
```

## Levels

Permissions are evaluated at four levels: site, user, org, org_member.

For each level, two checks are performed:
- Do the subject's permissions allow them to perform this action?
- Does the subject's scope allow them to perform this action?

Each of these checks gets a "vote", which must one of three values:
- -1 to deny (usually because of a negative permission)
-  0 to abstain (no matching permission)
-  1 to allow

If a level abstains, then the decision gets deferred to the next level. When
there is no "next" level to defer to it is equivalent to being denied.

### Known-org asymmetry (org and org_member levels)

The org and org_member levels are evaluated differently depending on whether
the object's org id is known.

When the org id is unknown (partial evaluation, e.g. filtering a list), the org
id must be kept out of comprehensions and must not be branched on (see "Unknown
values" below). To satisfy that, the known-org path tests the object's org id
for membership in a set of allowed org ids instead of looking up its vote:

- The org level (`check_org_permissions`, known-org clause) only ever votes
  `1` (allow) or abstains; it never votes `-1` for a known org. The
  `not org = -1` / `not scope_org = -1` gates in the allow rules are therefore
  no-ops for a known org and only block in the `any_org` case.
- Org-level deny is instead folded into the org_member level as a ground set
  difference (`member_allow - org_deny`), so an org-level deny still blocks a
  member-level allow.

The `any_org` path ("can the subject do this in any org?") still uses the full
`-1`/`0`/`1` vote (the `max` over the vote map), because there is no specific
object org id to be unknown. So do not assume `org == -1` signals an org-level
deny for a known org; reconstruct it from `org_ids_with_vote(role_org_votes, -1)`
if you need it.

### Scope
Additionally, each input has a "scope" that can be thought of as a second set of permissions, where each permission belongs to one of the four levels–exactly the same as role permissions. An action is only allowed if it is allowed by both the subject's permissions _and_ their current scope. This is to allow issuing tokens for a subject that have a subset of the full subjects permissions.

For example, you may have a scope like...

```json
{
  "by_org_id": {
    "<org_id>": {
      "member": [{ "resource_type": "workspace", "action": "*" }]
    }
  }
}
```

...to limit the token to only accessing workspaces owned by the user within a specific org. This provides some assurances for an admin user, that the token can only access intended resources, rather than having full access to everything.

The final policy decision is determined by evaluating each of these checks in their proper precedence order from the `allow` rule.

## AI agent workspace designation

An AI agent acts with its sponsoring human's roles, so `input.subject.id` is the sponsor and cannot identify the agent. The acting identity travels separately in `input.subject.ai_agent_id`, and a workspace records the identity it belongs to in `input.object.ai_agent_id` (from `workspaces.ai_agent_id`).

The `allow` rule therefore carries a third conjunct, `ai_workspace_designation_allow`, beside `permission_allow` and `scope_allow`. It always holds for human and system subjects. For an AI subject it permits `read` and `create` on workspace-typed objects, and requires `input.object.ai_agent_id = acting_ai_agent_id` for every other workspace action. Defining the exempt actions rather than the protected ones means a workspace action added later is protected by default.

This exists because scopes cannot express the boundary: an API key allow list is fixed at mint time, applies to the union of all selected scopes regardless of action, and must contain a workspace wildcard for `create` to authorize an object that has no ID yet. Designation is a property of the workspace being authorized, so it belongs on the object.

### The missing-field trap

Read the acting identity through the defaulted rule, not directly:

```rego
default acting_ai_agent_id := ""

acting_ai_agent_id := input.subject.ai_agent_id
```

In rego, `not input.subject.missing_field = ""` evaluates to **true**. Reading the field directly would therefore classify a subject whose field is absent as an AI agent and deny it every protected workspace action, so a regression in `astvalue.go` that stopped emitting the field would remove SSH from every human. Defaulting to the empty string makes an absent field mean "not an AI agent", which fails in the safe direction. Fail-closed behavior is kept where it belongs: a subject whose type says `ai_agent` but whose acting identity is empty is denied protected actions.

Two related invariants live in Go rather than in this policy:

1. `astvalue.go` always emits both fields, including as empty strings, so the comparison operates on concrete values.
2. `Object.All()` clears the designation. An aggregate object covers every resource of a type and cannot claim one designation, so protected AI authorizations against it fail closed.

### Partial evaluation

`input.object.ai_agent_id` is an unknown, and `regosql` requires a registered converter for it (`COALESCE(workspaces.ai_agent_id :: text, '')`). Because the action and object type are ground during partial evaluation, the `read` exemption resolves at compile time and workspace list filtering emits no designation predicate; `TestAIDesignationPartialEvaluation` asserts an AI actor's read filter is identical to a human's.

## Unknown values

This policy is specifically constructed to compress to a set of queries if 'input.object.owner' and 'input.object.org_owner' are unknown. There is no specific set of rules that will guarantee that this policy has this property, however, there are some tricks. We have tests that enforce this property, so any changes that pass the tests will be okay.

Some general rules to follow:

1. Do not use unknown values in any [comprehensions](https://www.openpolicyagent.org/docs/latest/policy-language/#comprehensions) or iterations.

2. Use the unknown values as minimally as possible.

3. Avoid making code branches based on the value of the unknown field.

Unknown values are like a "set" of possible values (which is why rule 1 usually breaks things).

For example, in the org level rules, we calculate the "vote" for all orgs, rather than just the `input.object.org_owner`. This way, if the `org_owner` changes, then we don't need to recompute any votes; we already have it for the changed value. This means we don't need branching, because the end result is just a lookup table.
