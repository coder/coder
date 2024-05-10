package agentapi

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/appearance"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestGetNotificationBanners(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		cfg := []codersdk.BannerConfig{{
			Enabled:         true,
			Message:         "The beep-bop will be boop-beeped on Saturday at 12AM PST.",
			BackgroundColor: "#00FF00",
		}}

		var ff appearance.Fetcher = fakeFetcher{cfg: codersdk.AppearanceConfig{NotificationBanners: cfg}}
		ptr := atomic.Pointer[appearance.Fetcher]{}
		ptr.Store(&ff)

		api := &NotificationBannerAPI{appearanceFetcher: &ptr}
		resp, err := api.GetNotificationBanners(context.Background(), &agentproto.GetNotificationBannersRequest{})
		require.NoError(t, err)
		require.Len(t, resp.NotificationBanners, 1)
		require.Equal(t, cfg[0], agentsdk.BannerConfigFromProto(resp.NotificationBanners[0]))
	})

	t.Run("FetchError", func(t *testing.T) {
		t.Parallel()

		expectedErr := xerrors.New("badness")
		var ff appearance.Fetcher = fakeFetcher{err: expectedErr}
		ptr := atomic.Pointer[appearance.Fetcher]{}
		ptr.Store(&ff)

		api := &NotificationBannerAPI{appearanceFetcher: &ptr}
		resp, err := api.GetNotificationBanners(context.Background(), &agentproto.GetNotificationBannersRequest{})
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
