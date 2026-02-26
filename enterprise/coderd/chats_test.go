package coderd_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chattest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestChatStreamRelay(t *testing.T) {
	t.Parallel()

	t.Run("RelayMessagePartsAcrossReplicas", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		db, pubsub := dbtestutil.NewDB(t)
		firstClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureHighAvailability: 1,
				},
			},
		})

		secondClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database: db,
				Pubsub:   pubsub,
			},
			DontAddLicense:   true,
			DontAddFirstUser: true,
		})
		secondClient.SetSessionToken(firstClient.SessionToken())

		// Verify we have two replicas
		replicas, err := secondClient.Replicas(ctx)
		require.NoError(t, err)
		require.Len(t, replicas, 2)
		firstReplicaID := replicaIDForClientURL(t, firstClient.URL, replicas)
		secondReplicaID := replicaIDForClientURL(t, secondClient.URL, replicas)

		streamingChunks := make(chan chattest.OpenAIChunk, 8)
		chatStreamStarted := make(chan struct{}, 1)
		openai := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if req.Stream {
				select {
				case chatStreamStarted <- struct{}{}:
				default:
				}
				return chattest.OpenAIResponse{StreamingChunks: streamingChunks}
			}
			return chattest.OpenAINonStreamingResponse("ok")
		})

		//nolint:gocritic // Test uses owner client to configure chat providers.
		provider, err := firstClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test",
			BaseURL:     openai,
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatProviderConfigSourceDatabase, provider.Source)

		model, err := firstClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:             provider.Provider,
			Model:                "gpt-4",
			DisplayName:          "GPT-4",
			ContextLimit:         &[]int64{1000}[0],
			CompressionThreshold: &[]int32{70}[0],
		})
		require.NoError(t, err)

		// Create a chat on the first replica
		chat, err := firstClient.CreateChat(ctx, codersdk.CreateChatRequest{
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Test chat for relay",
			}},
			ModelConfigID: &model.ID,
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatStatusPending, chat.Status)

		var runningChat database.Chat
		require.Eventually(t, func() bool {
			current, getErr := db.GetChatByID(ctx, chat.ID)
			if getErr != nil {
				return false
			}
			if current.Status != database.ChatStatusRunning || !current.WorkerID.Valid {
				return false
			}
			runningChat = current
			return true
		}, testutil.WaitLong, testutil.IntervalFast)

		var localClient *codersdk.Client
		var relayClient *codersdk.Client
		switch runningChat.WorkerID.UUID {
		case firstReplicaID:
			localClient = firstClient
			relayClient = secondClient
		case secondReplicaID:
			localClient = secondClient
			relayClient = firstClient
		default:
			require.FailNowf(
				t,
				"worker replica was not recognized",
				"worker %s was not one of %s or %s",
				runningChat.WorkerID.UUID,
				firstReplicaID,
				secondReplicaID,
			)
		}

		firstEvents, firstStream, err := localClient.StreamChat(ctx, chat.ID)
		require.NoError(t, err)
		defer firstStream.Close()

		select {
		case <-chatStreamStarted:
		case <-ctx.Done():
			require.FailNowf(
				t,
				"timed out waiting for OpenAI stream request",
				"chat stream request did not start before context deadline: %v",
				ctx.Err(),
			)
		}

		firstChunkText := "relay-part-one"
		streamingChunks <- chattest.OpenAITextChunks(firstChunkText)[0]
		firstEvent := waitForStreamTextPart(ctx, t, firstEvents, firstChunkText)
		require.Equal(t, "assistant", firstEvent.MessagePart.Role)

		secondEvents, secondStream, err := relayClient.StreamChat(ctx, chat.ID)
		require.NoError(t, err)
		defer secondStream.Close()

		secondSnapshotEvent := waitForStreamTextPart(ctx, t, secondEvents, firstChunkText)
		require.Equal(t, "assistant", secondSnapshotEvent.MessagePart.Role)

		secondChunkText := "relay-part-two"
		streamingChunks <- chattest.OpenAITextChunks(secondChunkText)[0]
		waitForStreamTextPart(ctx, t, firstEvents, secondChunkText)
		waitForStreamTextPart(ctx, t, secondEvents, secondChunkText)

		close(streamingChunks)
	})
}

