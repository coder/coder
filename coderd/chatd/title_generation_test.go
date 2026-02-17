package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"go.jetify.com/ai/api"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

type titleTestModel struct {
	generateFn func(context.Context, []api.Message, api.CallOptions) (*api.Response, error)
}

func (*titleTestModel) ProviderName() string {
	return "fake"
}

func (*titleTestModel) ModelID() string {
	return "fake"
}

func (*titleTestModel) SupportedUrls() []api.SupportedURL {
	return nil
}

func (m *titleTestModel) Generate(
	ctx context.Context,
	prompt []api.Message,
	opts api.CallOptions,
) (*api.Response, error) {
	if m.generateFn != nil {
		return m.generateFn(ctx, prompt, opts)
	}
	return &api.Response{}, nil
}

func (*titleTestModel) Stream(
	_ context.Context,
	_ []api.Message,
	_ api.CallOptions,
) (*api.StreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func TestAnyAvailableModel(t *testing.T) {
	t.Parallel()

	t.Run("OpenAIOnly", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(ProviderAPIKeys{OpenAI: "openai-key"})
		require.NoError(t, err)
		require.Equal(t, "openai", model.ProviderName())
		require.Equal(t, "gpt-4o-mini", model.ModelID())
	})

	t.Run("AnthropicOnly", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(ProviderAPIKeys{Anthropic: "anthropic-key"})
		require.NoError(t, err)
		require.Equal(t, "anthropic", model.ProviderName())
		require.Equal(t, "claude-haiku-4-5", model.ModelID())
	})

	t.Run("None", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(ProviderAPIKeys{})
		require.Nil(t, model)
		require.EqualError(t, err, "no AI provider API keys are configured")
	})
}

func TestGenerateChatTitle(t *testing.T) {
	t.Parallel()

	t.Run("SuccessNormalizesOutput", func(t *testing.T) {
		t.Parallel()

		var capturedPrompt []api.Message
		model := &titleTestModel{
			generateFn: func(_ context.Context, prompt []api.Message, _ api.CallOptions) (*api.Response, error) {
				capturedPrompt = append([]api.Message(nil), prompt...)
				return &api.Response{
					Content: []api.ContentBlock{
						&api.TextBlock{Text: `  " Debugging   Flaky   Go Tests "  `},
					},
				}, nil
			},
		}

		p := &Processor{
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleGeneration: TitleGenerationConfig{
				Prompt: "custom title prompt",
			},
			titleModelLookup: func(ProviderAPIKeys) (api.LanguageModel, error) {
				return model, nil
			},
		}

		title, err := p.generateChatTitle(context.Background(), "How can I debug this flaky Go test?")
		require.NoError(t, err)
		require.Equal(t, "Debugging Flaky Go Tests", title)
		require.Len(t, capturedPrompt, 2)

		system, ok := capturedPrompt[0].(*api.SystemMessage)
		require.True(t, ok)
		require.Equal(t, "custom title prompt", system.Content)

		user, ok := capturedPrompt[1].(*api.UserMessage)
		require.True(t, ok)
		require.Equal(t, "How can I debug this flaky Go test?", contentBlocksToText(user.Content))
	})

	t.Run("EmptyOutput", func(t *testing.T) {
		t.Parallel()

		model := &titleTestModel{
			generateFn: func(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
				return &api.Response{
					Content: []api.ContentBlock{
						&api.TextBlock{Text: "   "},
					},
				}, nil
			},
		}

		p := &Processor{
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (api.LanguageModel, error) {
				return model, nil
			},
		}

		title, err := p.generateChatTitle(context.Background(), "hello")
		require.EqualError(t, err, "generated title was empty")
		require.Empty(t, title)
	})

	t.Run("GenerateError", func(t *testing.T) {
		t.Parallel()

		model := &titleTestModel{
			generateFn: func(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
				return nil, xerrors.New("model failed")
			},
		}

		p := &Processor{
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (api.LanguageModel, error) {
				return model, nil
			},
		}

		title, err := p.generateChatTitle(context.Background(), "hello")
		require.EqualError(t, err, "generate title text: model failed")
		require.Empty(t, title)
	})
}

func TestMaybeGenerateChatTitle(t *testing.T) {
	t.Parallel()

	messageText := "How do I debug flaky Go tests?"
	chat := database.Chat{
		ID:    uuid.New(),
		Title: fallbackChatTitle(messageText),
	}
	messages := []database.ChatMessage{mustUserChatMessage(t, messageText)}

	t.Run("UpdatesTitle", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		db.EXPECT().UpdateChatByID(gomock.Any(), database.UpdateChatByIDParams{
			ID:    chat.ID,
			Title: "Debugging Flaky Go Tests",
		}).Return(database.Chat{}, nil)

		p := &Processor{
			db:           db,
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (api.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
						return &api.Response{
							Content: []api.ContentBlock{
								&api.TextBlock{Text: "Debugging Flaky Go Tests"},
							},
						}, nil
					},
				}, nil
			},
		}

		p.maybeGenerateChatTitle(context.Background(), chat, messages, testutil.Logger(t))
	})

	t.Run("SkipsUpdateOnEmptyTitle", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		p := &Processor{
			db:           db,
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (api.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
						return &api.Response{
							Content: []api.ContentBlock{
								&api.TextBlock{Text: "   "},
							},
						}, nil
					},
				}, nil
			},
		}

		p.maybeGenerateChatTitle(context.Background(), chat, messages, testutil.Logger(t))
	})

	t.Run("SkipsUpdateOnGenerationError", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		p := &Processor{
			db:           db,
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (api.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
						return nil, xerrors.New("title model failed")
					},
				}, nil
			},
		}

		p.maybeGenerateChatTitle(context.Background(), chat, messages, testutil.Logger(t))
	})

	t.Run("SkipsUpdateWhenTitleUnchanged", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)

		p := &Processor{
			db:           db,
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (api.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
						return &api.Response{
							Content: []api.ContentBlock{
								&api.TextBlock{Text: chat.Title},
							},
						}, nil
					},
				}, nil
			},
		}

		p.maybeGenerateChatTitle(context.Background(), chat, messages, testutil.Logger(t))
	})
}

func mustUserChatMessage(t *testing.T, text string) database.ChatMessage {
	t.Helper()

	raw, err := json.Marshal(api.ContentFromText(text))
	require.NoError(t, err)

	return database.ChatMessage{
		Role: string(api.MessageRoleUser),
		Content: pqtype.NullRawMessage{
			RawMessage: raw,
			Valid:      true,
		},
	}
}
