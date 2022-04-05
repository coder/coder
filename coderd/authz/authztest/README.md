# Authztest

An authz permission is a combination of `level`, `resource_type`, `resource_id`, and `action`. For testing purposes, we can assume only 1 action and resource exists. This package can generate all possible permissions from this.

A `Set` is a slice of permissions. The search space of all possible sets is too large, so instead this package allows generating more meaningful sets for testing. This is equivalent to pruning in AI problems: a technique to reduce the size of the search space by removing parts that do not have significance.

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

