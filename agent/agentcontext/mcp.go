package agentcontext

import (
	"crypto/sha256"
	"encoding/json"
	"slices"
	"strings"
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
func buildMCPServerResources(servers []MCPServerStatus) []Resource {
	if len(servers) == 0 {
		return nil
	}
	sorted := slices.Clone(servers)
	slices.SortFunc(sorted, func(a, b MCPServerStatus) int {
		return strings.Compare(a.Name, b.Name)
	})

	resources := make([]Resource, 0, len(sorted))
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
		resources = append(resources, Resource{
			ID:          resourceID(KindMCPServer, s.Name),
			Kind:        KindMCPServer,
			Source:      s.Name,
			Name:        s.Name,
			Status:      StatusOK,
			ContentHash: hashMCPServer(s.Name, serverTools),
			Tools:       serverTools,
		})
	}
	if len(resources) == 0 {
		return nil
	}
	return resources
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
