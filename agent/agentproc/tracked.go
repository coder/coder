package agentproc

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

var (
	errTrackedInvalid  = xerrors.New("invalid tracked process identity")
	errTrackedConflict = xerrors.New("tracked process identity conflicts")
	errTrackedExpired  = xerrors.New("tracked process history expired")
	errTrackedCapacity = xerrors.New("tracked process identity capacity reached")
)

// Tiny receipts outlive output buffers. Never recycle an ID within an instance:
// a delayed request with changed input must not become a new execution.
const maxTrackedProcessIdentities = 16384

type processReceipt struct {
	digest   string
	chatID   uuid.UUID
	deadline time.Time
	fenced   bool
	startErr error
	terminal *workspacesdk.ProcessInfo
}

// checkTrackedStart must be called with m.mu held.
func (m *manager) checkTrackedStart(req workspacesdk.StartProcessRequest, chatID uuid.UUID) (*process, bool, error) {
	if req.ProcessID == uuid.Nil && req.AgentInstanceID == uuid.Nil && req.InputDigest == "" && req.Deadline.IsZero() {
		return nil, false, nil
	}
	if req.ProcessID == uuid.Nil || req.AgentInstanceID == uuid.Nil || req.InputDigest != workspacesdk.StartProcessDigest(req) {
		return nil, false, errTrackedInvalid
	}
	if req.AgentInstanceID != m.instanceID {
		return nil, false, errTrackedConflict
	}
	id := req.ProcessID.String()
	if receipt, ok := m.receipts[id]; ok {
		if receipt.digest != req.InputDigest || receipt.chatID != chatID || !receipt.deadline.Equal(req.Deadline) {
			return nil, false, errTrackedConflict
		}
		if receipt.fenced {
			return nil, false, errTrackedConflict
		}
		if receipt.startErr != nil {
			return nil, true, receipt.startErr
		}
		if proc, ok := m.procs[procKey{chatID: chatID, id: id}]; ok {
			return proc, true, nil
		}
		return nil, false, errTrackedExpired
	}
	if _, exists := m.procs[procKey{chatID: chatID, id: id}]; exists {
		return nil, false, errTrackedConflict
	}
	if !req.Deadline.IsZero() && !m.clock.Now().Before(req.Deadline) {
		return nil, false, errTrackedExpired
	}
	if len(m.receipts) >= maxTrackedProcessIdentities {
		return nil, false, errTrackedCapacity
	}
	return nil, false, nil
}

func (api *API) handleTrackedOutput(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("agent_instance_id") != api.manager.instanceID.String() {
		writeTrackedError(rw, r, errTrackedConflict)
		return
	}
	chat, _ := agentchat.FromContext(r.Context())
	chatID := chat.ID
	id := chi.URLParam(r, "id")
	api.manager.mu.Lock()
	proc := api.manager.procs[procKey{chatID: chatID, id: id}]
	receipt, known := api.manager.receipts[id]
	api.manager.mu.Unlock()
	if !known || receipt.chatID != chatID {
		httpapi.ResourceNotFound(rw)
		return
	}
	digest := r.URL.Query().Get("input_digest")
	if known && digest != "" && digest != receipt.digest {
		writeTrackedError(rw, r, errTrackedConflict)
		return
	}
	if proc != nil {
		api.writeProcessOutput(rw, r, proc)
		return
	}
	if known && receipt.terminal != nil && digest == receipt.digest {
		httpapi.Write(r.Context(), rw, http.StatusGone, workspacesdk.ProcessOutputUnavailableError{
			AgentInstanceID: api.manager.instanceID, InputDigest: receipt.digest,
			OutputUnavailable: true, Process: *receipt.terminal,
		})
		return
	}
	httpapi.ResourceNotFound(rw)
}

