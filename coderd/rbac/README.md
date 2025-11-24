# Authz

Package `rbac` implements Role-Based Access Control for Coder.

See [USAGE.md](USAGE.md) for a hands-on approach to using this package.

## Overview

Authorization defines what **permission** a **subject** has to perform **actions** to **objects**:

- **Permission** is binary: _yes_ (allowed) or _no_ (denied).
- **Subject** in this case is anything that implements interface `rbac.Subject`.
- **Action** here is an enumerated list of actions. Actions can differ for each object type. They typically read like, `Create`, `Read`, `Update`, `Delete`, etc.
- **Object** here is anything that implements `rbac.Object`.

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
|--------|----------|----------|--------|
| read   | Y        | \_       | Y      |
| read   | Y        | N        | N      |
| read   | \_       | \_       | \_     |
| read   | \_       | N        | N      |

## Permission Representation

**Permissions** are represented in string format as `<sign>?<level>.<object>.<id>.<action>`, where:

- `negated` can be either `+` or `-`. If it is omitted, sign is assumed to be `+`.
- `level` is either `site`, `org`, or `user`.
- `object` is any valid resource type.
- `id` is any valid UUID v4.
- `id` is included in the permission syntax, however only scopes may use `id` to specify a specific object.
- `action` is typically `create`, `read`, `modify`, `delete`, but you can define other verbs as needed.

## Example Permissions

- `+site.app.*.read`: allowed to perform the `read` action against all objects of type `app` in a given Coder deployment.
- `-user.workspace.*.create`: user is not allowed to create workspaces.

## Levels

A user can be given (or deprived) a permission at several levels. Currently,
those levels are:

- Site-wide level
- Organization level
- User level
- Organization member level

The site-wide level is the most authoritative. Any permission granted or denied at the side-wide level is absolute. After checking the site-wide level, depending of if the resource is owned by an organization or not, it will check the other levels.

- If the resource is owned by an organization, the next most authoritative level is the organization level. It acts like the site-wide level, but only for resources within the corresponding organization. The user can use that permission on any resource within that organization.
  - After the organization level is the member level. This level only applies to resources that are owned by both the organization _and_ the user.

- If the resource is not owned by an organization, the next level to check is the user level. This level only applies to resources owned by the user and that are not owned by any organization.

```
                 ┌──────────┐
                 │   Site   │
                 └─────┬────┘
            ┌──────────┴───────────┐
         ┌──┤   Owned by an org?   ├──┐
         │  └──────────────────────┘  │
      ┌──┴──┐                      ┌──┴─┐
      │ Yes │                      │ No │
      └──┬──┘                      └──┬─┘
┌────────┴─────────┐            ┌─────┴────┐
│   Organization   │            │   User   │
└────────┬─────────┘            └──────────┘
   ┌─────┴──────┐
   │   Member   │
   └────────────┘
```

## Roles

A _role_ is a set of permissions. When evaluating a role's permission to form an action, all the relevant permissions for the role are combined at each level. Permissions at a higher level override permissions at a lower level.

The following tables show the per-level role evaluation. Y indicates that the role provides positive permissions, N indicates the role provides negative permissions, and _indicates the role does not provide positive or negative permissions. YN_ indicates that the value in the cell does not matter for the access result. The table varies depending on if the resource belongs to an organization or not.

If the resource is owned by an organization, such as a template or a workspace:

| Role (example)           | Site | Org  | OrgMember | Result |
|--------------------------|------|------|-----------|--------|
| site-admin               | Y    | YN\_ | YN\_      | Y      |
| negative-site-permission | N    | YN\_ | YN\_      | N      |
| org-admin                | \_   | Y    | YN\_      | Y      |
| non-org-member           | \_   | N    | YN\_      | N      |
| member-owned             | \_   | \_   | Y         | Y      |
| not-member-owned         | \_   | \_   | N         | N      |
| unauthenticated          | \_   | \_   | \_        | N      |

If the resource is not owned by an organization:

| Role (example)           | Site | User | Result |
|--------------------------|------|------|--------|
| site-admin               | Y    | YN\_ | Y      |
| negative-site-permission | N    | YN\_ | N      |
| user-owned               | \_   | Y    | Y      |
| not-user-owned           | \_   | N    | N      |
| unauthenticated          | \_   | \_   | N      |

## Scopes

Scopes can restrict a given set of permissions. The format of a scope matches a role with the addition of a list of resource ids. For a authorization call to be successful, the subject's roles and the subject's scopes must both allow the action. This means the resulting permissions is the intersection of the subject's roles and the subject's scopes.

An example to give a readonly token is to grant a readonly scope across all resources `+site.*.*.read`. The intersection with the user's permissions will be the readonly set of their permissions.

### Resource IDs

There exists use cases that require specifying a specific resource. If resource IDs are allowed in the roles, then there is
an unbounded set of resource IDs that be added to an "allow_list", as the number of roles a user can have is unbounded. This also adds a level of complexity to the role evaluation logic that has large costs at scale.

The use case for specifying this type of permission in a role is limited, and does not justify the extra cost. To solve this for the remaining cases (eg. workspace agent tokens), we can apply an `allow_list` on a scope. For most cases, the `allow_list` will just be `["*"]` which means the scope is allowed to be applied to any resource. This adds negligible cost to the role evaluation logic and 0 cost to partial evaluations.

Example of a scope for a workspace agent token, using an `allow_list` containing a single resource id.

