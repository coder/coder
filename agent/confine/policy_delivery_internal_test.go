package confine

import (
	"context"
	"io"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

type policyMonitorTestClient struct {
	policy   codersdk.AIEgressPolicy
	fetchErr error
	updates  chan codersdk.AIEgressPolicy
}

func (c *policyMonitorTestClient) AIEgressPolicy(context.Context) (codersdk.AIEgressPolicy, error) {
	return c.policy, c.fetchErr
}

func (c *policyMonitorTestClient) WatchAIEgressPolicy(ctx context.Context) (<-chan codersdk.AIEgressPolicy, io.Closer, error) {
	updates := c.updates
	if updates == nil {
		updates = make(chan codersdk.AIEgressPolicy)
	}
	out := make(chan codersdk.AIEgressPolicy)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case policy, ok := <-updates:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- policy:
				}
			}
		}
	}()
	return out, nil, nil
}

func TestPolicyMonitorFailClosed(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	monitor, err := NewPolicyMonitor(PolicyMonitorOptions{
		Client:    &policyMonitorTestClient{fetchErr: xerrors.New("policy unavailable")},
		Logger:    slog.Make(),
		AccessURL: accessURL,
	})
	require.NoError(t, err)
	_, err = monitor.Start(ctx)
	require.ErrorContains(t, err, "policy unavailable")
	require.True(t, monitor.Engine().Decide("coder.example.com", 443).Allowed)
	require.False(t, monitor.Engine().Decide("example.com", 443).Allowed)
}

func TestPolicyMonitorLiveUpdate(t *testing.T) {
	t.Parallel()

	accessURL, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	updates := make(chan codersdk.AIEgressPolicy, 1)
	applied := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	monitor, err := NewPolicyMonitor(PolicyMonitorOptions{
		Client:    &policyMonitorTestClient{updates: updates},
		Logger:    slog.Make(),
		AccessURL: accessURL,
		AfterUpdate: func(codersdk.AIEgressPolicy) {
			applied <- struct{}{}
		},
	})
	require.NoError(t, err)
	_, err = monitor.Start(ctx)
	require.NoError(t, err)
	updates <- codersdk.AIEgressPolicy{
		Revision: 7,
		Rules: []codersdk.AIEgressRule{{
			Host:  "example.com",
			Ports: []int{443},
		}},
	}
	select {
	case <-ctx.Done():
		t.Fatal("context canceled before policy update")
	case <-applied:
	}
	require.Equal(t, Decision{Allowed: true, Revision: 7}, monitor.Engine().Decide("example.com", 443))
}
