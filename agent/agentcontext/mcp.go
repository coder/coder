package agentcontext

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// MCPServerStatus is a non-blocking, point-in-time view of a single MCP
// server the runner has attempted to connect to. It is the data
// buildMCPServerResources turns into a KindMCPServer resource. The
// runner owns the connection lifecycle; this type carries only the
// resolved result.
type MCPServerStatus struct {
	// Name is the server name declared in a legacy .mcp.json or a
	// plugin's mcp.json.
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
	// PluginName is the Agent Plugin whose mcp.json declared the
	// server; empty for a legacy .mcp.json server.
	PluginName string
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
// A plugin server's source is "<plugin>/<server>", and sources must be
// unique within a push. A plugin server whose source equals a legacy
// server's name is dropped and named in that server's warning. One
// whose source exceeds workspacesdk.MaxContextSourceBytes is dropped
// and counted on the "<plugin>/mcp.json" resource, which is the
// plugin's server named mcp.json when there is one.
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
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.PluginName, b.PluginName)
	})

	legacyServers := make(map[string]struct{}, len(sorted))
	for _, s := range sorted {
		if s.PluginName == "" && s.Name != "" {
			legacyServers[s.Name] = struct{}{}
		}
	}
	hiddenServers := make(map[string][]string)
	longSources := make(map[string]int)
	for _, s := range sorted {
		if s.PluginName == "" || s.Name == "" {
			continue
		}
		source := mcpServerSource(s)
		if _, ok := legacyServers[source]; ok {
			hiddenServers[source] = append(hiddenServers[source], fmt.Sprintf(
				"plugin %q server %q is not started because its ID %q equals this server's name; rename this server in .mcp.json to run both",
				s.PluginName, s.Name, source))
		}
		if len(source) > workspacesdk.MaxContextSourceBytes {
			longSources[s.PluginName]++
		}
	}

	resources := make([]Resource, 0, len(sorted))
	for _, s := range sorted {
		if s.Name == "" {
			continue
		}
		source := mcpServerSource(s)
		if s.PluginName != "" {
			if _, ok := legacyServers[source]; ok {
				continue
			}
			if len(source) > workspacesdk.MaxContextSourceBytes {
				continue
			}
		}
		warning := strings.Join(hiddenServers[source], "; ")
		if !s.Connected {
			errMsg := s.Err
			if errMsg == "" {
				errMsg = "failed to connect"
			}
			if warning != "" {
				errMsg += "; " + warning
			}
			resources = append(resources, Resource{
				ID:          resourceID(KindMCPServer, source),
				Kind:        KindMCPServer,
				Source:      source,
				Name:        s.Name,
				Status:      StatusUnreadable,
				Error:       errMsg,
				ContentHash: hashMCPServerError(s.Name, errMsg),
				PluginName:  s.PluginName,
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
			ID:          resourceID(KindMCPServer, source),
			Kind:        KindMCPServer,
			Source:      source,
			Name:        s.Name,
			Status:      StatusOK,
			Error:       warning,
			ContentHash: hashMCPServer(s.Name, serverTools, warning),
			Tools:       serverTools,
			PluginName:  s.PluginName,
		})
	}
	for _, plugin := range slices.Sorted(maps.Keys(longSources)) {
		source := plugin + "/" + pluginMCPFileName
		errMsg := fmt.Sprintf("MCP servers not listed because \"<plugin>/<server>\" is longer than %d bytes: %d; shorten their names in mcp.json", workspacesdk.MaxContextSourceBytes, longSources[plugin])
		if i := slices.IndexFunc(resources, func(r Resource) bool { return r.Source == source }); i >= 0 {
			r := &resources[i]
			if r.Error != "" {
				r.Error += "; "
			}
			r.Error += errMsg
			if r.Status == StatusOK {
				r.ContentHash = hashMCPServer(r.Name, r.Tools, r.Error)
			} else {
				r.ContentHash = hashMCPServerError(r.Name, r.Error)
			}
			continue
		}
		resources = append(resources, Resource{
			ID:          resourceID(KindMCPServer, source),
			Kind:        KindMCPServer,
			Source:      source,
			Name:        pluginMCPFileName,
			Status:      StatusUnreadable,
			Error:       errMsg,
			ContentHash: hashMCPServerError(pluginMCPFileName, errMsg),
			PluginName:  plugin,
		})
	}
	if len(resources) == 0 {
		return nil
	}
	return resources
}

// pluginMCPFileName also names the resource that reports problems with
// a plugin's servers as a whole.
const pluginMCPFileName = "mcp.json"

// hashMCPServer produces a deterministic content hash over a server's
// name and full tool set (name, description, and input schema) so any
// tool-set change flips the resource's content hash. The schema is
// encoded with encoding/json, which sorts map keys. A non-empty warning
// is hashed too.
func hashMCPServer(server string, tools []MCPTool, warning string) [32]byte {
	h := sha256.New()
	if warning != "" {
		writeLengthPrefixed(h, "warning")
		writeLengthPrefixed(h, warning)
	}
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

// mcpServerSource prefixes plugin servers so a name shared with another
// server still gets its own resource.
func mcpServerSource(s MCPServerStatus) string {
	if s.PluginName == "" {
		return s.Name
	}
	return s.PluginName + "/" + s.Name
}