func waitForStreamTextPart(
	ctx context.Context,
	t *testing.T,
	events <-chan codersdk.ChatStreamEvent,
	expectedText string,
) codersdk.ChatStreamEvent {
	t.Helper()

	for {
		select {
		case <-ctx.Done():
			require.FailNowf(
				t,
				"timed out waiting for chat stream event",
				"expected text part %q before context deadline: %v",
				expectedText,
				ctx.Err(),
			)
		case event, ok := <-events:
			require.Truef(t, ok, "chat stream closed while waiting for %q", expectedText)

			if event.Type == codersdk.ChatStreamEventTypeError {
				errMessage := "unknown chat stream error"
				if event.Error != nil && event.Error.Message != "" {
					errMessage = event.Error.Message
				}
				require.FailNowf(
					t,
					"chat stream returned error event",
					"while waiting for %q: %s",
					expectedText,
					errMessage,
				)
			}

			if event.Type != codersdk.ChatStreamEventTypeMessagePart || event.MessagePart == nil {
				continue
			}
			if event.MessagePart.Part.Type != codersdk.ChatMessagePartTypeText {
				continue
			}

			require.Equal(t, expectedText, event.MessagePart.Part.Text)
			return event
		}
	}
}

func replicaIDForClientURL(
	t *testing.T,
	clientURL *url.URL,
	replicas []codersdk.Replica,
) uuid.UUID {
	t.Helper()

	for _, replica := range replicas {
		relayURL, err := url.Parse(replica.RelayAddress)
		require.NoErrorf(
			t,
			err,
			"parse replica relay address %q",
			replica.RelayAddress,
		)
		if relayURL.Host == clientURL.Host {
			return replica.ID
		}
	}

	require.FailNowf(
		t,
		"missing replica for client URL",
		"client host %q not present in replica list",
		clientURL.Host,
	)
	return uuid.Nil
}

func TestChatModelConfigDefault(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	client, _ := coderdenttest.New(t, nil)

	//nolint:gocritic // Test uses owner client to configure chat providers.
	provider, err := client.CreateChatProvider(
		ctx,
		codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test",
			BaseURL:     "https://example.com",
		},
	)
	require.NoError(t, err)

	contextLimit := int64(1000)
	compressionThreshold := int32(70)
	trueValue := true
	falseValue := false

	firstModel, err := client.CreateChatModelConfig(
		ctx,
		codersdk.CreateChatModelConfigRequest{
			Provider:             provider.Provider,
			Model:                "gpt-5-a",
			DisplayName:          "GPT 5 A",
			IsDefault:            &trueValue,
			ContextLimit:         &contextLimit,
			CompressionThreshold: &compressionThreshold,
		},
	)
	require.NoError(t, err)
	require.True(t, firstModel.IsDefault)

	secondModel, err := client.CreateChatModelConfig(
		ctx,
		codersdk.CreateChatModelConfigRequest{
			Provider:             provider.Provider,
			Model:                "gpt-5-b",
			DisplayName:          "GPT 5 B",
			IsDefault:            &trueValue,
			ContextLimit:         &contextLimit,
			CompressionThreshold: &compressionThreshold,
		},
	)
	require.NoError(t, err)
	require.True(t, secondModel.IsDefault)

	modelConfigs, err := client.ListChatModelConfigs(ctx)
	require.NoError(t, err)
	firstStored := findChatModelConfigByID(t, modelConfigs, firstModel.ID)
	secondStored := findChatModelConfigByID(t, modelConfigs, secondModel.ID)
	require.False(t, firstStored.IsDefault)
	require.True(t, secondStored.IsDefault)

	updatedFirst, err := client.UpdateChatModelConfig(
		ctx,
		firstModel.ID,
		codersdk.UpdateChatModelConfigRequest{
			IsDefault: &trueValue,
		},
	)
	require.NoError(t, err)
	require.True(t, updatedFirst.IsDefault)

	modelConfigs, err = client.ListChatModelConfigs(ctx)
	require.NoError(t, err)
	firstStored = findChatModelConfigByID(t, modelConfigs, firstModel.ID)
	secondStored = findChatModelConfigByID(t, modelConfigs, secondModel.ID)
	require.True(t, firstStored.IsDefault)
	require.False(t, secondStored.IsDefault)

	updatedFirst, err = client.UpdateChatModelConfig(
		ctx,
		firstModel.ID,
		codersdk.UpdateChatModelConfigRequest{
			IsDefault: &falseValue,
		},
	)
	require.NoError(t, err)
	require.False(t, updatedFirst.IsDefault)

	modelConfigs, err = client.ListChatModelConfigs(ctx)
	require.NoError(t, err)
	firstStored = findChatModelConfigByID(t, modelConfigs, firstModel.ID)
	secondStored = findChatModelConfigByID(t, modelConfigs, secondModel.ID)
	require.False(t, firstStored.IsDefault)
	require.True(t, secondStored.IsDefault)
}

func findChatModelConfigByID(
	t *testing.T,
	modelConfigs []codersdk.ChatModelConfig,
	id uuid.UUID,
) codersdk.ChatModelConfig {
	t.Helper()

	for _, modelConfig := range modelConfigs {
		if modelConfig.ID == id {
			return modelConfig
		}
	}

	require.FailNowf(t, "missing model config", "model config %s not found", id)
	return codersdk.ChatModelConfig{}
}
