package agentcontext

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
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
		// every comma between values.
		body := coderdMCPServerBody(t, got[0])
		require.LessOrEqual(t, len(body)+valueCommas(body), maxContextResourceBodyBytes)
	})

	t.Run("IgnoresCommasInStrings", func(t *testing.T) {
		t.Parallel()
		// protojson adds no space after a comma in a string, so schemas
		// full of them fit coderd's cap as their length does.
		serverTools := make([]MCPTool, 3)
		for i := range serverTools {
			schema := padded(0)
			schema["$defs"] = map[string]any{"Filter": map[string]any{
				"type":        "string",
				"description": strings.Repeat(",", (64<<10)-200),
			}}
			serverTools[i] = MCPTool{Name: fmt.Sprintf("t%d", i+1), InputSchema: schema}
		}

		got := buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{{Name: "fs", Connected: true, Tools: serverTools}}, nil)
		require.Equal(t, map[string]bool{"fs/t1": true, "fs/t2": true, "fs/t3": true}, fullSchemas(got))
		body := coderdMCPServerBody(t, got[0])
		require.LessOrEqual(t, len(body)+valueCommas(body), maxContextResourceBodyBytes)
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
		t2ExtraBytes, _, err := fullSchemaExtraBytes(serverTools[1], reducedTools[1])
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

	t.Run("KeepsPushUnderDRPCMessageLimit", func(t *testing.T) {
		t.Parallel()
		// A resource error stands in for push content that no body
		// holds, so coderd's body caps do not count it but dRPC's message
		// limit does. With the reduced MCP server, it nearly fills the
		// limit, and the headroom left fits t2's full schema but not t1's.
		serverTools := []MCPTool{
			{Name: "t1", InputSchema: padded(50_000)},
			{Name: "t2", InputSchema: padded(10_000)},
		}
		reducedTools := slices.Clone(serverTools)
		for i := range reducedTools {
			reducedTools[i].InputSchema = reduced
		}
		_, t2ExtraWireBytes, err := fullSchemaExtraBytes(serverTools[1], reducedTools[1])
		require.NoError(t, err)
		others := func(errorBytes int) []Resource {
			return []Resource{{ID: "instruction_file:a", Kind: KindInstructionFile, Source: "a", Status: StatusUnreadable, Error: strings.Repeat("e", errorBytes)}}
		}
		pushWireBytes := func(others, servers []Resource) int {
			return proto.Size(pushRequestToProto(&PushRequest{Resources: slices.Concat(others, servers)}))
		}
		server := Resource{ID: resourceID(KindMCPServer, "fs"), Kind: KindMCPServer, Source: "fs", Name: "fs", Status: StatusOK, Tools: reducedTools}
		// The server keeps room for the error applyProviderCaps would
		// add if it excluded the server.
		headroom := maxPushWireBytes - exclusionWireBytes - t2ExtraWireBytes
		errorBytes := 3 << 20
		errorBytes += headroom - pushWireBytes(others(errorBytes), []Resource{server})
		require.Equal(t, headroom, pushWireBytes(others(errorBytes), []Resource{server}))

		build := func(errorBytes int) []Resource {
			return buildMCPServerResources(testutil.Logger(t), []MCPServerStatus{{Name: "fs", Connected: true, Tools: serverTools}}, others(errorBytes))
		}
		got := build(errorBytes)
		require.Equal(t, map[string]bool{"fs/t1": false, "fs/t2": true}, fullSchemas(got))
		require.LessOrEqual(t, pushWireBytes(others(errorBytes), got), maxPushWireBytes)
		// The headroom fits the longest snapshot error coderd accepts.
		push := &PushRequest{
			Version:       math.MaxUint64,
			Initial:       true,
			SnapshotError: strings.Repeat("e", maxSnapshotErrorBytes),
			Resources:     slices.Concat(others(errorBytes), got),
		}
		require.LessOrEqual(t, proto.Size(pushRequestToProto(push)), drpcsdk.MaxMessageSize)
		got = build(errorBytes + 1)
		require.Equal(t, map[string]bool{"fs/t1": false, "fs/t2": false}, fullSchemas(got))
	})

	t.Run("KeepsRoomForExclusionErrors", func(t *testing.T) {
		t.Parallel()
		// Filesystem context fills the resource count cap, so
		// applyProviderCaps excludes every MCP server after the full
		// schemas are chosen and adds an error to each. A resource error
		// stands in for the rest of the push: with the reduced servers it
		// leaves room for every full schema, but not for those errors.
		const serverCount = 40
		servers := make([]MCPServerStatus, serverCount)
		reducedServers := make([]Resource, serverCount)
		extraWireBytes := 0
		for i := range servers {
			name := fmt.Sprintf("s%02d", i)
			tool := MCPTool{Name: "t", InputSchema: padded(10)}
			reducedTool := MCPTool{Name: "t", InputSchema: reduced}
			servers[i] = MCPServerStatus{Name: name, Connected: true, Tools: []MCPTool{tool}}
			reducedServers[i] = Resource{ID: resourceID(KindMCPServer, name), Kind: KindMCPServer, Source: name, Name: name, Status: StatusOK, Tools: []MCPTool{reducedTool}}
			_, toolExtraWireBytes, err := fullSchemaExtraBytes(tool, reducedTool)
			require.NoError(t, err)
			extraWireBytes += toolExtraWireBytes
		}
		others := func(errorBytes int) []Resource {
			return []Resource{{ID: "instruction_file:a", Kind: KindInstructionFile, Source: "a", Status: StatusUnreadable, Error: strings.Repeat("e", errorBytes)}}
		}
		pushWireBytes := func(resources []Resource) int {
			return proto.Size(pushRequestToProto(&PushRequest{Resources: resources}))
		}
		headroom := maxPushWireBytes - extraWireBytes
		errorBytes := 3 << 20
		errorBytes += headroom - pushWireBytes(slices.Concat(others(errorBytes), reducedServers))
		require.Equal(t, headroom, pushWireBytes(slices.Concat(others(errorBytes), reducedServers)))

		got := buildMCPServerResources(testutil.Logger(t), servers, others(errorBytes))
		full := fullSchemas(got)
		require.True(t, full["s00/t"])
		require.False(t, full[fmt.Sprintf("s%02d/t", serverCount-1)])
		resolver := &Resolver{MaxResources: 1, MaxSnapshotBytes: DefaultMaxSnapshotBytes}
		resources, _ := resolver.applyProviderCaps(slices.Concat(others(errorBytes), got), 1, 0, nil)
		for _, r := range resources[1:] {
			require.Equal(t, StatusExcluded, r.Status, r.Name)
		}
		push := &PushRequest{
			Version:       math.MaxUint64,
			Initial:       true,
			SnapshotError: strings.Repeat("e", maxSnapshotErrorBytes),
			Resources:     resources,
		}
		require.LessOrEqual(t, proto.Size(pushRequestToProto(push)), drpcsdk.MaxMessageSize)
	})

	t.Run("BoundsExclusionErrors", func(t *testing.T) {
		t.Parallel()
		require.LessOrEqual(t, len(fmt.Sprintf(countCapExclusionFormat, math.MinInt)), maxExclusionErrorBytes)
		require.LessOrEqual(t, len(fmt.Sprintf(byteCapExclusionFormat, uint64(math.MaxUint64))), maxExclusionErrorBytes)
	})
}

// acpHarnessesWithBodyBytes returns ACP harnesses, each under coderd's
// per-resource body cap, whose bodies total bodyBytes under
// coderdBodyBytesBound. Empty config values fill half of each body:
// they count twice as much toward that bound as toward the serialized
// push, so coderd's aggregate cap binds before dRPC's message limit.
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
		harness := &workspacesdk.ACPHarness{Slug: slug, DisplayName: "x", ConfigOptions: []workspacesdk.ACPConfigOption{
			{Values: make([]workspacesdk.ACPConfigValue, target/8)},
		}}
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
// after every comma between values so the bound holds for every coderd
// binary.
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
		total += len(encoded) + valueCommas(encoded)
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
