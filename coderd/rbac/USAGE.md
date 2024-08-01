# Using RBAC

# Overview

> _NOTE: you should probably read [`README.md`](README.md) beforehand, but it's
> not essential._

## Basic structure

RBAC is made up of nouns (the objects which are protected by RBAC rules) and
verbs (actions which can be performed on nouns).<br> For example, a
**workspace** (noun) can be **created** (verb), provided the requester has
appropriate permissions.

## Roles

We have a number of roles (some of which have legacy connotations back to v1).

These can be found in `coderd/rbac/roles.go`.

| Role                 | Description                                                         | Example resources (non-exhaustive)           |
| -------------------- | ------------------------------------------------------------------- | -------------------------------------------- |
| **owner**            | Super-user, first user in Coder installation, has all\* permissions | all\*                                        |
| **member**           | A regular user                                                      | workspaces, own details, provisioner daemons |
| **auditor**          | Viewer of audit log events, read-only access to a few resources     | audit logs, templates, users, groups         |
| **templateAdmin**    | Administrator of templates, read-only access to a few resources     | templates, workspaces, users, groups         |
| **userAdmin**        | Administrator of users                                              | users, groups, role assignments              |
| **orgAdmin**         | Like **owner**, but scoped to a single organization                 | _(org-level equivalent)_                     |
| **orgMember**        | Like **member**, but scoped to a single organization                | _(org-level equivalent)_                     |
| **orgAuditor**       | Like **auditor**, but scoped to a single organization               | _(org-level equivalent)_                     |
| **orgUserAdmin**     | Like **userAdmin**, but scoped to a single organization             | _(org-level equivalent)_                     |
| **orgTemplateAdmin** | Like **templateAdmin**, but scoped to a single organization         | _(org-level equivalent)_                     |

**Note an example resource indicates the role has at least 1 permission related
to the resource. Not that the role has complete CRUD access to the resource.**

_\* except some, which are not important to this overview_

## Actions

Roles are collections of permissions (we call them _actions_).

These can be found in `coderd/rbac/policy/policy.go`.

| Action                  | Description                             |
| ----------------------- | --------------------------------------- |
| **create**              | Create a resource                       |
| **read**                | Read a resource                         |
| **update**              | Update a resource                       |
| **delete**              | Delete a resource                       |
| **use**                 | Use a resource                          |
| **read_personal**       | Read owned resource                     |
| **update_personal**     | Update owned resource                   |
| **ssh**                 | SSH into a workspace                    |
| **application_connect** | Connect to workspace apps via a browser |
| **view_insights**       | View deployment insights                |
| **start**               | Start a workspace                       |
| **stop**                | Stop a workspace                        |
| **assign**              | Assign user to role / org               |

# Creating a new noun

In the following example, we're going to create a new RBAC noun for a new entity
called a "frobulator" _(just some nonsense word for demonstration purposes)_.

_Refer to https://github.com/coder/coder/pull/14055 to see a full
implementation._

## Creating a new entity

If you're creating a new resource which has to be owned by users of differing
roles, you need to create a new RBAC resource.

Let's say we're adding a new table called `frobulators` (we'll use this table
later):

```sql
CREATE TABLE frobulators
(
  id           uuid NOT NULL,
  user_id      uuid NOT NULL,
  model_number TEXT NOT NULL,
  PRIMARY KEY (id),
  UNIQUE (model_number),
  FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
```

Let's now add our frobulator noun to `coderd/rbac/policy/policy.go`:

```go
    ...
	"frobulator": {
		Actions: map[Action]ActionDefinition{
			ActionCreate: actDef("create a frobulator"),
			ActionRead:   actDef("read a frobulator"),
			ActionUpdate: actDef("update a frobulator"),
			ActionDelete: actDef("delete a frobulator"),
		},
	},
    ...
```

Entries in the `frobulators` table be created/read/updated/deleted, so we define
those actions.

`policy.go` is used to generate code in `coderd/rbac/object_gen.go`, and we can
execute this by running `make gen`.