func writeTrackedError(rw http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errTrackedInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, errTrackedConflict):
		status = http.StatusConflict
	case errors.Is(err, errTrackedExpired):
		status = http.StatusGone
	case errors.Is(err, errTrackedCapacity):
		status = http.StatusTooManyRequests
	}
	httpapi.Write(r.Context(), rw, status, codersdk.Response{Message: "Tracked process request failed.", Detail: err.Error()})
}

func processWaitDuration(value string) (time.Duration, error) {
	if value == "" || value == "0" {
		return maxWaitDuration, nil
	}
	ms, err := strconv.ParseInt(value, 10, 64)
	if err != nil || ms < 0 {
		return 0, errTrackedInvalid
	}
	// Clamp before multiplying to prevent overflow.
	return time.Duration(min(ms, maxWaitDuration.Milliseconds())) * time.Millisecond, nil
}

// fenceOrCancel resolves absence under the same lock used for spawn.
func (m *manager) fenceOrCancel(req workspacesdk.CancelProcessRequest, chatID uuid.UUID) (*process, bool, *workspacesdk.ProcessInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	digest, err := hex.DecodeString(req.InputDigest)
	if req.ProcessID == uuid.Nil || req.AgentInstanceID == uuid.Nil || err != nil || len(digest) != 32 {
		return nil, false, nil, errTrackedInvalid
	}
	if m.closed || req.AgentInstanceID != m.instanceID {
		return nil, false, nil, errTrackedConflict
	}
	id := req.ProcessID.String()
	receipt, known := m.receipts[id]
	if known {
		if receipt.digest != req.InputDigest || receipt.chatID != chatID || !receipt.deadline.Equal(req.Deadline) {
			return nil, false, nil, errTrackedConflict
		}
	} else {
		if _, exists := m.procs[procKey{chatID: chatID, id: id}]; exists {
			return nil, false, nil, errTrackedConflict
		}
		if len(m.receipts) >= maxTrackedProcessIdentities {
			return nil, false, nil, errTrackedCapacity
		}
		receipt = processReceipt{digest: req.InputDigest, chatID: chatID, deadline: req.Deadline, fenced: true}
		m.receipts[id] = receipt
	}
	proc := m.procs[procKey{chatID: chatID, id: id}]
	if proc != nil {
		proc.cancel()
	}
	return proc, receipt.fenced || receipt.startErr != nil, receipt.terminal, nil
}

func (api *API) handleCancelProcess(rw http.ResponseWriter, r *http.Request) {
	var req workspacesdk.CancelProcessRequest
	if !httpapi.Read(r.Context(), rw, r, &req) {
		return
	}
	wait, err := processWaitDuration(strconv.FormatInt(req.WaitMillis, 10))
	if req.WaitMillis == 0 {
		wait = 0
	}
	if err != nil {
		writeTrackedError(rw, r, err)
		return
	}
	chat, _ := agentchat.FromContext(r.Context())
	chatID := chat.ID
	proc, fenced, terminal, err := api.manager.fenceOrCancel(req, chatID)
	if err != nil {
		writeTrackedError(rw, r, err)
		return
	}
	response := workspacesdk.CancelProcessResponse{AgentInstanceID: api.manager.instanceID, Fenced: fenced, Process: terminal}
	if proc != nil {
		api.waitForProcess(rw, r, proc, wait)
		info := proc.info()
		response.Process = &info
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, response)
}

// waitForProcess only bounds observation. It never cancels execution.
func (api *API) waitForProcess(rw http.ResponseWriter, r *http.Request, proc *process, wait time.Duration) {
	// Give the network write deadline headroom beyond the bounded wait.
	if err := http.NewResponseController(rw).SetWriteDeadline(time.Now().Add(wait + 30*time.Second)); err != nil {
		api.logger.Debug(r.Context(), "extend process response deadline", slog.Error(err))
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	timer := api.manager.clock.AfterFunc(wait, cancel, "process-wait")
	defer timer.Stop()
	_ = proc.waitForOutput(ctx)
}
