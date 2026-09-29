package workspacesdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// TrackedProcessProtocolVersion identifies the instance-fenced process protocol.
const TrackedProcessProtocolVersion = 1

// StartProcessDigest hashes the requested execution, excluding its identity.
// JSON orders environment keys. Normalize time zones and an empty environment
// so equivalent requests have the same digest.
func StartProcessDigest(req StartProcessRequest) string {
	req.ProcessID = uuid.Nil
	req.AgentInstanceID = uuid.Nil
	req.InputDigest = ""
	req.Deadline = req.Deadline.UTC()
	if len(req.Env) == 0 {
		req.Env = nil
	}
	data, _ := json.Marshal(req)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// ProcessOutputUnavailableError carries actual terminal state after output has
// been reaped. Command and workdir are omitted; no output bytes remain available.
type ProcessOutputUnavailableError struct {
	AgentInstanceID   uuid.UUID   `json:"agent_instance_id"`
	InputDigest       string      `json:"input_digest"`
	OutputUnavailable bool        `json:"output_unavailable"`
	Process           ProcessInfo `json:"process"`
}

// Error reports unavailable output without implying an empty successful result.
func (*ProcessOutputUnavailableError) Error() string {
	return "process output unavailable: output was reaped"
}

// CancelProcessRequest cancels a planned process or fences its absent identity.
// A fence prevents a delayed start from executing on the same agent instance.
type CancelProcessRequest struct {
	ProcessID       uuid.UUID `json:"process_id"`
	AgentInstanceID uuid.UUID `json:"agent_instance_id"`
	InputDigest     string    `json:"input_digest"`
	Deadline        time.Time `json:"deadline"`
	WaitMillis      int64     `json:"wait_ms,omitempty"`
}

// CancelProcessResponse reports observed state, not merely signal delivery.
// Fenced means this identity never started. A missing Process without Fenced
// means its execution history expired and its outcome is unknown.
type CancelProcessResponse struct {
	AgentInstanceID uuid.UUID    `json:"agent_instance_id"`
	Fenced          bool         `json:"fenced"`
	Process         *ProcessInfo `json:"process,omitempty"`
}

// CancelProcess fences an absent planned identity or kills an existing process.
// It never treats sending a signal as proof that the process has exited.
func (c *agentConn) CancelProcess(ctx context.Context, req CancelProcessRequest) (CancelProcessResponse, error) {
	res, err := c.apiRequest(ctx, http.MethodPost, "/api/v0/processes/cancel-tracked", req)
	if err != nil {
		return CancelProcessResponse{}, xerrors.Errorf("cancel process: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return CancelProcessResponse{}, codersdk.ReadBodyAsError(res)
	}
	var resp CancelProcessResponse
	return resp, decodeAgentJSON(res, &resp)
}
