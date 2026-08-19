package aibridged

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridged/proto"
)

func TestParseMCPGatewayRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantBatch bool
		wantItems int
		wantErr   bool
	}{
		{name: "single", body: ` {"jsonrpc":"2.0","id":1,"method":"ping"} `, wantItems: 1},
		{name: "batch", body: ` [{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","method":"notifications/initialized"}] `, wantBatch: true, wantItems: 2},
		{name: "empty", wantErr: true},
		{name: "empty batch", body: `[]`, wantErr: true},
		{name: "invalid", body: `{`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request, err := parseMCPGatewayRequest([]byte(tt.body))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantBatch, request.Batch)
			require.Len(t, request.Items, tt.wantItems)
		})
	}
}

func TestPlanMCPGatewayRequest(t *testing.T) {
	t.Parallel()

	cfg := &proto.MCPGatewayServerConfig{
		ToolDefault: "disabled",
		ToolRules: []*proto.MCPGatewayToolRule{
			{Tool: "allowed", Enabled: true},
		},
	}
	policy, err := newMCPGatewayPolicy(cfg)
	require.NoError(t, err)

	request, err := parseMCPGatewayRequest([]byte(`[
		{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"allowed","arguments":{"value":1}}},
		{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"denied"}},
		{"jsonrpc":"2.0","id":3,"method":"tools/list"},
		{"jsonrpc":"2.0","method":"notifications/initialized"}
	]`))
	require.NoError(t, err)

	plan, err := planMCPGatewayRequest(request, policy)
	require.NoError(t, err)
	require.Len(t, plan.forward, 3)
	require.Len(t, plan.local, 1)
	require.True(t, plan.filterResponse)
	require.Contains(t, string(plan.local[0]), `"id":2`)
	require.Contains(t, string(plan.local[0]), `denied`)
}

func TestFilterMCPGatewayResponse(t *testing.T) {
	t.Parallel()

	cfg := &proto.MCPGatewayServerConfig{
		ToolAllowList: []string{"read"},
		ToolDenyRegex: `^read_secret$`,
	}
	policy, err := newMCPGatewayPolicy(cfg)
	require.NoError(t, err)
	plan := mcpGatewayPlan{
		toolsListIDs: map[string]struct{}{"1": {}},
		policy:       policy,
	}

	filtered, err := filterMCPGatewayResponse([]byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"read"},{"name":"write"},{"name":"read_secret"}],"nextCursor":"next"}}`), plan)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools      []map[string]any `json:"tools"`
			NextCursor string           `json:"nextCursor"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(filtered, &response))
	require.Equal(t, "next", response.Result.NextCursor)
	require.Equal(t, []map[string]any{{"name": "read"}}, response.Result.Tools)
}
