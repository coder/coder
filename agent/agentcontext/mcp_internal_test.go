package agentcontext

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestBuildMCPServerResources(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, buildMCPServerResources(testutil.Logger(t), nil, nil))
		require.Nil(t, buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{}, nil))
	})

	t.Run("GroupsByServerSortedWithTools", func(t *testing.T) {
		t.Parallel()
		// Tool names are whatever the server reported; the runner no
		// longer prefixes them with the server name.
		servers := []MCPServerStatus{
			{Name: "github", Connected: true, Tools: []MCPTool{
				{Name: "search", Description: "Search"},
				{Name: "create", Description: "Create"},
			}},
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read", InputSchema: map[string]any{"type": "object"}},
			}},
			// Dropped: a server with no name cannot be addressed.
			{Name: "", Connected: true, Tools: []MCPTool{{Name: "orphan"}}},
		}
		got := buildMCPServerResources(testutil.Logger(t), servers, nil)
		require.Len(t, got, 2)

		// Servers are emitted in name order: fs, then github.
		require.Equal(t, "fs", got[0].Source)
		require.Equal(t, "fs", got[0].Name)
		require.Equal(t, KindMCPServer, got[0].Kind)
		require.Equal(t, "mcp_server:fs", got[0].ID)
		require.Equal(t, StatusOK, got[0].Status)
		require.NotEqual(t, [32]byte{}, got[0].ContentHash)
		require.Len(t, got[0].Tools, 1)
		require.Equal(t, "read", got[0].Tools[0].Name)
		require.Equal(t, map[string]any{"type": "object"}, got[0].Tools[0].InputSchema)

		require.Equal(t, "github", got[1].Source)
		require.Len(t, got[1].Tools, 2)
		// Tools within a server are sorted by name: create, then search.
		require.Equal(t, "create", got[1].Tools[0].Name)
		require.Equal(t, "search", got[1].Tools[1].Name)
	})

	t.Run("ConnectedWithoutToolsSkipped", func(t *testing.T) {
		t.Parallel()
		// A connected server that has not yet reported any tools is
		// not surfaced; a later re-resolve picks it up once tools
		// arrive.
		require.Nil(t, buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "fs", Connected: true},
		}, nil))
	})

	t.Run("FailedServerSurfacesAsIssue", func(t *testing.T) {
		t.Parallel()
		got := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "broken", Connected: false, Err: "initialize \"broken\": exec: no such file"},
		}, nil)
		require.Len(t, got, 1)
		require.Equal(t, KindMCPServer, got[0].Kind)
		require.Equal(t, "broken", got[0].Source)
		require.Equal(t, "broken", got[0].Name)
		require.Equal(t, "mcp_server:broken", got[0].ID)
		require.Equal(t, StatusUnreadable, got[0].Status)
		require.Equal(t, "initialize \"broken\": exec: no such file", got[0].Error)
		require.Empty(t, got[0].Tools)
		require.NotEqual(t, [32]byte{}, got[0].ContentHash)
	})

	t.Run("FailedServerWithoutErrorGetsDefault", func(t *testing.T) {
		t.Parallel()
		got := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "broken", Connected: false},
		}, nil)
		require.Len(t, got, 1)
		require.Equal(t, StatusUnreadable, got[0].Status)
		require.Equal(t, "failed to connect", got[0].Error)
	})

	t.Run("ContentHashStableAndToolSensitive", func(t *testing.T) {
		t.Parallel()
		base := []MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read"},
			}},
		}
		h1 := buildMCPServerResources(testutil.Logger(t), base, nil)[0].ContentHash
		// Identical input is hashed identically.
		require.Equal(t, h1, buildMCPServerResources(testutil.Logger(t), base, nil)[0].ContentHash)
		// A description change flips the hash.
		require.NotEqual(t, h1, buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read files"},
			}},
		}, nil)[0].ContentHash)
		// Adding a tool flips the hash.
		require.NotEqual(t, h1, buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read"},
				{Name: "write", Description: "Write"},
			}},
		}, nil)[0].ContentHash)
		// A schema change flips the hash.
		require.NotEqual(t, h1, buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read", InputSchema: map[string]any{"type": "object"}},
			}},
		}, nil)[0].ContentHash)
	})

	t.Run("FailedServerHashErrorSensitive", func(t *testing.T) {
		t.Parallel()
		h1 := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "fs", Connected: false, Err: "boom"},
		}, nil)[0].ContentHash
		// The error text participates in the hash so a changed error
		// is detectable.
		require.NotEqual(t, h1, buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "fs", Connected: false, Err: "different"},
		}, nil)[0].ContentHash)
		// A failed server hashes differently from a connected one, so
		// the connected->failed transition is detectable.
		require.NotEqual(t, h1, buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "boom"},
			}},
		}, nil)[0].ContentHash)
	})

	t.Run("MixedServersSortedByName", func(t *testing.T) {
		t.Parallel()
		// Failed and connected servers are emitted together in name
		// order: broken (failed) before fs (ok).
		got := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{{Name: "read"}}},
			{Name: "broken", Connected: false, Err: "nope"},
		}, nil)
		require.Len(t, got, 2)
		require.Equal(t, "broken", got[0].Source)
		require.Equal(t, StatusUnreadable, got[0].Status)
		require.Equal(t, "fs", got[1].Source)
		require.Equal(t, StatusOK, got[1].Status)
	})
}

