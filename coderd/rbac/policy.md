


Helpful cli commands to debug.
opa eval --format=pretty 'data.authz.allow' -d policy.rego  -i input.json
opa eval --partial --format=pretty 'data.authz.allow' -d policy.rego --unknowns input.object.owner --unknowns input.object.org_owner --unknowns input.object.acl_user_list --unknowns input.object.acl_group_list -i input.json


This policy is specifically constructed to compress to a set of queries if the
object's 'owner' and 'org_owner' fields are unknown. There is no specific set
of rules that will guarantee that this policy has this property. However, there
are some tricks. A unit test will enforce this property, so any edits that pass
the unit test will be ok.

Tricks: (It's hard to really explain this, fiddling is required)
1. Do not use unknown fields in any list/set comprehension or iteration.
2. Use the unknown fields as minimally as possible.
3. Avoid making code branches based on the value of the unknown field.
   Unknown values are like a "set" of possible values.
   (This is why rule 1 usually breaks things)
   For example:
   In the org section, we calculate the 'allow' number for all orgs, rather
   than just the input.object.org_owner. This is because if the org_owner
   changes, then we don't need to recompute any 'allow' sets. We already have
   the 'allow' for the changed value. So the answer is in a lookup table.
   The final statement 'num := allow[input.object.org_owner]' does not have
   different code branches based on the org_owner. 'num's value does, but
   that is the whole point of partial evaluation.

Permissions are evaluated at four levels: site, user, org, org_member.

For each level, two checks are performed:
- Do the subject's permissions allow them to perform this action?
- Does the subject's scope allow them to perform this action?

Additionally, each input has a "scope" that can be thought of as a second set
of permissions, including the different authorization levels. An action is
only allowed if it is allowed by both the subject's permissions, and their
current scope. This is to allow issuing tokens for a subject that have a
subset of the full subjects permissions.
For example, you may have a scope like...

  {
    "by_org_id": {
      "<org_id>": {
        "member": [{ resource_type": "workspace", "action": "*" }]
      }
    }
  }
...to limit the token to only accessing workspaces within a specific org for
an admin user, rather than having full access to everything.

Each of these checks gets a "vote", which must one of three values:
  -1 to deny (usually because of a negative permission)
   0 to abstain (no matching permission)
   1 to allow

If a level abstains, then the decision gets deferred to the next level. When
there is no "next" level to defer to it is equivalent to being denied.

The final decision is determined by evaluating each of these checks in their
proper precedence order from the `allow` rule.