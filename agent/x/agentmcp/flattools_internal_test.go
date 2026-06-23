package agentmcp

import (
	"slices"
	"strings"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// flatTools flattens the Manager's per-server catalog into the prefixed
// MCPToolInfo list the Manager used to expose, so tests can assert on the
// flattened "server__tool" form. Only connected servers contribute tools.
// Tools are returned sorted by prefixed name.
func (m *Manager) flatTools() []workspacesdk.MCPToolInfo {
	var out []workspacesdk.MCPToolInfo
	for _, s := range m.Catalog() {
		if !s.Connected {
			continue
		}
		for _, t := range s.Tools {
			info := workspacesdk.MCPToolInfo{
				ServerName:  s.Name,
				Name:        s.Name + ToolNameSep + t.Name,
				Description: t.Description,
			}
			if props, ok := t.InputSchema["properties"].(map[string]any); ok {
				info.Schema = props
			}
			if req, ok := t.InputSchema["required"].([]any); ok {
				for _, r := range req {
					if str, ok := r.(string); ok {
						info.Required = append(info.Required, str)
					}
				}
			}
			out = append(out, info)
		}
	}
	slices.SortFunc(out, func(a, b workspacesdk.MCPToolInfo) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}