func TestBuildMCPServerResourcesInputSchemaCaps(t *testing.T) {
	t.Parallel()

	ref := map[string]any{"filter": map[string]any{"$ref": "#/$defs/Filter"}}
	padded := func(padding int) map[string]any {
		return map[string]any{
			"type":                 "object",
			"properties":           ref,
			"required":             []any{"filter"},
			"additionalProperties": false,
			"$defs": map[string]any{"Filter": map[string]any{
				"type":        "string",
				"description": strings.Repeat("x", padding),
			}},
		}
	}
	reduced := map[string]any{
		"type":       "object",
		"properties": ref,
		"required":   []any{"filter"},
	}
	tools := func(prefix string, count, padding int) []MCPTool {
		out := make([]MCPTool, count)
		for i := range out {
			out[i] = MCPTool{Name: fmt.Sprintf("%s%d", prefix, i+1), InputSchema: padded(padding)}
		}
		return out
	}
	fullSchemas := func(resources []Resource) map[string]bool {
		full := map[string]bool{}
		for _, r := range resources {
			for _, tool := range r.Tools {
				full[r.Name+"/"+tool.Name] = tool.InputSchema["$defs"] != nil
				if tool.InputSchema["$defs"] == nil {
					require.Equal(t, reduced, tool.InputSchema, tool.Name)
				}
			}
		}
		return full
	}

	t.Run("ReducesSchemaOverToolCap", func(t *testing.T) {
		t.Parallel()
		got := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{{Name: "fs", Connected: true, Tools: []MCPTool{
			{Name: "big", InputSchema: padded(64 << 10)},
			{Name: "small", InputSchema: padded(10)},
		}}}, nil)
		require.Equal(t, map[string]bool{"fs/big": false, "fs/small": true}, fullSchemas(got))
	})

	t.Run("AllocatesBudgetInToolNameOrder", func(t *testing.T) {
		t.Parallel()
		forward := tools("t", 5, 55_000)
		reversed := slices.Clone(forward)
		slices.Reverse(reversed)
		want := map[string]bool{"fs/t1": true, "fs/t2": true, "fs/t3": true, "fs/t4": true, "fs/t5": false}
		fromForward := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{{Name: "fs", Connected: true, Tools: forward}}, nil)
		fromReversed := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{{Name: "fs", Connected: true, Tools: reversed}}, nil)
		require.Equal(t, want, fullSchemas(fromForward))
		require.Equal(t, want, fullSchemas(fromReversed))
		require.Equal(t, fromForward[0].ContentHash, fromReversed[0].ContentHash)
	})

	t.Run("SharesBudgetAcrossServers", func(t *testing.T) {
		t.Parallel()
		got := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{
			{Name: "b", Connected: true, Tools: tools("t", 3, 60_000)},
			{Name: "a", Connected: true, Tools: tools("t", 3, 60_000)},
		}, nil)
		require.Equal(t, map[string]bool{
			"a/t1": true, "a/t2": true, "a/t3": true,
			"b/t1": true, "b/t2": false, "b/t3": false,
		}, fullSchemas(got))
	})

	t.Run("ReducedShapeSchemasTakeNoBudget", func(t *testing.T) {
		t.Parallel()
		// coderd cannot tell a reduced fallback from a schema that only
		// ever had type, properties, and required, so neither counts.
		simple := MCPTool{Name: "simple", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{"q": map[string]any{
				"type":        "string",
				"description": strings.Repeat("x", 30_000),
			}},
		}}
		jsonBytes := func(schema map[string]any) int {
			encoded, err := json.Marshal(schema)
			require.NoError(t, err)
			return len(encoded)
		}
		full := tools("t", 4, 60_000)
		remaining := maxMCPCatalogInputSchemasBytes
		for _, tool := range full {
			remaining -= jsonBytes(tool.InputSchema)
		}
		lastPadding := remaining - jsonBytes(padded(0))
		require.Equal(t, remaining, jsonBytes(padded(lastPadding)))
		catalog := func(lastPadding int) []MCPServerStatus {
			return []MCPServerStatus{
				{Name: "a", Connected: true, Tools: []MCPTool{simple}},
				{Name: "b", Connected: true, Tools: full},
				{Name: "c", Connected: true, Tools: []MCPTool{{Name: "last", InputSchema: padded(lastPadding)}}},
			}
		}

		got := buildMCPServerResources(testutil.Logger(t), catalog(lastPadding), nil)
		require.Equal(t, simple.InputSchema, got[0].Tools[0].InputSchema)
		require.Equal(t, map[string]bool{
			"b/t1": true, "b/t2": true, "b/t3": true, "b/t4": true, "c/last": true,
		}, fullSchemas(got[1:]))

		got = buildMCPServerResources(testutil.Logger(t), catalog(lastPadding+1), nil)
		require.False(t, fullSchemas(got[1:])["c/last"])
	})

	t.Run("KeepsServerBodyUnderCoderdCap", func(t *testing.T) {
		t.Parallel()
		// Four schemas just under the per-tool cap fit the catalog budget,
		// but with descriptions the body would pass coderd's cap.
		serverTools := tools("t", 4, (64<<10)-200)
		for i := range serverTools {
			serverTools[i].Description = strings.Repeat("d", 4<<10)
		}
		allFull := Resource{Kind: KindMCPServer, Name: "fs", Tools: serverTools}
		require.Greater(t, len(coderdMCPServerBody(t, allFull)), maxContextResourceBodyBytes)

		got := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{{Name: "fs", Connected: true, Tools: serverTools}}, nil)
		require.Equal(t, map[string]bool{"fs/t1": true, "fs/t2": true, "fs/t3": true, "fs/t4": false}, fullSchemas(got))
		// The bound holds for coderd binaries that add a space after
		// every comma.
		body := coderdMCPServerBody(t, got[0])
		require.LessOrEqual(t, len(body)+bytes.Count(body, []byte(",")), maxContextResourceBodyBytes)
	})

	t.Run("KeepsOversizedBodyReduced", func(t *testing.T) {
		t.Parallel()
		got := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{{Name: "fs", Connected: true, Tools: []MCPTool{
			{Name: "t1", Description: strings.Repeat("d", 300<<10), InputSchema: padded(10)},
		}}}, nil)
		require.Equal(t, map[string]bool{"fs/t1": false}, fullSchemas(got))
	})

	t.Run("KeepsPushUnderCoderdAggregateCap", func(t *testing.T) {
		t.Parallel()
		// ACP harness bodies stand in for ordinary context that, with
		// the reduced MCP server body, nearly fills coderd's aggregate
		// cap. The headroom left fits t2's full schema but not t1's.
		serverTools := []MCPTool{
			{Name: "t1", InputSchema: padded(50_000)},
			{Name: "t2", InputSchema: padded(10_000)},
		}
		reducedTools := slices.Clone(serverTools)
		for i := range reducedTools {
			reducedTools[i].InputSchema = reduced
		}
		reducedBodyBytes, err := coderdBodyBytesBound(mcpServerBody(Resource{Name: "fs", Tools: reducedTools}))
		require.NoError(t, err)
		t2ExtraBytes, err := fullSchemaExtraBodyBytes(serverTools[1], reducedTools[1])
		require.NoError(t, err)

		resolve := func(otherBodyBytes int) Snapshot {
			harnesses := acpHarnessesWithBodyBytes(t, otherBodyBytes)
			resolver := &Resolver{
				ACPResources: func() []Resource { return harnesses },
				MCPResources: func(others []Resource) []Resource {
					return buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{{Name: "fs", Connected: true, Tools: serverTools}}, others)
				},
			}
			snap := resolver.Resolve(nil)
			require.Len(t, snap.Resources, len(harnesses)+1)
			require.LessOrEqual(t, coderdPushBodyBytesBound(t, snap), maxContextAggregateBodyBytes)
			return snap
		}

		snap := resolve(maxContextAggregateBodyBytes - reducedBodyBytes - t2ExtraBytes)
		require.Equal(t, map[string]bool{"fs/t1": false, "fs/t2": true}, fullSchemas(snap.Resources))
		snap = resolve(maxContextAggregateBodyBytes - reducedBodyBytes - t2ExtraBytes + 1)
		require.Equal(t, map[string]bool{"fs/t1": false, "fs/t2": false}, fullSchemas(snap.Resources))
	})
}