Now we have this change in `coderd/rbac/object_gen.go`:

```go
    ...
    // ResourceFrobulator
    // Valid Actions
    //  - "ActionCreate" ::
    //  - "ActionDelete" ::
    //  - "ActionRead" ::
    //  - "ActionUpdate" ::
    ResourceFrobulator = Object{
        Type: "frobulator",
    }
    ...

    func AllResources() []Objecter {
    	...
    	ResourceFrobulator,
    	...
    }
```

This creates a resource which represents this noun, and adds it to a list of all
available resources.

## Role Assignment

In our case, we want **members** to be able to CRUD their own frobulators and we
want **owners** to CRUD all members' frobulators. This is how most resources
work, and the RBAC system is setup for this by default.

However, let's say we want **organization auditors** to have read-only access to
all organization's frobulators; we need to add it to `coderd/rbac/roles.go`:

```go
func ReloadBuiltinRoles(opts *RoleOptions) {
	...
		orgAuditor: func(organizationID uuid.UUID) Role {
			...
			return Role{
				...
				Org: map[string][]Permission{
					organizationID.String(): Permissions(map[string][]policy.Action{
						...
						ResourceFrobulator.Type: {policy.ActionRead},
					})
				...
	...
}
```

## Testing

The RBAC system is configured to test all possible actions on all available
resources.

Let's run the RBAC test suite:

`go test github.com/coder/coder/v2/coderd/rbac`

We'll see a failure like this:

```bash
--- FAIL: TestRolePermissions (0.61s)
    --- FAIL: TestRolePermissions/frobulator-AllActions (0.00s)
        roles_test.go:705:
            	Error Trace:	/tmp/coder/coderd/rbac/roles_test.go:705
            	Error:      	Not equal:
            	            	expected: map[policy.Action]bool{}
            	            	actual  : map[policy.Action]bool{"create":true, "delete":true, "read":true, "update":true}

            	            	Diff:
            	            	--- Expected
            	            	+++ Actual
            	            	@@ -1,2 +1,6 @@
            	            	-(map[policy.Action]bool) {
            	            	+(map[policy.Action]bool) (len=4) {
            	            	+ (policy.Action) (len=6) "create": (bool) true,
            	            	+ (policy.Action) (len=6) "delete": (bool) true,
            	            	+ (policy.Action) (len=4) "read": (bool) true,
            	            	+ (policy.Action) (len=6) "update": (bool) true
            	            	 }
            	Test:       	TestRolePermissions/frobulator-AllActions
            	Messages:   	remaining permissions should be empty for type "frobulator"
FAIL
FAIL	github.com/coder/coder/v2/coderd/rbac	1.314s
FAIL
```

The message `remaining permissions should be empty for type "frobulator"`
indicates that we're missing tests which validate the desired actions on our new
noun.

