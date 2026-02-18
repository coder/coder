package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

type titleTestModel struct {
	generateFn func(context.Context, fantasy.Call) (*fantasy.Response, error)
}

func (*titleTestModel) Provider() string {
	return "fake"
}

func (*titleTestModel) Model() string {
	return "fake"
}

func (m *titleTestModel) Generate(
	ctx context.Context,
	call fantasy.Call,
) (*fantasy.Response, error) {
	if m.generateFn != nil {
		return m.generateFn(ctx, call)
	}
	return &fantasy.Response{}, nil
}

func (*titleTestModel) Stream(
	_ context.Context,
	_ fantasy.Call,
) (fantasy.StreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*titleTestModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*titleTestModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func TestAnyAvailableModel(t *testing.T) {
	t.Parallel()

	t.Run("OpenAIOnly", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(ProviderAPIKeys{OpenAI: "openai-key"})
		require.NoError(t, err)
		require.Equal(t, "openai", model.Provider())
		require.Equal(t, "gpt-4o-mini", model.Model())
	})

	t.Run("AnthropicOnly", func(t *testing.T) {
		t.Parallel()

		model, err := anyAvailableModel(ProviderAPIKeys{Anthropic: "anthropic-key"})
		require.NoError(t, err)
		require.Equal(t, "anthropic", model.Provider())
		require.Equal(t, "claude-haiku-4-5", model.Model())
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

		var capturedPrompt []fantasy.Message
		var capturedToolChoice *fantasy.ToolChoice
		model := &titleTestModel{
			generateFn: func(_ context.Context, call fantasy.Call) (*fantasy.Response, error) {
				capturedPrompt = append([]fantasy.Message(nil), call.Prompt...)
				capturedToolChoice = call.ToolChoice
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: `  " Debugging   Flaky   Go Tests "  `},
					},
				}, nil
			},
		}

		p := &Processor{
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleGeneration: TitleGenerationConfig{
				Prompt: "custom title prompt",
			},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return model, nil
			},
		}

		title, err := p.generateChatTitle(context.Background(), "How can I debug this flaky Go test?")
		require.NoError(t, err)
		require.Equal(t, "Debugging Flaky Go Tests", title)
		require.Len(t, capturedPrompt, 2)
		require.NotNil(t, capturedToolChoice)
		require.Equal(t, fantasy.ToolChoiceNone, *capturedToolChoice)

		require.Equal(t, fantasy.MessageRoleSystem, capturedPrompt[0].Role)
		require.Len(t, capturedPrompt[0].Content, 1)
		systemPart, ok := fantasy.AsMessagePart[fantasy.TextPart](capturedPrompt[0].Content[0])
		require.True(t, ok)
		require.Equal(t, "custom title prompt", systemPart.Text)

		require.Equal(t, fantasy.MessageRoleUser, capturedPrompt[1].Role)
		require.Len(t, capturedPrompt[1].Content, 1)
		userPart, ok := fantasy.AsMessagePart[fantasy.TextPart](capturedPrompt[1].Content[0])
		require.True(t, ok)
		require.Equal(t, "How can I debug this flaky Go test?", userPart.Text)
	})

	t.Run("EmptyOutput", func(t *testing.T) {
		t.Parallel()

		model := &titleTestModel{
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: "   "},
					},
				}, nil
			},
		}

		p := &Processor{
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
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
			generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				return nil, xerrors.New("model failed")
			},
		}

		p := &Processor{
			providerKeys: ProviderAPIKeys{OpenAI: "openai-key"},
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
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
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
						return &fantasy.Response{
							Content: []fantasy.Content{
								fantasy.TextContent{Text: "Debugging Flaky Go Tests"},
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
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
						return &fantasy.Response{
							Content: []fantasy.Content{
								fantasy.TextContent{Text: "   "},
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
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
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
			titleModelLookup: func(ProviderAPIKeys) (fantasy.LanguageModel, error) {
				return &titleTestModel{
					generateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
						return &fantasy.Response{
							Content: []fantasy.Content{
								fantasy.TextContent{Text: chat.Title},
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

	raw, err := json.Marshal(contentFromText(text))
	require.NoError(t, err)

	return database.ChatMessage{
		Role: string(fantasy.MessageRoleUser),
		Content: pqtype.NullRawMessage{
			RawMessage: raw,
			Valid:      true,
		},
	}
}
