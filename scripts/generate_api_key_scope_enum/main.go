package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

func main() {
	seen := map[string]struct{}{}
	var vals []string
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		vals = append(vals, name)
	}

	for resource, def := range policy.RBACPermissions {
		if resource == policy.WildcardSymbol {
			continue
		}
		add(resource + ":" + policy.WildcardSymbol)
		for action := range def.Actions {
			add(fmt.Sprintf("%s:%s", resource, action))
		}
	}
	// Include composite coder:* scopes as first-class enum values.
	for _, name := range rbac.CompositeScopeNames() {
		add(name)
	}
	// Include built-in coder-prefixed scopes such as coder:all.
	for _, name := range rbac.BuiltinScopeNames() {
		s := string(name)
		if !strings.Contains(s, ":") {
			continue
		}
		add(s)
	}

	sort.Strings(vals)
	for _, v := range vals {
		_, _ = fmt.Printf("ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS '%s';\n", v)
	}
}