> Take a look at `coderd/rbac/roles_test.go` in the
> [reference PR](https://github.com/coder/coder/pull/14055) for a complete
> example

Let's add a test case:

```go
func TestRolePermissions(t *testing.T) {
    ...
    {
        // Users should be able to modify their own frobulators
        // Admins from the current organization should be able to modify any other members' frobulators
        // Owner should be able to modify any other user's frobulators
        Name:     "FrobulatorsModify",
        Actions:  []policy.Action{policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete},
        Resource: rbac.ResourceFrobulator.WithOwner(currentUser.String()).InOrg(orgID),
        AuthorizeMap: map[bool][]hasAuthSubjects{
            true:  {orgMemberMe, orgAdmin, owner},
            false: {setOtherOrg, memberMe, templateAdmin, userAdmin, orgTemplateAdmin, orgUserAdmin, orgAuditor},
        },
    },
    {
        // Users should be able to read their own frobulators
        // Admins from the current organization should be able to read any other members' frobulators
        // Auditors should be able to read any other members' frobulators
        // Owner should be able to read any other user's frobulators
        Name:     "FrobulatorsReadOnly",
        Actions:  []policy.Action{policy.ActionRead},
        Resource: rbac.ResourceFrobulator.WithOwner(currentUser.String()).InOrg(orgID),
        AuthorizeMap: map[bool][]hasAuthSubjects{
            true:  {orgMemberMe, orgAdmin, owner, orgAuditor},
            false: {setOtherOrg, memberMe, templateAdmin, userAdmin, orgTemplateAdmin, orgUserAdmin},
        },
    },
```

Note how the `FrobulatorsModify` test case is just validating the
`policy.ActionCreate, policy.ActionUpdate, policy.ActionDelete` actions, and
only the **orgMember**, **orgAdmin**, and **owner** can access it.

Similarly, the `FrobulatorsReadOnly` test case is only validating
`policy.ActionRead`, which is allowed on all of the above plus the
**orgAuditor** role.

Now the tests pass, because we have covered all the possible scenarios:

```bash
$ go test github.com/coder/coder/v2/coderd/rbac -count=1
ok  	github.com/coder/coder/v2/coderd/rbac	1.313s
```

When a case is not covered, you'll see an error like this (I moved the
`orgAuditor` option from `true` to `false):

```bash
--- FAIL: TestRolePermissions (0.79s)
    --- FAIL: TestRolePermissions/FrobulatorsReadOnly (0.01s)
        roles_test.go:737:
            	Error Trace:	/tmp/coder/coderd/rbac/roles_test.go:737
            	Error:      	An error is expected but got nil.
            	Test:       	TestRolePermissions/FrobulatorsReadOnly
            	Messages:   	Should fail: FrobulatorsReadOnly as "org_auditor" doing "read" on "frobulator"
FAIL
FAIL	github.com/coder/coder/v2/coderd/rbac	1.390s
FAIL
```

This shows you that the `org_auditor` role has `read` permissions on the
frobulator, but no test case covered it.

**NOTE: don't just add cases which make the tests pass; consider all the way in
which your resource must be used, and test all of those scenarios!**

# Database authorization

Now that we have the RBAC system fully configured, we need to make use of it.

Let's add a SQL query to `coderd/database/queries/frobulators.sql`:

```sql
-- name: GetUserFrobulators :many
SELECT *
FROM frobulators
WHERE user_id = @user_id::uuid;
```

Once we run `make gen`, we'll find some stubbed code in
`coderd/database/dbauthz/dbauthz.go`.

```go
...
func (q *querier) GetUserFrobulators(ctx context.Context) ([]database.Frobulator, error) {
    panic("not implemented")
}
...
```

Let's modify this function:

```go
...
func (q *querier) GetUserFrobulators(ctx context.Context, userID uuid.UUID) ([]database.Frobulator, error) {
  return fetch(q.log, q.auth, q.db.GetUserFrobulators)(ctx, id)
}
...
```

This states that the `policy.ActionRead` permission is required in this query on
the `ResourceFrobulator` resources, and `WithOwner(userID.String())` specifies
that this user must own the resource.

All queries are executed through `dbauthz`, and now our little frobulators are
protected!

# API authorization

API authorization is not strictly required because we have database
authorization in place, but it's a good practice to reject requests as soon as
possible when the requester is unprivileged.

> Take a look at `coderd/frobulators.go` in the
> [reference PR](https://github.com/coder/coder/pull/14055) for a complete
> example

```go
...
func (api *API) listUserFrobulators(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := httpmw.APIKey(r)
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceFrobulator.WithOwner(key.UserID.String())) {
		httpapi.Forbidden(rw)
		return
	}
	...
}
```

`api.Authorize(r, policy.ActionRead, rbac.ResourceFrobulator.WithOwner(key.UserID.String()))`
is specifying that we only want to permit a user to read their own frobulators.
If the requester does not have this permission, we forbid the request. We're
checking the user associated to the API key here because this could also be an
**owner** or **orgAdmin**, and we want to permit those users.
