# Authztest

Package `authztest` implements _exhaustive_ unit testing for the `authz` package.

## Why this exists

The `authz.Authorize` function has three* inputs:
- Subject (for example, a user or API key)
- Resource (for example, a workspace or a DevURL)
- Action (for example, read or write).

**Not including the ruleset, which we're keeping static for the moment.*

Normally to test a pure function like this, you'd write a table test with all of the permutations by hand, for example:

```go
func Test_Authorize(t *testing.T) {
    ....
    testCases := []struct {
        name string
        subject authz.Subject
        resource authz.Resource
        action authz.Action
        expectedError error
    }{
        {
            name: "site admin can write config",
            subject: &User{ID: "admin"},
            object: &authz.ZObject{
                OrgOwner: "default",
                ResourceType: authz.ResourceSiteConfig,
            },
            expectedError: nil,
        },
        ...
    }
    for _, testCase := range testCases {
        t.Run(testCase.Name, func(t *testing.T) { ... })
    }
}
```

This approach is problematic because of the cardinality of the RBAC model.

Recall that the legacy `pkg/access/authorize`:
- Exposes 8 possible actions, 5 possible site-level roles, 4 possible org-level roles, and 24 possible resource types
- Enforces site-wide versus organization-wide permissions separately

The new authentication model must maintain backward compatibility with this model, whilst allowing additional features such as:
- User-level ownership (which means user-level permission enforcement)
- Resources shared between users (which means permissions granular down to resource IDs)
- Custom roles

The resulting permissions model ([documented in Notion](https://www.notion.so/coderhq/Workspaces-V2-Authz-RBAC-24fd193386eb4cf79a282a2a69e8f917)) results in a large **finite** solution space in the order of **hundreds of millions**.

We want to have a high level of confidence that changes to the implementation **do not have unintended side-effects**.
This means that simply manually writing a set of test cases possibly risks errors slipping through the cracks.

Instead, we generate (almost) all possible sets of inputs to the library, and ensure that `authz.Authorize` performs as expected.

The actual investigation of the solution space is [documented in Notion](https://www.notion.so/coderhq/Authz-Exhaustive-Testing-7683ea694c6e4c12ab0124439916b13a), but the crucial take-away of that document is:
- There is a **large** but **finite** number of possible inputs to `authz.Authorize`,
- The solution space can be broken down into 9 groups, and
- Most importantly, *each group has the same expected result.*

## Testing Methodology


We group the search space into a number of groups. Each group corresponds to a set of test cases with the same expected result. Each group consists of a set of **impactful** permissions and a set of **noise** permissions.

**Impactful** permissions are the top-level permissions that are expected to override anything else, and should be the only inputs that determine the expected result.
**Noise** is simply a set of additional permissions at a lower level that *should not* be impactful.

For each group, we take the **impactful set** of permissions, and add **noise**, and combine this into a role. 

We then take the *set cross-product* of the **impactful set** and the **noise**, and assert that the expected access level of that role to perform a given action.

As some of these sets are quite large, we sample some of the noise to reduce the search space.

TODO: example.

## Permission Permutations

Recall that we define a permission as a 4-tuple of `(level, resource_type, resource_id, action)` (for example, `(site, workspace, 123, read)`).

A `Set` is a slice of permissions. The search space of all possible permissions is too large, so instead this package allows generating more meaningful sets for testing. This is equivalent to pruning in AI problems: a technique to reduce the size of the search space by removing parts that do not have significance.

This is the final pruned search space used in authz. Each set is represented by a Y, N, or _. The leftmost set in a row that is not '_' is the impactful set. The impactful set determines the access result. All other sets are non-impactful, and should include the `<nil>` permission. The resulting search space for a row is the cross product between all sets in said row. 


| Row | *    | Site | Org  | Org:mem | User | Access |
|-----|------|------|------|---------|------|--------|
| W+  | Y+_  | YN_  | YN_  | YN_     | YN_  | Y      |
| W-  | N+Y_ | YN_  | YN_  | YN_     | YN_  | N      |
| S+  | _    | Y+_  | YN_  | NY_     | NY_  | Y      |
| S-  | _    | N+Y_ | YN_  | NY_     | NY_  | N      |
| O+  | _    | _    | Y+_  | NY_     | NY_  | Y      |
| O-  | _    | _    | N+Y_ | NY_     | NY_  | N      |
| M+  | _    | _    | _    | Y+_     | NY_  | Y      |
| M-  | _    | _    | _    | N+Y_    | NY_  | N      |
| U+  | _    | _    | _    | _       | Y+_  | Y      |
| U-  | _    | _    | _    | _       | N+Y_ | N      |
| A+  | _    | _    | _    | _       | Y+_  | Y      |
| A-  | _    | _    | _    | _       | _    | N      |

Each row in the above table corresponds to a set of test cases. These are described in the next section.

## Test Cases

There are 12 possible permutations.

### Case 1: W+

TODO

### Case 2: W-

TODO

### Case 3: S+

TODO

### Case 4: S-

TODO

### Case 5: O+

TODO

### Case 6: O-

TODO

### Case 7: M+

TODO

### Case 8: M-

TODO

### Case 9: U+

TODO

### Case 10: U-

TODO

### Case 11: A+

TODO
### Case 12: A-

TODO
