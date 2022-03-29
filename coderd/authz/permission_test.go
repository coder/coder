package authz

import (
	crand "github.com/coder/coder/cryptorand"
	"testing"
)

func BenchmarkPermissionString(b *testing.B) {
	total := 10000
	if b.N < total {
		total = b.N
	}
	perms := make([]Permission, b.N)
	for n := 0; n < total; n++ {
		perms[n] = RandomPermission()
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		var _ = perms[n%total].String()
	}
}

var resourceTypes = []string{
	"project", "config", "user", "user_role",
	"workspace", "dev-url", "metric", "*",
}

var actions = []string{
	"read", "create", "delete", "modify", "*",
}

func RandomPermission() Permission {
	n, _ := crand.Intn(len(PermissionLevels))
	m, _ := crand.Intn(len(resourceTypes))
	a, _ := crand.Intn(len(actions))
	return Permission{
		Sign:         n%2 == 0,
		Level:        PermissionLevels[n],
		ResourceType: resourceTypes[m],
		ResourceID:   "*",
		Action:       actions[a],
	}
}
