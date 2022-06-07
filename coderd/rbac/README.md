# Authz

Package `authz` implements AuthoriZation for Coder.

## Overview

Authorization defines what **permission** a **subject** has to perform **actions** to **objects**:
- **Permission** is binary: *yes* (allowed) or *no* (denied).
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

This can be represented by the following truth table, where Y represents *positive*, N represents *negative*, and _ represents *abstain*:

| Action | Positive | Negative | Result |
|--------|----------|----------|--------|
| read   | Y        | _        | Y      |
| read   | Y        | N        | N      |
| read   | _        | _        | _      |
| read   | _        | N        | Y      |


## Permission Representation

**Permissions** are represented in string format as `<sign>?<level>.<object>.<id>.<action>`, where:

- `negated` can be either `+` or `-`. If it is omitted, sign is assumed to be `+`.
- `level` is either `site`, `org`, or `user`.
- `object` is any valid resource type.
- `id` is any valid UUID v4.
- `action` is `create`, `read`, `modify`, or `delete`.

## Example Permissions

- `+site.*.*.read`: allowed to perform the `read` action against all objects of type `app` in a given Coder deployment.
- `-user.workspace.*.create`: user is not allowed to create workspaces.

## Roles

A *role* is a set of permissions. When evaluating a role's permission to form an action, all the relevant permissions for the role are combined at each level. Permissions at a higher level override permissions at a lower level.

The following table shows the per-level role evaluation.
Y indicates that the role provides positive permissions, N indicates the role provides negative permissions, and _ indicates the role does not provide positive or negative permissions. YN_ indicates that the value in the cell does not matter for the access result.

| Role (example)  | Site | Org | User | Result |
|-----------------|------|-----|------|--------|
| site-admin      | Y    | YN_ | YN_  | Y      |
| no-permission   | N    | YN_ | YN_  | N      |
| org-admin       | _    | Y   | YN_  | Y      |
| non-org-member  | _    | N   | YN_  | N      |
| user            | _    | _   | Y    | Y      |
|                 | _    | _   | N    | N      |
| unauthenticated | _    | _   | _    | N      |

