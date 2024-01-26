package agentapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/agentapi"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
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
		cfgJSON, err := json.Marshal(cfg)
		require.NoError(t, err)

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetServiceBanner(gomock.Any()).Return(string(cfgJSON), nil)

		api := &agentapi.ServiceBannerAPI{
			Database: dbM,
		}

		resp, err := api.GetServiceBanner(context.Background(), &agentproto.GetServiceBannerRequest{})
		require.NoError(t, err)

		require.Equal(t, &agentproto.ServiceBanner{
			Enabled:         cfg.Enabled,
			Message:         cfg.Message,
			BackgroundColor: cfg.BackgroundColor,
		}, resp)
	})

	t.Run("None", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetServiceBanner(gomock.Any()).Return("", sql.ErrNoRows)

		api := &agentapi.ServiceBannerAPI{
			Database: dbM,
		}

		resp, err := api.GetServiceBanner(context.Background(), &agentproto.GetServiceBannerRequest{})
		require.NoError(t, err)

		require.Equal(t, &agentproto.ServiceBanner{
			Enabled:         false,
			Message:         "",
			BackgroundColor: "",
		}, resp)
	})

	t.Run("BadJSON", func(t *testing.T) {
		t.Parallel()

		dbM := dbmock.NewMockStore(gomock.NewController(t))
		dbM.EXPECT().GetServiceBanner(gomock.Any()).Return("hi", nil)

		api := &agentapi.ServiceBannerAPI{
			Database: dbM,
		}

		resp, err := api.GetServiceBanner(context.Background(), &agentproto.GetServiceBannerRequest{})
		require.Error(t, err)
		require.ErrorContains(t, err, "unmarshal json")
		require.Nil(t, resp)
	})
}
