package metricscache

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/coder/coder/v2/codersdk"
)

var benchmarkApps map[string]codersdk.SessionCountApp

type legacySessionCountDeploymentStats struct {
	VSCode          int64 `json:"vscode"`
	SSH             int64 `json:"ssh"`
	JetBrains       int64 `json:"jetbrains"`
	ReconnectingPTY int64 `json:"reconnecting_pty"`
}

func benchmarkDeploymentStats(names int) codersdk.DeploymentStats {
	counts := make(map[string]int64, names)
	apps := make(map[string]codersdk.SessionCountApp)
	known := []string{"vscode", "cursor", "zed", "jetbrains", "reconnecting_pty"}
	for i := range names {
		name := fmt.Sprintf("future_ide_%d", i)
		if i < len(known) {
			name = known[i]
		}
		counts[name] = int64(i + 1)
		if app, ok := codersdk.SessionCountAppMetadata(name); ok {
			apps[name] = app
		}
	}
	return codersdk.DeploymentStats{SessionCount: codersdk.SessionCountDeploymentStats{
		SessionCounts: counts, Apps: apps, VSCode: 3, SSH: 3, JetBrains: 5, ReconnectingPTY: 6,
	}}
}

func BenchmarkDeploymentStatsAccessor(b *testing.B) {
	for _, names := range []int{4, 16, 65} {
		b.Run(fmt.Sprintf("names=%d", names), func(b *testing.B) {
			cache := &Cache{}
			stats := benchmarkDeploymentStats(names)
			cache.deploymentStatsResponse.Store(&stats)
			b.ReportAllocs()
			for b.Loop() {
				if _, ok := cache.DeploymentStats(); !ok {
					b.Fatal("deployment stats unavailable")
				}
			}
		})
	}
}

func BenchmarkSessionCountProjection(b *testing.B) {
	for _, names := range []int{4, 16, 65} {
		b.Run(fmt.Sprintf("names=%d", names), func(b *testing.B) {
			counts := benchmarkDeploymentStats(names).SessionCount.SessionCounts
			b.ReportAllocs()
			for b.Loop() {
				apps := make(map[string]codersdk.SessionCountApp)
				for name := range counts {
					if app, ok := codersdk.SessionCountAppMetadata(name); ok {
						apps[name] = app
					}
				}
				benchmarkApps = apps
			}
		})
	}
}

func BenchmarkDeploymentStatsPayload(b *testing.B) {
	stats := benchmarkDeploymentStats(100 * 65)
	cache := &Cache{}
	cache.deploymentStatsResponse.Store(&stats)
	legacy := legacySessionCountDeploymentStats{VSCode: 3, SSH: 3, JetBrains: 5, ReconnectingPTY: 6}
	for name, value := range map[string]any{"per_app": cache, "legacy_only": legacy} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var payload []byte
				var err error
				if cache, ok := value.(*Cache); ok {
					got, _ := cache.DeploymentStats()
					payload, err = json.Marshal(got.SessionCount)
				} else {
					payload, err = json.Marshal(value)
				}
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(payload)), "payload-bytes")
			}
		})
	}
}
