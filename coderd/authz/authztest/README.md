# Authztest

An authz permission is a combination of `level`, `resource_type`, `resource_id`, and action`. For testing purposes, we can assume only 1 action and resource exists. This package can generate all possible permissions from this.

A `Set` is a slice of permissions. The search space of all possible sets is too large, so instead this package allows generating more meaningful sets for testing. This is equivalent to pruning in AI problems: a technique to reduce the size of the search space by removing parts that do not have significance.

This is the final pruned search space used in authz. Each set is represented by a ✅, ❌, or ⛶. The leftmost set in a row that is not '⛶' is the impactful set. The impactful set determines the access result. All other sets are non-impactful, and should include the `<nil>` permission. The resulting search space for a row is the cross product between all sets in said row. 

| Row | *    | Site | Org  | Org:mem | User | Access |
|-----|------|------|------|---------|------|--------|
| W+  | ✅⛶   | ✅❌⛶  | ✅❌⛶  | ✅❌⛶     | ✅❌⛶  | ✅      |
| W-  | ❌+✅⛶ | ✅❌⛶  | ✅❌⛶  | ✅❌⛶     | ✅❌⛶  | ❌      |
| S+  | ⛶    | ✅⛶   | ✅❌⛶  | ❌✅⛶     | ❌✅⛶  | ✅      |
| S-  | ⛶    | ❌+✅⛶ | ✅❌⛶  | ❌✅⛶     | ❌✅⛶  | ❌      |
| O+  | ⛶    | ⛶    | ✅⛶   | ❌✅⛶     | ❌✅⛶  | ✅      |
| O-  | ⛶    | ⛶    | ❌+✅⛶ | ❌✅⛶     | ❌✅⛶  | ❌      |
| M+  | ⛶    | ⛶    | ⛶    | ✅⛶      | ❌✅⛶  | ✅      |
| M-  | ⛶    | ⛶    | ⛶    | ❌+✅⛶    | ❌✅⛶  | ❌      |
| U+  | ⛶    | ⛶    | ⛶    | ⛶       | ✅⛶   | ✅      |
| U-  | ⛶    | ⛶    | ⛶    | ⛶       | ❌+✅⛶ | ❌      |
| A+  | ⛶    | ⛶    | ⛶    | ⛶       | ✅+⛶  | ✅      |
| A-  | ⛶    | ⛶    | ⛶    | ⛶       | ⛶    | ❌      |

