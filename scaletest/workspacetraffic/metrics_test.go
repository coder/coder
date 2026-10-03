package workspacetraffic_test

import (
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/scaletest/workspacetraffic"
)

func TestConnMetrics_Concurrent(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := workspacetraffic.NewMetrics(reg, "username", "workspace_name", "agent_name")
	cm := m.ReadMetrics("username", "workspace_name", "agent_name")

	const (
		writers       = 8
		readers       = 8
		opsPerWriter  = 1000
		bytesPerWrite = 1
	)

	var wg sync.WaitGroup
	wg.Add(writers + readers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWriter; j++ {
				cm.AddTotal(float64(bytesPerWrite))
			}
		}()
	}
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWriter; j++ {
				_ = cm.GetTotalBytes()
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int64(writers*opsPerWriter*bytesPerWrite), cm.GetTotalBytes())
}
