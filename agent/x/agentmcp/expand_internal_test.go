package agentmcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandPluginPlaceholders(t *testing.T) {
	t.Parallel()

	scope := PluginScope{Root: "/plugins/p", DataDir: "/data/p"}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "Empty", in: "", want: ""},
		{name: "NoPlaceholders", in: "plain", want: "plain"},
		{name: "Root", in: "${PLUGIN_ROOT}/bin", want: "/plugins/p/bin"},
		{name: "Data", in: "${PLUGIN_DATA}/cache", want: "/data/p/cache"},
		{name: "Both", in: "${PLUGIN_ROOT}:${PLUGIN_DATA}", want: "/plugins/p:/data/p"},
		{name: "Repeated", in: "${PLUGIN_ROOT}${PLUGIN_ROOT}", want: "/plugins/p/plugins/p"},
		{name: "OtherBraced", in: "${HOME}/x", want: "${HOME}/x"},
		{name: "BareDollar", in: "$PLUGIN_ROOT", want: "$PLUGIN_ROOT"},
		{name: "Unterminated", in: "${PLUGIN_ROOT", want: "${PLUGIN_ROOT"},
		{name: "CaseSensitive", in: "${plugin_root}", want: "${plugin_root}"},
		{name: "MixedKnownUnknown", in: "${PLUGIN_ROOT}/${OTHER}", want: "/plugins/p/${OTHER}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, expandPluginPlaceholders(tt.in, scope))
		})
	}
}

// TestExpandPluginPlaceholders_SinglePass verifies a placeholder that
// appears inside a substituted value is not expanded again.
func TestExpandPluginPlaceholders_SinglePass(t *testing.T) {
	t.Parallel()

	scope := PluginScope{Root: "${PLUGIN_DATA}", DataDir: "/data"}
	assert.Equal(t, "${PLUGIN_DATA}/x", expandPluginPlaceholders("${PLUGIN_ROOT}/x", scope))
}
