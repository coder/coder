package agentcontext

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"strings"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
)

// MCPServerStatus is a non-blocking, point-in-time view of a single MCP
// server the runner has attempted to connect to. It is the data
// buildMCPServerResources turns into a KindMCPServer resource. The
// runner owns the connection lifecycle; this type carries only the
// resolved result.
type MCPServerStatus struct {
	// Name is the server name declared in .mcp.json.
	Name string
	// Connected reports whether the runner reached the server and
	// listed its tools during the most recent reload.
	Connected bool
	// Err carries the connect/list failure when Connected is false.
	Err string
	// Tools is the server's tool list, with the tool names exactly
	// as the server reported them (no server prefix), when
	// Connected; empty otherwise.
	Tools []MCPTool
}

// buildMCPServerResources turns a per-server MCP snapshot into one
// KindMCPServer resource per server. Servers are emitted in name
// order, and tools within a server in name order, so the resource ID
// list and content hashes are deterministic across resolves.
//
// A connected server that exposes at least one tool becomes a
// StatusOK resource carrying its tools. A server that failed to
// connect becomes a StatusUnreadable resource carrying the connection
// error, so it appears in the snapshot's issues instead of vanishing.
// A connected server with no tools yet is skipped until its tools
// arrive (a later re-resolve, driven by the runner's reload, surfaces
// it). A server's .mcp.json entry still appears separately as a
// KindMCPConfig resource from the filesystem pass.
//
// Tool names are emitted exactly as the server reported them; flattening
// them into a single namespace (e.g. "server__tool") is the control
// plane's concern, since the resource already carries the server name.
//
// Full input schemas are capped by fitMCPToolInputSchemas in the same
// server and tool order, so the same catalog always yields the same
// resources.
func buildMCPServerResources(logger slog.Logger, servers []MCPServerStatus) []Resource {
	if len(servers) == 0 {
		return nil
	}
	sorted := slices.Clone(servers)
	slices.SortFunc(sorted, func(a, b MCPServerStatus) int {
		return strings.Compare(a.Name, b.Name)
	})

	resources := make([]Resource, 0, len(sorted))
	schemaBudget := maxMCPCatalogInputSchemasBytes
	for _, s := range sorted {
		if s.Name == "" {
			continue
		}
		if !s.Connected {
			errMsg := s.Err
			if errMsg == "" {
				errMsg = "failed to connect"
			}
			resources = append(resources, Resource{
				ID:          resourceID(KindMCPServer, s.Name),
				Kind:        KindMCPServer,
				Source:      s.Name,
				Name:        s.Name,
				Status:      StatusUnreadable,
				Error:       errMsg,
				ContentHash: hashMCPServerError(s.Name, errMsg),
			})
			continue
		}
		if len(s.Tools) == 0 {
			continue
		}
		serverTools := slices.Clone(s.Tools)
		slices.SortFunc(serverTools, func(a, b MCPTool) int {
			return strings.Compare(a.Name, b.Name)
		})
		res := Resource{
			ID:     resourceID(KindMCPServer, s.Name),
			Kind:   KindMCPServer,
			Source: s.Name,
			Name:   s.Name,
			Status: StatusOK,
		}
		res.Tools, schemaBudget = fitMCPToolInputSchemas(logger, res, serverTools, schemaBudget)
		res.ContentHash = hashMCPServer(s.Name, res.Tools)
		resources = append(resources, res)
	}
	if len(resources) == 0 {
		return nil
	}
	return resources
}

// Caps on the full MCP tool input schemas pushed to coderd. A tool past
// a cap keeps only type, properties, and required.
const (
	// maxMCPToolInputSchemaBytes and maxMCPCatalogInputSchemasBytes
	// cap one tool's full schema and all full schemas of one catalog,
	// matching coderd's definition caps for inline MCP servers.
	maxMCPToolInputSchemaBytes     = 64 << 10
	maxMCPCatalogInputSchemasBytes = 256 << 10
	// maxMCPServerBodyBytes is coderd's maxContextResourceBodyBytes:
	// one body past it makes coderd reject the whole push.
	maxMCPServerBodyBytes = 256 << 10
)

