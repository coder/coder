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

## Unknown values

This policy is specifically constructed to compress to a set of queries if 'input.object.owner' and 'input.object.org_owner' are unknown. There is no specific set of rules that will guarantee that this policy has this property, however, there are some tricks. We have tests that enforce this property, so any changes that pass the tests will be okay.

Some general rules to follow:

1. Do not use unknown values in any [comprehensions](https://www.openpolicyagent.org/docs/latest/policy-language/#comprehensions) or iterations.

2. Use the unknown values as minimally as possible.

3. Avoid making code branches based on the value of the unknown field.

Unknown values are like a "set" of possible values (which is why rule 1 usually breaks things).

For example, in the org level rules, we calculate the "vote" for all orgs, rather than just the `input.object.org_owner`. This way, if the `org_owner` changes, then we don't need to recompute any votes; we already have it for the changed value. This means we don't need branching, because the end result is just a lookup table.
