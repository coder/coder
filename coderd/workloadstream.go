package coderd

import (
	"bufio"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
)

const workloadStreamFrameLimit = 4 << 20

// followWorkload observes one supervisor request without holding run.op. A
// disconnected observer must neither cancel the workload nor block DELETE.
func (api *API) followWorkload(rw http.ResponseWriter, r *http.Request, run *workloadRun, offset int64) {
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("X-Accel-Buffering", "no")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(api.ctx, cancel)
	defer stop()
	controller := http.NewResponseController(rw)
	// API shutdown must also unblock writes to a slow downstream socket.
	writeStopped := make(chan struct{})
	stopWrite := context.AfterFunc(ctx, func() {
		defer close(writeStopped)
		_ = controller.SetWriteDeadline(time.Now())
	})
	defer func() {
		if !stopWrite() {
			<-writeStopped
		}
	}()

	reg := &api.workloads
	reg.mu.Lock()
	pending, dispatchDone := run.pending, run.dispatchDone
	reg.mu.Unlock()
	if pending {
		// Placement may not exist yet. Wait for dispatch, but allow this
		// observer to disconnect independently of the dispatch request.
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
	if host == nil {
		// Local admission failures have no supervisor or retained output.
		rw.Header().Set("Content-Type", "application/x-ndjson")
		_ = writeWorkloadFrame(rw, controller, codersdk.WorkloadOutput{Workload: value, NextOffset: offset})
		return
	}

	ctx = context.WithValue(ctx, agentIDKey{}, host.agentID)
	addr := netip.AddrPortFrom(tailnet.TailscaleServicePrefix.AddrFromUUID(host.agentID), workspacesdk.AgentHTTPAPIServerPort).String()
	path := "/api/v0/workloads/workloads/" + value.ID.String() + "?offset=" + strconv.FormatInt(offset, 10) + "&follow=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		workloadError(rw, r, http.StatusBadGateway, "Workload output stream is unavailable.")
		return
	}
	req.Header.Set("Authorization", "Bearer "+host.token)
	req.Header.Set("Accept", "application/x-ndjson")
	// The pooled transport bounds dialing and response headers. There is no
	// whole-request timeout on a stream, and redirects never receive tokens.
	client := &http.Client{Transport: api.workloadTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		workloadError(rw, r, http.StatusBadGateway, "Workload output stream is unavailable.")
		return
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusBadRequest {
		workloadError(rw, r, http.StatusBadRequest, "Offset is outside the retained output range.")
		return
	}
	mediaType, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if res.StatusCode != http.StatusOK || err != nil || mediaType != "application/x-ndjson" {
		workloadError(rw, r, http.StatusBadGateway, "Workload output stream is unavailable.")
		return
	}
	rw.Header().Set("Content-Type", "application/x-ndjson")
	rw.WriteHeader(http.StatusOK)
	if controller.Flush() != nil {
		return
	}

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 64<<10), workloadStreamFrameLimit+1)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			// Forward supervisor heartbeats immediately, even without output.
			if _, err := rw.Write([]byte{'\n'}); err != nil || controller.Flush() != nil {
				return
			}
			continue
		}
		var output codersdk.WorkloadOutput
		if len(line) > workloadStreamFrameLimit || json.Unmarshal(line, &output) != nil ||
			output.NextOffset < offset || output.NextOffset-offset < int64(len(output.Events)) ||
			(!output.Truncated && output.NextOffset-offset != int64(len(output.Events))) {
			return
		}
		reg.mu.Lock()
		valid := mergeWorkloadObservation(&run.value, output.Workload)
		output.Workload = run.value
		reg.mu.Unlock()
		if !valid || !writeWorkloadFrame(rw, controller, output) {
			return
		}
		offset = output.NextOffset
		if output.Workload.Terminal() && len(output.Events) == 0 {
			return
		}
	}
	// A malformed frame, transport error, or early EOF closes without a
	// terminal empty batch. Callers can distinguish this from drained output
	// and resume; no supervisor error text or partial frame is forwarded.
}

func writeWorkloadFrame(rw http.ResponseWriter, controller *http.ResponseController, output codersdk.WorkloadOutput) bool {
	data, err := json.Marshal(output)
	if err != nil || len(data) > workloadStreamFrameLimit {
		return false
	}
	if _, err := rw.Write(append(data, '\n')); err != nil {
		return false
	}
	return controller.Flush() == nil
}
