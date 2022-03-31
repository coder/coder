package authz_test

import (
	"testing"

	"github.com/coder/coder/coderd/authz"
	crand "github.com/coder/coder/cryptorand"
)

func BenchmarkPermissionString(b *testing.B) {
	total := 10000
	if b.N < total {
		total = b.N
	}
	perms := make([]authz.Permission, b.N)
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

func RandomPermission() authz.Permission {
	n, _ := crand.Intn(len(authz.PermissionLevels))
	m, _ := crand.Intn(len(resourceTypes))
	a, _ := crand.Intn(len(actions))
	return authz.Permission{
		Sign:         n%2 == 0,
		Level:        authz.PermissionLevels[n],
		ResourceType: resourceTypes[m],
		ResourceID:   "*",
		Action:       actions[a],
	}
}
