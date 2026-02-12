package coderd

import (
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk"
)

// detectPatterns analyzes workspaces and summary to identify common
// connection patterns such as device sleep, agent crashes, and clean usage.
func detectPatterns(workspaces []codersdk.DiagnosticWorkspace, summary codersdk.DiagnosticSummary) []codersdk.DiagnosticPattern {
	var patterns []codersdk.DiagnosticPattern

	if p, ok := detectWorkspaceAutostart(workspaces, summary); ok {
		patterns = append(patterns, p)
	}
	if p, ok := detectDeviceSleep(workspaces, summary); ok {
		patterns = append(patterns, p)
	}
	if p, ok := detectAgentCrash(workspaces, summary); ok {
		patterns = append(patterns, p)
	}
	if p, ok := detectCleanUsage(summary); ok {
		patterns = append(patterns, p)
	}

	return patterns
}

// detectWorkspaceAutostart fires when more than half of all sessions ended
// due to a workspace being stopped (e.g. auto-stop schedule).
func detectWorkspaceAutostart(_ []codersdk.DiagnosticWorkspace, summary codersdk.DiagnosticSummary) (codersdk.DiagnosticPattern, bool) {
	if summary.TotalSessions == 0 || summary.ByStatus.WorkspaceStopped <= summary.TotalSessions/2 {
		return codersdk.DiagnosticPattern{}, false
	}
	return codersdk.DiagnosticPattern{
		ID:               uuid.New(),
		Type:             codersdk.DiagnosticPatternWorkspaceAutostart,
		Severity:         codersdk.ConnectionDiagnosticSeverityInfo,
		AffectedSessions: summary.ByStatus.WorkspaceStopped,
		TotalSessions:    summary.TotalSessions,
		Title:            "Workspace auto-stop is ending sessions",
		Description: fmt.Sprintf(
			"%d of %d sessions ended because the workspace was stopped, likely by an auto-stop schedule.",
			summary.ByStatus.WorkspaceStopped, summary.TotalSessions,
		),
		Commonalities: codersdk.DiagnosticPatternCommonality{
			ConnectionTypes:    []string{},
			ClientDescriptions: []string{},
			DisconnectReasons:  []string{"workspace stopped"},
		},
		Recommendation: "Review workspace auto-stop schedules or extend TTLs if sessions are being interrupted.",
	}, true
}

// detectDeviceSleep fires when two or more sessions from the same client
// lost control and lasted less than 15 minutes.
func detectDeviceSleep(workspaces []codersdk.DiagnosticWorkspace, summary codersdk.DiagnosticSummary) (codersdk.DiagnosticPattern, bool) {
	const maxDurationSecs = 900.0 // 15 minutes

	// Collect short control-lost sessions grouped by client description.
	type clientHit struct {
		count   int
		minDur  float64
		maxDur  float64
		reasons map[string]struct{}
	}
	hits := make(map[string]*clientHit)

	for _, ws := range workspaces {
		for _, sess := range ws.Sessions {
			if sess.Status != codersdk.ConnectionStatusControlLost {
				continue
			}
			dur := 0.0
			if sess.DurationSeconds != nil {
				dur = *sess.DurationSeconds
			}
			if dur <= 0 || dur > maxDurationSecs {
				continue
			}
			key := sess.ShortDescription
			if key == "" {
				key = sess.ClientHostname
			}
			if key == "" {
				continue
			}
			h, ok := hits[key]
			if !ok {
				h = &clientHit{minDur: dur, maxDur: dur, reasons: make(map[string]struct{})}
				hits[key] = h
			}
			h.count++
			h.minDur = math.Min(h.minDur, dur)
			h.maxDur = math.Max(h.maxDur, dur)
			if sess.DisconnectReason != "" {
				h.reasons[sess.DisconnectReason] = struct{}{}
			}
		}
	}

	// Find the client with the most hits (>= 2).
	var (
		bestKey   string
		bestHit   *clientHit
		totalHits int
	)
	for k, h := range hits {
		if h.count >= 2 {
			totalHits += h.count
			if bestHit == nil || h.count > bestHit.count {
				bestKey = k
				bestHit = h
			}
		}
	}
	if bestHit == nil {
		return codersdk.DiagnosticPattern{}, false
	}

	reasons := make([]string, 0, len(bestHit.reasons))
	for r := range bestHit.reasons {
		reasons = append(reasons, r)
	}

	return codersdk.DiagnosticPattern{
		ID:               uuid.New(),
		Type:             codersdk.DiagnosticPatternDeviceSleep,
		Severity:         codersdk.ConnectionDiagnosticSeverityWarning,
		AffectedSessions: totalHits,
		TotalSessions:    summary.TotalSessions,
		Title:            "Device sleep or network interruption",
		Description: fmt.Sprintf(
			"%d short sessions from %q lost control, suggesting the client device went to sleep or lost network.",
			totalHits, bestKey,
		),
		Commonalities: codersdk.DiagnosticPatternCommonality{
			ConnectionTypes:    []string{},
			ClientDescriptions: []string{bestKey},
			DurationRange: &codersdk.DiagnosticDurationRange{
				MinSeconds: bestHit.minDur,
				MaxSeconds: bestHit.maxDur,
			},
			DisconnectReasons: reasons,
		},
		Recommendation: "Check power/sleep settings on the client device, or adjust keep-alive intervals.",
	}, true
}

