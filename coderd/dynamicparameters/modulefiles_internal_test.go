package dynamicparameters

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestMissingModuleFiles(t *testing.T) {
	t.Parallel()

	plan := func(calls string) string {
		return `{"configuration":{"root_module":{"module_calls":` + calls + `}}}`
	}

	const (
		noCalls     = `{}`
		remoteCall  = `{"workspace":{"source":"registry.coder.com/coder/example/coder"}}`
		localCall   = `{"workspace":{"source":"./modules/workspace"}}`
		parentCall  = `{"workspace":{"source":"../shared/workspace"}}`
		nestedLocal = `{"workspace":{"source":"./modules/workspace","module":{"module_calls":{"gate":{"source":"./modules/gate"}}}}}`
		// A local module may pull in a remote one, which is archived.
		nestedRemote = `{"workspace":{"source":"./modules/workspace","module":{"module_calls":{"ai":{"source":"registry.coder.com/coder/claude-code/coder"}}}}}`
	)

	tests := []struct {
		name            string
		cachedPlan      string
		haveModuleFiles bool
		expect          bool
	}{
		{
			name:       "remote module and archive missing",
			cachedPlan: plan(remoteCall),
			expect:     true,
		},
		{
			name:            "remote module and archive present",
			cachedPlan:      plan(remoteCall),
			haveModuleFiles: true,
			expect:          false,
		},
		{
			// Local modules ship in the template archive and are never cached,
			// so their absence from .terraform/modules is expected.
			name:       "local modules only",
			cachedPlan: plan(localCall),
			expect:     false,
		},
		{
			name:       "parent directory source is local",
			cachedPlan: plan(parentCall),
			expect:     false,
		},
		{
			name:       "nested local modules only",
			cachedPlan: plan(nestedLocal),
			expect:     false,
		},
		{
			name:       "remote module nested under a local one",
			cachedPlan: plan(nestedRemote),
			expect:     true,
		},
		{
			name:       "no modules declared",
			cachedPlan: plan(noCalls),
			expect:     false,
		},
		{
			// Without a plan there is no evidence either way, so fall back to
			// the render diagnostics.
			name:       "no plan",
			cachedPlan: "",
			expect:     false,
		},
		{
			name:       "unreadable plan",
			cachedPlan: "not json",
			expect:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			values := &database.TemplateVersionTerraformValue{
				CachedPlan:        []byte(tc.cachedPlan),
				CachedModuleFiles: uuid.NullUUID{UUID: uuid.New(), Valid: tc.haveModuleFiles},
			}

			require.Equal(t, tc.expect, missingModuleFiles(values))
		})
	}
}