// acpHarnessesWithBodyBytes returns ACP harnesses, each under coderd's
// per-resource body cap, whose bodies total bodyBytes under
// coderdBodyBytesBound.
func acpHarnessesWithBodyBytes(t *testing.T, bodyBytes int) []Resource {
	t.Helper()
	const perHarness = 200 << 10
	count := (bodyBytes + perHarness - 1) / perHarness
	out := make([]Resource, count)
	for i := range out {
		target := bodyBytes / count
		if i < bodyBytes%count {
			target++
		}
		slug := fmt.Sprintf("h%03d", i)
		harness := &workspacesdk.ACPHarness{Slug: slug, DisplayName: "x"}
		r := Resource{ID: "acp_harness:" + slug, Kind: KindACPHarness, Source: slug, Status: StatusOK, ACPHarness: harness}
		base := acpHarnessBodyBytesBound(t, r)
		harness.DisplayName = strings.Repeat("x", 1+target-base)
		require.Equal(t, target, acpHarnessBodyBytesBound(t, r))
		out[i] = r
	}
	return out
}

func acpHarnessBodyBytesBound(t *testing.T, r Resource) int {
	t.Helper()
	var entry agentproto.ContextResource
	setResourceBody(&entry, r)
	bodyBytes, err := coderdBodyBytesBound(entry.GetAcpHarness())
	require.NoError(t, err)
	return bodyBytes
}

