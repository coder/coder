package coderd

import (
	"context"
	"net/http"
	"net/netip"
	"time"

	"github.com/coder/websocket"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
)

// @Summary Connect to workload input (experimental)
// @Description Opens a WebSocket accepting JSON WorkloadInput messages. Disconnecting does not cancel the workload.
// @ID connect-to-workload-input-experimental
// @Security CoderSessionToken
// @Tags Workspaces
// @Param organization path string true "Organization ID" format(uuid)
// @Param workload path string true "Workload ID" format(uuid)
// @Success 101
// @Router /api/v2/organizations/{organization}/workloads/{workload}/input [get]
// @x-apidocgen {"skip": true}
//
//nolint:revive // HTTP GET handlers write responses rather than return values.
func (api *API) getWorkloadInput(rw http.ResponseWriter, r *http.Request) {
	run := api.authorizedWorkload(rw, r)
	if run == nil {
		return
	}
	if !run.request.Stdin {
		workloadError(rw, r, http.StatusBadRequest, "Workload stdin is not enabled.")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stopAPI := context.AfterFunc(api.ctx, cancel)
	defer stopAPI()

	// Dispatch is the only operation input must wait for. Never hold run.op
	// across the connection, since DELETE must remain independent of input.
	reg := &api.workloads
	reg.mu.Lock()
	pending, dispatchDone := run.pending, run.dispatchDone
	reg.mu.Unlock()
	if pending {
		select {
		case <-ctx.Done():
			return
		case <-dispatchDone:
		}
	}
	reg.mu.Lock()
	value, host := run.value, run.host
	reg.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	if host == nil || value.Terminal() {
		workloadError(rw, r, http.StatusConflict, "Workload input is unavailable.")
		return
	}

	api.WebsocketWaitMutex.Lock()
	if api.ctx.Err() != nil {
		api.WebsocketWaitMutex.Unlock()
		return
	}
	api.WebsocketWaitGroup.Add(1)
	api.WebsocketWaitMutex.Unlock()
	defer api.WebsocketWaitGroup.Done()

	ctx = context.WithValue(ctx, agentIDKey{}, host.agentID)
	addr := netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(host.agentID), workspacesdk.AgentHTTPAPIServerPort).String()
	url := "http://" + addr + "/api/v0/workloads/workloads/" + value.ID.String() + "/input"
	client := &http.Client{Transport: api.workloadTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// Establish upstream before accepting downstream so admission failures can
	// still be reported as HTTP errors. Only the dedicated token is forwarded.
	timer := time.AfterFunc(10*time.Second, cancel)
	upstream, res, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: client,
		HTTPHeader: http.Header{"Authorization": {"Bearer " + host.token}},
	})
	timedOut := !timer.Stop()
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	if err != nil || timedOut {
		if upstream != nil {
			_ = upstream.CloseNow()
		}
		status := http.StatusBadGateway
		if res != nil && (res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusConflict) {
			status = res.StatusCode
		}
		workloadError(rw, r, status, "Workload input is unavailable.")
		return
	}
	defer upstream.CloseNow()
	stopUpstream := context.AfterFunc(ctx, func() { _ = upstream.CloseNow() })
	defer stopUpstream()
	upstream.SetReadLimit(32 << 10)

	downstream, err := websocket.Accept(rw, r, nil)
	if err != nil {
		return
	}
	defer downstream.CloseNow()
	stopDownstream := context.AfterFunc(ctx, func() { _ = downstream.CloseNow() })
	defer stopDownstream()
	downstream.SetReadLimit(32 << 10)

	type result struct {
		peer *websocket.Conn
		err  error
	}
	results := make(chan result, 2)
	copyMessages := func(dst, src *websocket.Conn) {
		for {
			typ, data, err := src.Read(ctx)
			if err != nil {
				results <- result{peer: dst, err: err}
				return
			}
			if err := dst.Write(ctx, typ, data); err != nil {
				results <- result{peer: src, err: err}
				return
			}
		}
	}
	go copyMessages(upstream, downstream)
	go copyMessages(downstream, upstream)
	finished := <-results
	if ctx.Err() == nil {
		code := websocket.CloseStatus(finished.err)
		if code < 0 || code == websocket.StatusAbnormalClosure || code == websocket.StatusTLSHandshake {
			code = websocket.StatusInternalError
		}
		// Preserve close status, but never disclose supervisor error text.
		_ = finished.peer.Close(code, "")
	}
	cancel()
	_ = upstream.CloseNow()
	_ = downstream.CloseNow()
	<-results
}