// detectAgentCrash fires when a single workspace has two or more sessions
// with "agent timeout" while other workspaces have none.
func detectAgentCrash(workspaces []codersdk.DiagnosticWorkspace, summary codersdk.DiagnosticSummary) (codersdk.DiagnosticPattern, bool) {
	type wsHit struct {
		name  string
		count int
	}
	var hitsPerWS []wsHit
	for _, ws := range workspaces {
		count := 0
		for _, sess := range ws.Sessions {
			if strings.Contains(strings.ToLower(sess.DisconnectReason), "agent timeout") {
				count++
			}
		}
		hitsPerWS = append(hitsPerWS, wsHit{name: ws.Name, count: count})
	}

	// Find a workspace with >= 2 agent timeout sessions.
	var affected wsHit
	othersClean := true
	for _, h := range hitsPerWS {
		if h.count >= 2 {
			if affected.count == 0 || h.count > affected.count {
				affected = h
			}
		} else if h.count > 0 {
			othersClean = false
		}
	}
	if affected.count == 0 || !othersClean {
		return codersdk.DiagnosticPattern{}, false
	}

	return codersdk.DiagnosticPattern{
		ID:               uuid.New(),
		Type:             codersdk.DiagnosticPatternAgentCrash,
		Severity:         codersdk.ConnectionDiagnosticSeverityCritical,
		AffectedSessions: affected.count,
		TotalSessions:    summary.TotalSessions,
		Title:            "Possible agent crash in " + affected.name,
		Description: fmt.Sprintf(
			"Workspace %q has %d sessions that ended with agent timeout while other workspaces are unaffected.",
			affected.name, affected.count,
		),
		Commonalities: codersdk.DiagnosticPatternCommonality{
			ConnectionTypes:    []string{},
			ClientDescriptions: []string{},
			DisconnectReasons:  []string{"agent timeout"},
		},
		Recommendation: "Check agent logs in the affected workspace for crashes or resource exhaustion.",
	}, true
}

// detectCleanUsage fires when all sessions ended cleanly.
func detectCleanUsage(summary codersdk.DiagnosticSummary) (codersdk.DiagnosticPattern, bool) {
	if summary.ByStatus.Lost != 0 || summary.ByStatus.WorkspaceStopped != 0 || summary.ByStatus.WorkspaceDeleted != 0 {
		return codersdk.DiagnosticPattern{}, false
	}
	if summary.TotalSessions == 0 {
		return codersdk.DiagnosticPattern{}, false
	}
	return codersdk.DiagnosticPattern{
		ID:               uuid.New(),
		Type:             codersdk.DiagnosticPatternCleanUsage,
		Severity:         codersdk.ConnectionDiagnosticSeverityInfo,
		AffectedSessions: summary.TotalSessions,
		TotalSessions:    summary.TotalSessions,
		Title:            "All sessions are clean",
		Description:      "No connection issues detected in the selected time window.",
		Commonalities: codersdk.DiagnosticPatternCommonality{
			ConnectionTypes:    []string{},
			ClientDescriptions: []string{},
			DisconnectReasons:  []string{},
		},
		Recommendation: "No action needed.",
	}, true
}