// coderdPushBodyBytesBound sums the bodies of snap's push the way
// coderd/agentapi encodes them for its aggregate cap, counting a space
// after every comma so the bound holds for every coderd binary.
func coderdPushBodyBytesBound(t *testing.T, snap Snapshot) int {
	t.Helper()
	total := 0
	for _, r := range pushRequestToProto(snapshotToPushRequest(snap, true)).Resources {
		var body proto.Message
		switch b := r.Body.(type) {
		case *agentproto.ContextResource_McpServer:
			body = b.McpServer
		case *agentproto.ContextResource_AcpHarness:
			body = b.AcpHarness
		default:
			t.Fatalf("unexpected body %T", r.Body)
		}
		encoded, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(body)
		require.NoError(t, err)
		total += len(encoded) + bytes.Count(encoded, []byte(","))
	}
	return total
}

// maxContextResourceBodyBytes and maxContextAggregateBodyBytes are
// coderd/agentapi's caps on one pushed resource body and on all bodies
// of one push.
const (
	maxContextResourceBodyBytes  = 256 << 10
	maxContextAggregateBodyBytes = 4 << 20
)

// coderdMCPServerBody encodes r's MCP server body the way
// coderd/agentapi does before checking its size.
func coderdMCPServerBody(t *testing.T, r Resource) []byte {
	t.Helper()
	var entry agentproto.ContextResource
	setResourceBody(&entry, r)
	body, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(entry.GetMcpServer())
	require.NoError(t, err)
	return body
}
