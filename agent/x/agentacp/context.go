package agentacp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/coder/coder/v2/agent/agentcontext"
)

// ContextResources returns cached metadata and deterministic hashes. Raw launch
// configuration contributes to the hash but never leaves the agent.
func (m *Manager) ContextResources() []agentcontext.Resource {
	m.mu.Lock()
	defer m.mu.Unlock()
	catalog := m.catalogLocked()
	out := make([]agentcontext.Resource, 0, len(catalog))
	for _, h := range catalog {
		r := agentcontext.Resource{
			ID:         "acp_harness:" + h.Slug,
			Kind:       agentcontext.KindACPHarness,
			Source:     h.Slug,
			SourcePath: m.configDir,
			Status:     agentcontext.StatusOK,
		}
		payload, err := json.Marshal(h)
		if err != nil {
			r.Status = agentcontext.StatusInvalid
			r.Error = fmt.Sprintf("marshal ACP harness metadata: %v", err)
			out = append(out, r)
			continue
		}
		configHash := m.configHashes[h.Slug]
		hashInput := make([]byte, 0, len(configHash)+len(payload)+len(m.configDir))
		hashInput = append(hashInput, configHash[:]...)
		hashInput = append(hashInput, payload...)
		hashInput = append(hashInput, m.configDir...)
		r.ContentHash = sha256.Sum256(hashInput)
		r.SizeBytes = uint64(len(payload))
		r.Payload = payload
		r.ACPHarness = &h
		if h.Error != "" {
			r.Status, r.Error = agentcontext.StatusInvalid, h.Error
		}
		out = append(out, r)
	}
	return out
}
