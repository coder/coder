package main

import (
	"fmt"
	"sort"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func main() {
	seen := map[string]struct{}{}
	var vals []string
	for resource, def := range policy.RBACPermissions {
		if resource == policy.WildcardSymbol {
			continue
		}
		for action := range def.Actions {
			vals = append(vals, fmt.Sprintf("%s:%s", resource, action))
		}
	}
	// Include composite coder:* scopes as first-class enum values
	vals = append(vals, rbac.CompositeScopeNames()...)
	sort.Strings(vals)
	for _, v := range vals {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		_, _ = fmt.Printf("ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS '%s';\n", v)
	}
}
