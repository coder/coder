# Authz

Package `authz` implements AuthoriZation for Coder.

## Overview

Authorization defines what **permission** a **subject** has to perform **actions** to **objects**:

- **Permission** is binary: _yes_ (allowed) or _no_ (denied).
- **Subject** in this case is anything that implements interface `authz.Subject`.
- **Action** here is an enumerated list of actions, but we stick to `Create`, `Read`, `Update`, and `Delete` here.
- **Object** here is anything that implements `authz.Object`.

## Permission Structure

A **permission** is a rule that grants or denies access for a **subject** to perform an **action** on a **object**.
A **permission** is always applied at a given **level**:

- **site** level applies to all objects in a given Coder deployment.
- **org** level applies to all objects that have an organization owner (`org_owner`)
- **user** level applies to all objects that have an owner with the same ID as the subject.

**Permissions** at a higher **level** always override permissions at a **lower** level.

The effect of a **permission** can be:

- **positive** (allows)
- **negative** (denies)
- **abstain** (neither allows or denies, not applicable)

**Negative** permissions **always** override **positive** permissions at the same level.
Both **negative** and **positive** permissions override **abstain** at the same level.

This can be represented by the following truth table, where Y represents _positive_, N represents _negative_, and \_ represents _abstain_:

| Action | Positive | Negative | Result |
| ------ | -------- | -------- | ------ |
| read   | Y        | \_       | Y      |
| read   | Y        | N        | N      |
| read   | \_       | \_       | \_     |
| read   | \_       | N        | Y      |

## Permission Representation

**Permissions** are represented in string format as `<sign>?<level>.<object>.<id>.<action>`, where:

- `negated` can be either `+` or `-`. If it is omitted, sign is assumed to be `+`.
- `level` is either `site`, `org`, or `user`.
- `object` is any valid resource type.
- `id` is any valid UUID v4.
- `id` is included in the permission syntax, however only scopes may use `id` to specify a specific object.
- `action` is `create`, `read`, `modify`, or `delete`.

## Example Permissions

- `+site.*.*.read`: allowed to perform the `read` action against all objects of type `app` in a given Coder deployment.
- `-user.workspace.*.create`: user is not allowed to create workspaces.

## Roles

A _role_ is a set of permissions. When evaluating a role's permission to form an action, all the relevant permissions for the role are combined at each level. Permissions at a higher level override permissions at a lower level.

The following table shows the per-level role evaluation.
Y indicates that the role provides positive permissions, N indicates the role provides negative permissions, and _ indicates the role does not provide positive or negative permissions. YN_ indicates that the value in the cell does not matter for the access result.

| Role (example)  | Site | Org  | User | Result |
| --------------- | ---- | ---- | ---- | ------ |
| site-admin      | Y    | YN\_ | YN\_ | Y      |
| no-permission   | N    | YN\_ | YN\_ | N      |
| org-admin       | \_   | Y    | YN\_ | Y      |
| non-org-member  | \_   | N    | YN\_ | N      |
| user            | \_   | \_   | Y    | Y      |
|                 | \_   | \_   | N    | N      |
| unauthenticated | \_   | \_   | \_   | N      |

## Scopes

Scopes can restrict a given set of permissions. The format of a scope matches a role with the addition of a list of resource ids. For a authorization call to be successful, the subject's roles and the subject's scopes must both allow the action. This means the resulting permissions is the intersection of the subject's roles and the subject's scopes.

An example to give a readonly token is to grant a readonly scope across all resources `+site.*.*.read`. The intersection with the user's permissions will be the readonly set of their permissions.

### Resource IDs

There exists use cases that require specifying a specific resource. If resource IDs are allowed in the roles, then there is
an unbounded set of resource IDs that be added to an "allow_list", as the number of roles a user can have is unbounded. This also adds a level of complexity to the role evaluation logic that has large costs at scale.

The use case for specifying this type of permission in a role is limited, and does not justify the extra cost. To solve this for the remaining cases (eg. workspace agent tokens), we can apply an `allow_list` on a scope. For most cases, the `allow_list` will just be `["*"]` which means the scope is allowed to be applied to any resource. This adds negligible cost to the role evaluation logic and 0 cost to partial evaluations.

Example of a scope for a workspace agent token, using an `allow_list` containing a single resource id.

```javascript
    "scope": {
      "name": "workspace_agent",
      "display_name": "Workspace_Agent",
      // The ID of the given workspace the agent token correlates to.
      "allow_list": ["10d03e62-7703-4df5-a358-4f76577d4e2f"],
      "site": [/* ... perms ... */],
      "org": {/* ... perms ... */},
      "user": [/* ... perms ... */]
    }
```

# Testing

You can test outside of golang by using the `opa` cli.

**Evaluation**

opa eval --format=pretty "data.authz.allow" -d policy.rego -i input.json

**Partial Evaluation**

```bash
opa eval --partial --format=pretty 'data.authz.allow' -d policy.rego --unknowns input.object.owner --unknowns input.object.org_owner --unknowns input.object.acl_user_list --unknowns input.object.acl_group_list -i input.json
```
