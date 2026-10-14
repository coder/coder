package agentacp

import (
	"crypto/sha256"
	"encoding/json"

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
		payload, _ := json.Marshal(h)
		hash := sha256.New()
		configHash := m.configHashes[h.Slug]
		_, _ = hash.Write(configHash[:])
		_, _ = hash.Write(payload)
		_, _ = hash.Write([]byte(m.configDir))
		r := agentcontext.Resource{ID: "acp_harness:" + h.Slug, Kind: agentcontext.KindACPHarness, Source: h.Slug, SourcePath: m.configDir, SizeBytes: uint64(len(payload)), Payload: payload, Status: agentcontext.StatusOK, ACPHarness: &h}
		copy(r.ContentHash[:], hash.Sum(nil))
		if h.Error != "" {
			r.Status, r.Error = agentcontext.StatusInvalid, h.Error
		}
		out = append(out, r)
	}
	return out
}
