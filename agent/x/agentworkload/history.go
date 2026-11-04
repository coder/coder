package agentworkload

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// Command text, environment values, and generation metadata may require smaller
// pages to fit the proxy limit after JSON escaping.
const (
	maxHistoryPage  = 10
	maxHistoryBytes = 4 << 20
)

func (s *Supervisor) statsHTTP(w http.ResponseWriter) {
	stats, err := s.stats()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Supervisor) stats() (codersdk.WorkloadHostStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed || s.closed {
		return codersdk.WorkloadHostStats{}, xerrors.New("workload supervisor is detached or unavailable")
	}
	stats := codersdk.WorkloadHostStats{Total: int64(len(s.state.Runs))}
	for _, run := range s.state.Runs {
		switch run.Workload.State {
		case "requested", "reserved", "starting", "pending":
			stats.Pending++
		case "running":
			stats.Running++
		case "succeeded":
			stats.Succeeded++
		case "failed":
			stats.Failed++
		case "canceled":
			stats.Canceled++
		case "unknown":
			stats.Unknown++
		}
	}
	return stats, nil
}

func (s *Supervisor) historyHTTP(w http.ResponseWriter, r *http.Request) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid history query")
		return
	}
	offset, limit := 0, maxHistoryPage
	if raw, ok := query["offset"]; ok {
		if len(raw) != 1 {
			writeError(w, http.StatusBadRequest, "offset must be a single nonnegative integer")
			return
		}
		offset, err = strconv.Atoi(raw[0])
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "offset must be a nonnegative integer")
			return
		}
	}
	if raw, ok := query["limit"]; ok {
		if len(raw) != 1 {
			writeError(w, http.StatusBadRequest, "limit must be a single integer between 1 and 10")
			return
		}
		limit, err = strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > maxHistoryPage {
			writeError(w, http.StatusBadRequest, "limit must be an integer between 1 and 10")
			return
		}
	}
	history, err := s.history(offset, limit)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (s *Supervisor) history(offset, limit int) (codersdk.WorkloadHostHistory, error) {
	s.mu.Lock()
	if s.failed || s.closed {
		s.mu.Unlock()
		return codersdk.WorkloadHostHistory{}, xerrors.New("workload supervisor is detached or unavailable")
	}
	items := make([]codersdk.WorkloadHistoryEntry, 0, len(s.state.Runs))
	for _, run := range s.state.Runs {
		items = append(items, codersdk.WorkloadHistoryEntry{Request: run.Request, Workload: run.Workload})
	}
	s.mu.Unlock()

	// Snapshot values before sorting or encoding so lifecycle transitions are
	// not blocked on the response. Generation metadata is immutable.
	slices.SortFunc(items, func(a, b codersdk.WorkloadHistoryEntry) int {
		if order := a.Workload.CreatedAt.Compare(b.Workload.CreatedAt); order != 0 {
			return order
		}
		return bytes.Compare(a.Workload.ID[:], b.Workload.ID[:])
	})
	offset = min(offset, len(items))
	// Reserve space for the envelope, including the total, offset and newline.
	// Both numbers are bounded by maxHistory, so 128 bytes is sufficient.
	end, size := offset, 128
	for end < len(items) && end-offset < limit {
		data, err := json.Marshal(items[end])
		if err != nil {
			return codersdk.WorkloadHostHistory{}, xerrors.New("encode workload history entry")
		}
		if size+len(data)+1 > maxHistoryBytes {
			if end == offset {
				return codersdk.WorkloadHostHistory{}, xerrors.New("workload history entry exceeds response limit")
			}
			break
		}
		size += len(data) + 1 // Include the separator between entries.
		end++
	}
	return codersdk.WorkloadHostHistory{
		Items: items[offset:end], Total: int64(len(items)), NextOffset: end,
	}, nil
}
