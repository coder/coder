package aibridged

import (
	"sync"
)

type ToolManager interface {
	AddTools(server string, tools []*MCPTool)
	GetTool(name string) *MCPTool
	ListTools() []*MCPTool
}

type ToolRegistry map[string][]*MCPTool

var _ ToolManager = &InjectedToolManager{}

// InjectedToolManager is responsible for all injected tools.
type InjectedToolManager struct {
	mu    sync.RWMutex
	tools map[string]*MCPTool
}

//
//
//
//
// TODO: need to inject tools along with their server name
//
//
//
//

func NewInjectedToolManager(tools ToolRegistry) *InjectedToolManager {
	tm := &InjectedToolManager{}
	for server, val := range tools {
		tm.AddTools(server, val)
	}
	return tm
}

func (t *InjectedToolManager) AddTools(server string, tools []*MCPTool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.tools == nil {
		t.tools = make(map[string]*MCPTool, len(tools))
	}

	for _, tool := range tools {
		t.tools[EncodeToolID(server, tool.Name)] = tool
	}
}

func (t *InjectedToolManager) GetTool(name string) *MCPTool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.tools == nil {
		return nil
	}

	return t.tools[name]
}

func (t *InjectedToolManager) ListTools() []*MCPTool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.tools == nil {
		return nil
	}

	var out []*MCPTool
	for _, tool := range t.tools {
		out = append(out, tool)
	}
	return out
}
