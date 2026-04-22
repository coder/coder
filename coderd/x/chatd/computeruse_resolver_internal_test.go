package chatd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type advancingTestClock struct {
	now  time.Time
	step time.Duration
}

func newAdvancingTestClock(step time.Duration) *advancingTestClock {
	return &advancingTestClock{
		now:  time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		step: step,
	}
}

func (*advancingTestClock) NewTicker(time.Duration, ...string) *quartz.Ticker {
	panic("unexpected call to advancingTestClock.NewTicker")
}

func (*advancingTestClock) TickerFunc(
	context.Context,
	time.Duration,
	func() error,
	...string,
) quartz.Waiter {
	panic("unexpected call to advancingTestClock.TickerFunc")
}

func (*advancingTestClock) NewTimer(time.Duration, ...string) *quartz.Timer {
	panic("unexpected call to advancingTestClock.NewTimer")
}

func (*advancingTestClock) AfterFunc(
	time.Duration,
	func(),
	...string,
) *quartz.Timer {
	panic("unexpected call to advancingTestClock.AfterFunc")
}

func (c *advancingTestClock) Now(tags ...string) time.Time {
	now := c.now
	c.now = c.now.Add(c.step)
	return now
}

func (c *advancingTestClock) Since(t time.Time, tags ...string) time.Duration {
	return c.now.Sub(t)
}

func (c *advancingTestClock) Until(t time.Time, tags ...string) time.Duration {
	return t.Sub(c.now)
}

func TestComputerUseTargetFromConfig_RejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := computerUseTargetFromConfig(database.ChatModelConfig{
		Provider: "openai-compat",
		Model:    "computer-use-preview-2025-03-11",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `computer use provider "openai-compat" is not supported`)

	classified := chaterror.Classify(err)
	require.Equal(t, chaterror.KindConfig, classified.Kind)
	require.Equal(t, "openai-compat", classified.Provider)
}

func TestComputerUseTargetEligibilityError_ClassifiesInvalidOpenAIOptions(t *testing.T) {
	t.Parallel()

	target := computerUseTarget{
		provider: "openai",
		model:    "computer-use-preview-2025-03-11",
		config: database.ChatModelConfig{
			Provider: "openai",
			Model:    "computer-use-preview-2025-03-11",
			Enabled:  true,
			Options:  json.RawMessage(`{"provider_options":`),
		},
	}

	err := computerUseTargetEligibilityError(target, func(string) bool { return true })
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse computer use model call config")

	classified := chaterror.Classify(err)
	require.Equal(t, chaterror.KindConfig, classified.Kind)
	require.Equal(t, "openai", classified.Provider)
}

func TestResolveComputerUseTarget_FallsBackWhenEnabledProviderCheckerFails(
	t *testing.T,
) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)

	anthropicModel, ok := chattool.DefaultComputerUseModel("anthropic")
	require.True(t, ok)
	openAIModel, ok := chattool.DefaultComputerUseModel("openai")
	require.True(t, ok)

	enabledProvidersErr := xerrors.New("enabled providers unavailable")
	gomock.InOrder(
		db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return(
			[]database.ChatProvider{
				{
					Provider:                   "anthropic",
					CentralApiKeyEnabled:       true,
					AllowCentralApiKeyFallback: true,
				},
				{
					Provider:                   "openai",
					CentralApiKeyEnabled:       true,
					AllowCentralApiKeyFallback: true,
				},
			},
			nil,
		),
		db.EXPECT().GetEnabledChatProviders(gomock.Any()).Return(
			[]database.ChatProvider(nil), enabledProvidersErr,
		),
		db.EXPECT().GetEnabledChatModelConfigs(gomock.Any()).Return(
			[]database.ChatModelConfig{
				{
					ID:       uuid.New(),
					Provider: "openai",
					Model:    openAIModel,
					Enabled:  true,
				},
				{
					ID:       uuid.New(),
					Provider: "anthropic",
					Model:    anthropicModel,
					Enabled:  true,
				},
			},
			nil,
		),
	)

	server := &Server{
		db:     db,
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		configCache: newChatConfigCache(
			context.Background(),
			db,
			newAdvancingTestClock(chatConfigProvidersTTL+time.Second),
		),
		providerAPIKeys: chatprovider.ProviderAPIKeys{
			OpenAI:    "openai-deployment-key",
			Anthropic: "anthropic-deployment-key",
			ByProvider: map[string]string{
				"anthropic": "anthropic-deployment-key",
				"openai":    "openai-deployment-key",
			},
		},
	}

	target, err := resolveComputerUseTarget(ctx, server, database.Chat{
		OwnerID:           uuid.New(),
		LastModelConfigID: uuid.New(),
	})
	require.NoError(t, err)
	require.Equal(t, "anthropic", target.provider)
	require.Equal(t, anthropicModel, target.model)
}
