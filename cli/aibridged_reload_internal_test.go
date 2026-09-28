//go:build !slim

package cli

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
)

func TestProviderRPCReloaderPublication(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		name := "Success"
		if fail {
			name = "Failure"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := aibridgedmock.NewMockDRPCClient(gomock.NewController(t))
			client.EXPECT().GetAIProviders(gomock.Any(), gomock.Any()).Return(&proto.GetAIProvidersResponse{}, nil)
			metrics := aibridged.NewMetrics(prometheus.NewRegistry())
			publishErr := xerrors.New("publish failed")
			called := false
			reloader := NewProviderRPCReloader(func(ctx context.Context, providers []aibridge.Provider) error {
				called = true
				require.Equal(t, t.Context(), ctx)
				require.Empty(t, providers)
				if fail {
					return publishErr
				}
				return nil
			}, func(context.Context) (aibridged.DRPCClient, error) {
				return client, nil
			}, codersdk.AIBridgeConfig{}, slogtest.Make(t, nil), nil, metrics)

			err := reloader.Reload(t.Context())
			require.True(t, called)
			require.Positive(t, promtest.ToFloat64(metrics.ProvidersLastReloadTimestampSeconds))
			if fail {
				require.ErrorIs(t, err, publishErr)
				require.Zero(t, promtest.ToFloat64(metrics.ProvidersLastReloadSuccessTimestampSeconds))
			} else {
				require.NoError(t, err)
				require.Positive(t, promtest.ToFloat64(metrics.ProvidersLastReloadSuccessTimestampSeconds))
			}
		})
	}
}
