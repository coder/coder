package agentapi

import (
	"context"
	"sync/atomic"
	"testing"

	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/codersdk"
	"github.com/stretchr/testify/require"
)

func TestGetServiceBanner(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		cfg := codersdk.ServiceBannerConfig{
			Enabled:         true,
			Message:         "hello world",
			BackgroundColor: "#000000",
		}

		var ff appearance.Fetcher = fakeFetcher{cfg: codersdk.AppearanceConfig{ServiceBanner: cfg}}
		ptr := atomic.Pointer[appearance.Fetcher]{}
		ptr.Store(&ff)

		api := &ServiceBannerAPI{
			appearanceFetcher: &ptr,
		}

		resp, err := api.GetServiceBanner(context.Background(), &agentproto.GetServiceBannerRequest{})
		require.NoError(t, err)

		require.Equal(t, &agentproto.ServiceBanner{
			Enabled:         cfg.Enabled,
			Message:         cfg.Message,
			BackgroundColor: cfg.BackgroundColor,
		}, resp)
	})

	t.Run("FetchError", func(t *testing.T) {
		t.Parallel()

		expectedErr := xerrors.New("badness")
		var ff appearance.Fetcher = fakeFetcher{err: expectedErr}
		ptr := atomic.Pointer[appearance.Fetcher]{}
		ptr.Store(&ff)

		api := &ServiceBannerAPI{
			appearanceFetcher: &ptr,
		}

		resp, err := api.GetServiceBanner(context.Background(), &agentproto.GetServiceBannerRequest{})
		require.Error(t, err)
		require.ErrorIs(t, err, expectedErr)
		require.Nil(t, resp)
	})
}

type fakeFetcher struct {
	cfg codersdk.AppearanceConfig
	err error
}

func (f fakeFetcher) Fetch(context.Context) (codersdk.AppearanceConfig, error) {
	return f.cfg, f.err
}