```javascript
{
	"scope": {
		"name": "workspace_agent",
		"display_name": "Workspace_Agent",
		// The ID of the given workspace the agent token correlates to.
		"allow_list": ["10d03e62-7703-4df5-a358-4f76577d4e2f"],
		"site": [/* ... perms ... */],
		"org": {/* ... perms ... */},
		"user": [/* ... perms ... */]
	}
}
```

## OPA (Open Policy Agent)

Open Policy Agent (OPA) is an open source tool used to define and enforce policies.
Policies are written in a high-level, declarative language called Rego.
Coder’s RBAC rules are defined in the [`policy.rego`](policy.rego) file under the `authz` package.

When OPA evaluates policies, it binds input data to a global variable called `input`.
In the `rbac` package, this structured data is defined as JSON and contains the action, object and subject (see `regoInputValue` in [astvalue.go](astvalue.go)).
OPA evaluates whether the subject is allowed to perform the action on the object across three levels: `site`, `org`, and `user`.
This is determined by the final rule `allow`, which aggregates the results of multiple rules to decide if the user has the necessary permissions.
Similarly to the input, OPA produces structured output data, which includes the `allow` variable as part of the evaluation result.
Authorization succeeds only if `allow` explicitly evaluates to `true`. If no `allow` is returned, it is considered unauthorized.
To learn more about OPA and Rego, see https://www.openpolicyagent.org/docs.

### Application and Database Integration

- [`rbac/authz.go`](authz.go) – Application layer integration: provides the core authorization logic that integrates with Rego for policy evaluation.
- [`database/dbauthz/dbauthz.go`](../database/dbauthz/dbauthz.go) – Database layer integration: wraps the database layer with authorization checks to enforce access control.

There are two types of evaluation in OPA:

- **Full evaluation**: Produces a decision that can be enforced.
  This is the default evaluation mode, where OPA evaluates the policy using `input` data that contains all known values and returns output data with the `allow` variable.
- **Partial evaluation**: Produces a new policy that can be evaluated later when the _unknowns_ become _known_.
  This is an optimization in OPA where it evaluates as much of the policy as possible without resolving expressions that depend on _unknown_ values from the `input`.
  To learn more about partial evaluation, see this [OPA blog post](https://blog.openpolicyagent.org/partial-evaluation-162750eaf422).

Application of Full and Partial evaluation in `rbac` package:

- **Full Evaluation** is handled by the `RegoAuthorizer.Authorize()` method in [`authz.go`](authz.go).
  This method determines whether a subject (user) can perform a specific action on an object.
  It performs a full evaluation of the Rego policy, which returns the `allow` variable to decide whether access is granted (`true`) or denied (`false` or undefined).
- **Partial Evaluation** is handled by the `RegoAuthorizer.Prepare()` method in [`authz.go`](authz.go).
  This method compiles OPA’s partial evaluation queries into `SQL WHERE` clauses.
  These clauses are then used to enforce authorization directly in database queries, rather than in application code.

Authorization Patterns:

- Fetch-then-authorize: an object is first retrieved from the database, and a single authorization check is performed using full evaluation via `Authorize()`.
- Authorize-while-fetching: Partial evaluation via `Prepare()` is used to inject SQL filters directly into queries, allowing efficient authorization of many objects of the same type.
  `dbauthz` methods that enforce authorization directly in the SQL query are prefixed with `Authorized`, for example, `GetAuthorizedWorkspaces`.

## Testing

- OPA Playground: https://play.openpolicyagent.org/
- OPA CLI (`opa eval`): useful for experimenting with different inputs and understanding how the policy behaves under various conditions.
  `opa eval` returns the constraints that must be satisfied for a rule to evaluate to `true`.
  - `opa eval` requires an `input.json` file containing the input data to run the policy against.
    You can generate this file using the [gen_input.go](../../scripts/rbac-authz/gen_input.go) script.
    Note: the script currently produces a fixed input. You may need to tweak it for your specific use case.

### Full Evaluation

```bash
opa eval --format=pretty "data.authz.allow" -d policy.rego -i input.json
```

This command fully evaluates the policy in the `policy.rego` file using the input data from `input.json`, and returns the result of the `allow` variable:

- `data.authz.allow` accesses the `allow` rule within the `authz` package.
- `data.authz` on its own would return the entire output object of the package.

This command answers the question: “Is the user allowed?”

### Partial Evaluation

```bash
opa eval --partial --format=pretty 'data.authz.allow' -d policy.rego --unknowns input.object.id --unknowns input.object.owner --unknowns input.object.org_owner --unknowns input.object.acl_user_list --unknowns input.object.acl_group_list -i input.json
```

This command performs a partial evaluation of the policy, specifying a set of unknown input parameters.
The result is a set of partial queries that can be converted into `SQL WHERE` clauses and injected into SQL queries.

This command answers the question: “What conditions must be met for the user to be allowed?”

### Benchmarking

Benchmark tests to evaluate the performance of full and partial evaluation can be found in `authz_test.go`.
You can run these tests with the `-bench` flag, for example:

```bash
go test -bench=BenchmarkRBACFilter -run=^$
```

To capture memory and CPU profiles, use the following flags:

- `-memprofile memprofile.out`
- `-cpuprofile cpuprofile.out`

The script [`benchmark_authz.sh`](../../scripts/rbac-authz/benchmark_authz.sh) runs the `authz` benchmark tests on the current Git branch or compares benchmark results between two branches using [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).
`benchstat` compares the performance of a baseline benchmark against a new benchmark result and highlights any statistically significant differences.

- To run benchmark on the current branch:

  ```bash
  benchmark_authz.sh --single
  ```

- To compare benchmarks between 2 branches:

  ```bash
  benchmark_authz.sh --compare main prebuild_policy
  ```