// fitMCPToolInputSchemas returns tools with each schema reduced unless
// its full form fits the per-tool cap, the remaining catalog budget,
// and res's MCP server body under coderd's body cap. It returns the
// unspent budget. Tools are considered in the order given.
func fitMCPToolInputSchemas(logger slog.Logger, res Resource, tools []MCPTool, budget int) ([]MCPTool, int) {
	ctx := context.Background()
	fitted := make([]MCPTool, len(tools))
	for i, tool := range tools {
		fitted[i] = tool
		fitted[i].InputSchema = reducedToolInputSchema(tool.InputSchema)
	}
	res.Tools = fitted
	bodyBytes, err := coderdBodyBytesBound(mcpServerBody(res))
	if err != nil {
		logger.Warn(ctx, "failed to measure MCP server body, sending reduced tool input schemas",
			slog.F("server", res.Name), slog.Error(err))
		return fitted, budget
	}
	for i, tool := range tools {
		if tool.InputSchema == nil {
			continue
		}
		encoded, err := json.Marshal(tool.InputSchema)
		if err != nil {
			logger.Warn(ctx, "failed to encode MCP tool input schema, sending it reduced",
				slog.F("server", res.Name), slog.F("tool", tool.Name), slog.Error(err))
			continue
		}
		if len(encoded) > min(maxMCPToolInputSchemaBytes, budget) {
			continue
		}
		extraBytes, err := fullSchemaExtraBodyBytes(tool, fitted[i])
		if err != nil {
			logger.Warn(ctx, "failed to measure MCP tool, sending its input schema reduced",
				slog.F("server", res.Name), slog.F("tool", tool.Name), slog.Error(err))
			continue
		}
		if bodyBytes+extraBytes > maxMCPServerBodyBytes {
			continue
		}
		bodyBytes += extraBytes
		budget -= len(encoded)
		fitted[i] = tool
	}
	return fitted, budget
}

// fullSchemaExtraBodyBytes returns how much sending full instead of
// reduced grows the bound on its MCP server body.
func fullSchemaExtraBodyBytes(full, reduced MCPTool) (int, error) {
	fullBytes, err := coderdBodyBytesBound(mcpToolToProto(full))
	if err != nil {
		return 0, err
	}
	reducedBytes, err := coderdBodyBytesBound(mcpToolToProto(reduced))
	if err != nil {
		return 0, err
	}
	return fullBytes - reducedBytes, nil
}

// coderdBodyBytesBound returns an upper bound on the size of msg as
// coderd encodes a pushed body for its size check. protojson puts a
// space after every comma or after none, chosen per binary, so this
// binary's encoding can be shorter than coderd's. The bound is additive:
// swapping one tool changes a body's bound by the difference of the two
// tools' bounds.
func coderdBodyBytesBound(msg proto.Message) (int, error) {
	encoded, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(msg)
	if err != nil {
		return 0, xerrors.Errorf("encode %T: %w", msg, err)
	}
	return len(encoded) + bytes.Count(encoded, []byte(",")), nil
}

// reducedToolInputSchema keeps only the type, properties, and required
// names of a schema, or returns nil when it has none of them.
func reducedToolInputSchema(m map[string]any) map[string]any {
	out := map[string]any{}
	if typ, ok := m["type"].(string); ok && typ != "" {
		out["type"] = typ
	}
	// Preserve an empty "properties" object: a nil properties
	// serializes to JSON null, which OpenAI rejects.
	if properties, ok := m["properties"].(map[string]any); ok {
		out["properties"] = properties
	}
	if required, ok := m["required"].([]any); ok && len(required) > 0 {
		out["required"] = required
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hashMCPServer produces a deterministic content hash over a server's
// identity and full tool set (name, description, and input schema) so
// any tool-set change flips the resource's content hash. The schema is
// encoded with encoding/json, which sorts map keys.
func hashMCPServer(server string, tools []MCPTool) [32]byte {
	h := sha256.New()
	writeLengthPrefixed(h, server)
	for _, t := range tools {
		writeLengthPrefixed(h, t.Name)
		writeLengthPrefixed(h, t.Description)
		if len(t.InputSchema) > 0 {
			if schema, err := json.Marshal(t.InputSchema); err == nil {
				writeLengthPrefixed(h, string(schema))
			}
		}
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// hashMCPServerError produces a deterministic content hash for a
// failed-to-connect server. The "unreadable" discriminator keeps a
// failed server's hash distinct from an OK server's, so a server that
// transitions between connected and failed (or whose error text
// changes) flips its content hash.
func hashMCPServerError(server, errMsg string) [32]byte {
	h := sha256.New()
	writeLengthPrefixed(h, "unreadable")
	writeLengthPrefixed(h, server)
	writeLengthPrefixed(h, errMsg)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
