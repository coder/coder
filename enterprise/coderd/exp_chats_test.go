package coderd_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestChatStreamRelay(t *testing.T) {
	t.Parallel()

	t.Run("RelayMessagePartsAcrossReplicas", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		db, pubsub := dbtestutil.NewDB(t)
		firstClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
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
		provider, err := codersdk.NewExperimentalClient(firstClient).CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test",
			BaseURL:     openai,
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatProviderConfigSourceDatabase, provider.Source)

		model, err := codersdk.NewExperimentalClient(firstClient).CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:             provider.Provider,
			Model:                "gpt-4",
			DisplayName:          "GPT-4",
			ContextLimit:         &[]int64{1000}[0],
			CompressionThreshold: &[]int32{70}[0],
		})
		require.NoError(t, err)

		// Create a chat on the first replica
		chat, err := codersdk.NewExperimentalClient(firstClient).CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Test chat for relay",
			}},
			ModelConfigID: &model.ID,
		})
		require.NoError(t, err)

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

		var localClient *codersdk.ExperimentalClient
		var relayClient *codersdk.ExperimentalClient
		switch runningChat.WorkerID.UUID {
		case firstReplicaID:
			localClient = codersdk.NewExperimentalClient(firstClient)
			relayClient = codersdk.NewExperimentalClient(secondClient)
		case secondReplicaID:
			localClient = codersdk.NewExperimentalClient(secondClient)
			relayClient = codersdk.NewExperimentalClient(firstClient)
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

		firstEvents, firstStream, err := localClient.StreamChat(ctx, chat.ID, nil)
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
		require.Equal(t, codersdk.ChatMessageRoleAssistant, firstEvent.MessagePart.Role)

		secondEvents, secondStream, err := relayClient.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer secondStream.Close()

		secondSnapshotEvent := waitForStreamTextPart(ctx, t, secondEvents, firstChunkText)
		require.Equal(t, codersdk.ChatMessageRoleAssistant, secondSnapshotEvent.MessagePart.Role)

		secondChunkText := "relay-part-two"
		streamingChunks <- chattest.OpenAITextChunks(secondChunkText)[0]
		waitForStreamTextPart(ctx, t, firstEvents, secondChunkText)
		waitForStreamTextPart(ctx, t, secondEvents, secondChunkText)

		close(streamingChunks)
	})

	// This test verifies that the relay WebSocket dial works when replicas
	// use TLS (mesh certificates) and the original request authenticates
	// via cookies only (as browsers do for WebSocket upgrades, since
	// browsers cannot set custom headers on WebSocket connections).
	//
	// The bug: codersdk.Client.Dial() does not propagate c.HTTPClient to
	// websocket.DialOptions.HTTPClient, so the websocket library falls
	// back to http.DefaultClient. With TLS between replicas,
	// http.DefaultClient lacks the required TLS config, causing a 401
	// (or TLS handshake failure) when the relay subscriber replica
	// dials the worker replica.
	t.Run("RelayWithTLSAndCookieAuth", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		certificates := []tls.Certificate{testutil.GenerateTLSCertificate(t, "localhost")}
		db, pubsub := dbtestutil.NewDB(t)
		firstClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:        db,
				Pubsub:          pubsub,
				TLSCertificates: certificates,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureHighAvailability: 1,
				},
			},
		})

		secondClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:        db,
				Pubsub:          pubsub,
				TLSCertificates: certificates,
			},
			DontAddLicense:   true,
			DontAddFirstUser: true,
		})

		// Authenticate the second client using cookies only, simulating
		// browser WebSocket behavior. Browsers cannot set custom
		// headers (like Coder-Session-Token) on WebSocket upgrades;
		// they rely on cookies for authentication.
		//
		// We intentionally do NOT call secondClient.SetSessionToken()
		// because that would set the Coder-Session-Token header,
		// which masks the bug.
		//nolint:gocritic // Test uses owner client session token for cookie-based auth.
		sessionToken := firstClient.SessionToken()
		// Set session token via cookie on the second client's HTTP
		// jar so that HTTP requests authenticate, but the WebSocket
		// relay between replicas only gets cookie-based auth forwarded.
		cookieJar := secondClient.HTTPClient.Jar
		if cookieJar == nil {
			var jarErr error
			cookieJar, jarErr = cookiejar.New(nil)
			require.NoError(t, jarErr)
			secondClient.HTTPClient.Jar = cookieJar
		}
		cookieJar.SetCookies(secondClient.URL, []*http.Cookie{{
			Name:  codersdk.SessionTokenCookie,
			Value: sessionToken,
		}})

		// Also set the session token header so regular API calls work
		// (e.g. Replicas(), CreateChatProvider()). The relay code
		// extracts credentials from the original request's headers,
		// which includes Cookie but the Coder-Session-Token header
		// won't be present on browser WebSocket requests.
		secondClient.SetSessionToken(sessionToken)

		// Verify we have two replicas.
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
		provider, err := codersdk.NewExperimentalClient(firstClient).CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test",
			BaseURL:     openai,
		})
		require.NoError(t, err)

		model, err := codersdk.NewExperimentalClient(firstClient).CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:             provider.Provider,
			Model:                "gpt-4",
			DisplayName:          "GPT-4",
			ContextLimit:         &[]int64{1000}[0],
			CompressionThreshold: &[]int32{70}[0],
		})
		require.NoError(t, err)

		// Create a chat on the first replica.
		chat, err := codersdk.NewExperimentalClient(firstClient).CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Test chat for TLS relay",
			}},
			ModelConfigID: &model.ID,
		})
		require.NoError(t, err)

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

		var localClient *codersdk.ExperimentalClient
		var relayClient *codersdk.ExperimentalClient
		switch runningChat.WorkerID.UUID {
		case firstReplicaID:
			localClient = codersdk.NewExperimentalClient(firstClient)
			relayClient = codersdk.NewExperimentalClient(secondClient)
		case secondReplicaID:
			localClient = codersdk.NewExperimentalClient(secondClient)
			relayClient = codersdk.NewExperimentalClient(firstClient)
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

		// Subscribe on the worker replica to start the stream.
		firstEvents, firstStream, err := localClient.StreamChat(ctx, chat.ID, nil)
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

		// Send a chunk on the worker.
		firstChunkText := "tls-relay-part-one"
		streamingChunks <- chattest.OpenAITextChunks(firstChunkText)[0]
		firstEvent := waitForStreamTextPart(ctx, t, firstEvents, firstChunkText)
		require.Equal(t, codersdk.ChatMessageRoleAssistant, firstEvent.MessagePart.Role)

		// Subscribe from the non-worker replica. This triggers the
		// relay dial to the worker over TLS. With the bug, this
		// fails because Dial() does not propagate HTTPClient (with
		// the TLS config) to the websocket library.
		secondEvents, secondStream, err := relayClient.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer secondStream.Close()

		// The relay should deliver the already-sent chunk as a
		// snapshot event.
		secondSnapshotEvent := waitForStreamTextPart(ctx, t, secondEvents, firstChunkText)
		require.Equal(t, codersdk.ChatMessageRoleAssistant, secondSnapshotEvent.MessagePart.Role)

		// Send another chunk and verify it flows through the relay.
		secondChunkText := "tls-relay-part-two"
		streamingChunks <- chattest.OpenAITextChunks(secondChunkText)[0]
		waitForStreamTextPart(ctx, t, firstEvents, secondChunkText)
		waitForStreamTextPart(ctx, t, secondEvents, secondChunkText)

		close(streamingChunks)
	})

	// This test verifies that the relay works when the subscriber
	// replica's incoming request authenticates via cookies only,
	// exactly as a browser WebSocket upgrade does. Browsers cannot
	// set custom headers (like Coder-Session-Token) on WebSocket
	// connections, so the relay must forward the Cookie header and
	// the worker replica must accept it.
	//
	// Previous tests used SetSessionToken() which sets the
	// Coder-Session-Token header, masking this code path.
	t.Run("RelayCookieOnlyAuth", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		db, pubsub := dbtestutil.NewDB(t)
		firstClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
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

		//nolint:gocritic // Test uses owner client session token for cookie-based relay auth.
		sessionToken := firstClient.SessionToken()

		// Configure the second client to authenticate via cookies		// only for WebSocket dials, matching browser behavior.
		// For regular HTTP API calls we still need the header.
		secondClient.SetSessionToken(sessionToken)
		secondClient.SessionTokenProvider = cookieOnlySessionTokenProvider{
			token:     sessionToken,
			targetURL: secondClient.URL,
		}

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

		//nolint:gocritic // Test uses owner client to configure providers.
		provider, err := codersdk.NewExperimentalClient(firstClient).CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test",
			BaseURL:     openai,
		})
		require.NoError(t, err)

		model, err := codersdk.NewExperimentalClient(firstClient).CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:             provider.Provider,
			Model:                "gpt-4",
			DisplayName:          "GPT-4",
			ContextLimit:         &[]int64{1000}[0],
			CompressionThreshold: &[]int32{70}[0],
		})
		require.NoError(t, err)

		chat, err := codersdk.NewExperimentalClient(firstClient).CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Test cookie-only relay",
			}},
			ModelConfigID: &model.ID,
		})
		require.NoError(t, err)

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

		var localClient *codersdk.ExperimentalClient
		var relayClient *codersdk.ExperimentalClient
		switch runningChat.WorkerID.UUID {
		case firstReplicaID:
			localClient = codersdk.NewExperimentalClient(firstClient)
			relayClient = codersdk.NewExperimentalClient(secondClient)
		case secondReplicaID:
			localClient = codersdk.NewExperimentalClient(secondClient)
			relayClient = codersdk.NewExperimentalClient(firstClient)
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

		firstEvents, firstStream, err := localClient.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer firstStream.Close()

		select {
		case <-chatStreamStarted:
		case <-ctx.Done():
			require.FailNowf(
				t,
				"timed out waiting for OpenAI stream request",
				"chat stream did not start: %v",
				ctx.Err(),
			)
		}

		firstChunkText := "cookie-relay-part-one"
		streamingChunks <- chattest.OpenAITextChunks(firstChunkText)[0]
		firstEvent := waitForStreamTextPart(ctx, t, firstEvents, firstChunkText)
		require.Equal(t, codersdk.ChatMessageRoleAssistant, firstEvent.MessagePart.Role)

		// Subscribe from the non-worker replica with cookie-only
		// auth. This triggers the relay dial. If the relay doesn't
		// correctly forward cookies, this fails with 401.
		secondEvents, secondStream, err := relayClient.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer secondStream.Close()

		secondSnapshotEvent := waitForStreamTextPart(ctx, t, secondEvents, firstChunkText)
		require.Equal(t, codersdk.ChatMessageRoleAssistant, secondSnapshotEvent.MessagePart.Role)

		secondChunkText := "cookie-relay-part-two"
		streamingChunks <- chattest.OpenAITextChunks(secondChunkText)[0]
		waitForStreamTextPart(ctx, t, firstEvents, secondChunkText)
		waitForStreamTextPart(ctx, t, secondEvents, secondChunkText)

		close(streamingChunks)
	})

	// This test verifies that cookie-only relay auth works when
	// EnableHostPrefix is true. When the subscriber replica's
	// HTTPCookies.Middleware normalizes __Host-coder_session_token
	// to coder_session_token, the relay forwards the bare cookie.
	// On the worker replica, the same middleware must not strip it.
	//
	// The fix ensures relayHeaders also extracts the token value
	// and sets the Coder-Session-Token header so the worker
	// replica can authenticate regardless of cookie prefix config.
	t.Run("RelayCookieOnlyAuthWithHostPrefix", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		db, pubsub := dbtestutil.NewDB(t)
		hostPrefixValues := coderdtest.DeploymentValues(t, func(dv *codersdk.DeploymentValues) {
			dv.HTTPCookies.EnableHostPrefix = true
			dv.HTTPCookies.Secure = true
		})
		firstClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:         db,
				Pubsub:           pubsub,
				DeploymentValues: hostPrefixValues,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureHighAvailability: 1,
				},
			},
		})

		secondClient, _ := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				Database:         db,
				Pubsub:           pubsub,
				DeploymentValues: hostPrefixValues,
			},
			DontAddLicense:   true,
			DontAddFirstUser: true,
		})

		//nolint:gocritic // Test uses owner client session token for cookie-based relay auth.
		sessionToken := firstClient.SessionToken()

		// Use cookie-only auth for WebSocket, as browsers do.		// With EnableHostPrefix, the browser would have
		// __Host-coder_session_token but the middleware
		// normalizes it. The relay copies the normalized cookie.
		secondClient.SetSessionToken(sessionToken)
		secondClient.SessionTokenProvider = cookieOnlySessionTokenProvider{
			token:      sessionToken,
			targetURL:  secondClient.URL,
			hostPrefix: true,
		}

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

		//nolint:gocritic // Test uses owner client to configure providers.
		provider, err := codersdk.NewExperimentalClient(firstClient).CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test",
			BaseURL:     openai,
		})
		require.NoError(t, err)

		model, err := codersdk.NewExperimentalClient(firstClient).CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:             provider.Provider,
			Model:                "gpt-4",
			DisplayName:          "GPT-4",
			ContextLimit:         &[]int64{1000}[0],
			CompressionThreshold: &[]int32{70}[0],
		})
		require.NoError(t, err)

		chat, err := codersdk.NewExperimentalClient(firstClient).CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Test host-prefix relay",
			}},
			ModelConfigID: &model.ID,
		})
		require.NoError(t, err)

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

		var localClient *codersdk.ExperimentalClient
		var relayClient *codersdk.ExperimentalClient
		switch runningChat.WorkerID.UUID {
		case firstReplicaID:
			localClient = codersdk.NewExperimentalClient(firstClient)
			relayClient = codersdk.NewExperimentalClient(secondClient)
		case secondReplicaID:
			localClient = codersdk.NewExperimentalClient(secondClient)
			relayClient = codersdk.NewExperimentalClient(firstClient)
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

		firstEvents, firstStream, err := localClient.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer firstStream.Close()

		select {
		case <-chatStreamStarted:
		case <-ctx.Done():
			require.FailNowf(
				t,
				"timed out waiting for OpenAI stream request",
				"chat stream did not start: %v",
				ctx.Err(),
			)
		}

		firstChunkText := "hostprefix-relay-part-one"
		streamingChunks <- chattest.OpenAITextChunks(firstChunkText)[0]
		firstEvent := waitForStreamTextPart(ctx, t, firstEvents, firstChunkText)
		require.Equal(t, codersdk.ChatMessageRoleAssistant, firstEvent.MessagePart.Role)

		// This subscribe triggers the relay. With the bug, the
		// worker replica's HTTPCookies.Middleware strips the bare
		// coder_session_token cookie and there's no fallback
		// Coder-Session-Token header, causing a 401.
		secondEvents, secondStream, err := relayClient.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer secondStream.Close()

		secondSnapshotEvent := waitForStreamTextPart(ctx, t, secondEvents, firstChunkText)
		require.Equal(t, codersdk.ChatMessageRoleAssistant, secondSnapshotEvent.MessagePart.Role)

		secondChunkText := "hostprefix-relay-part-two"
		streamingChunks <- chattest.OpenAITextChunks(secondChunkText)[0]
		waitForStreamTextPart(ctx, t, firstEvents, secondChunkText)
		waitForStreamTextPart(ctx, t, secondEvents, secondChunkText)

		close(streamingChunks)
	})

	t.Run("RelaySnapshotIncludesBufferedParts", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		db, pubsub := dbtestutil.NewDB(t)
		firstClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
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

		// Verify we have two replicas.
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
		provider, err := codersdk.NewExperimentalClient(firstClient).CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test",
			BaseURL:     openai,
		})
		require.NoError(t, err)

		model, err := codersdk.NewExperimentalClient(firstClient).CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			Provider:             provider.Provider,
			Model:                "gpt-4",
			DisplayName:          "GPT-4",
			ContextLimit:         &[]int64{1000}[0],
			CompressionThreshold: &[]int32{70}[0],
		})
		require.NoError(t, err)

		// Create a chat on the first replica.
		chat, err := codersdk.NewExperimentalClient(firstClient).CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{{
				Type: codersdk.ChatInputPartTypeText,
				Text: "Test chat for buffered relay",
			}},
			ModelConfigID: &model.ID,
		})
		require.NoError(t, err)

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

		var localClient *codersdk.ExperimentalClient
		var relayClient *codersdk.ExperimentalClient
		switch runningChat.WorkerID.UUID {
		case firstReplicaID:
			localClient = codersdk.NewExperimentalClient(firstClient)
			relayClient = codersdk.NewExperimentalClient(secondClient)
		case secondReplicaID:
			localClient = codersdk.NewExperimentalClient(secondClient)
			relayClient = codersdk.NewExperimentalClient(firstClient)
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

		// Subscribe on the local (worker) replica so the stream is
		// consumed and chunks flow through the pipeline.
		localEvents, localStream, err := localClient.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer localStream.Close()

		// Wait for the OpenAI handler to start serving the stream.
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

		// Send multiple chunks BEFORE the relay subscriber connects.
		// This is the key difference from the existing test: we
		// buffer several parts so the drainInitial timer in
		// newRemotePartsProvider must collect them all.
		bufferedTexts := []string{"buffered-one", "buffered-two", "buffered-three"}
		for _, text := range bufferedTexts {
			streamingChunks <- chattest.OpenAITextChunks(text)[0]
			// Confirm each part arrives on the local subscriber so
			// we know it has been processed by the worker.
			waitForStreamTextPart(ctx, t, localEvents, text)
		}

		// NOW connect the relay subscriber on the non-worker replica.
		// The relay must pick up all three buffered parts in its
		// initial snapshot via the drainInitial loop.
		relayEvents, relayStream, err := relayClient.StreamChat(ctx, chat.ID, nil)
		require.NoError(t, err)
		defer relayStream.Close()

		// Verify every buffered part arrives on the relay subscriber.
		for _, text := range bufferedTexts {
			event := waitForStreamTextPart(ctx, t, relayEvents, text)
			require.Equal(t, codersdk.ChatMessageRoleAssistant, event.MessagePart.Role)
		}

		// Send one more chunk after the relay subscriber is connected
		// and verify it arrives through the live channel.
		liveText := "live-after-relay"
		streamingChunks <- chattest.OpenAITextChunks(liveText)[0]
		waitForStreamTextPart(ctx, t, localEvents, liveText)
		waitForStreamTextPart(ctx, t, relayEvents, liveText)

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
	expClient := codersdk.NewExperimentalClient(client)

	//nolint:gocritic // Test uses owner client to configure chat providers.
	provider, err := expClient.CreateChatProvider(
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

	firstModel, err := expClient.CreateChatModelConfig(
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

	secondModel, err := expClient.CreateChatModelConfig(
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

	modelConfigs, err := expClient.ListChatModelConfigs(ctx)
	require.NoError(t, err)
	firstStored := findChatModelConfigByID(t, modelConfigs, firstModel.ID)
	secondStored := findChatModelConfigByID(t, modelConfigs, secondModel.ID)
	require.False(t, firstStored.IsDefault)
	require.True(t, secondStored.IsDefault)

	updatedFirst, err := expClient.UpdateChatModelConfig(
		ctx,
		firstModel.ID,
		codersdk.UpdateChatModelConfigRequest{
			IsDefault: &trueValue,
		},
	)
	require.NoError(t, err)
	require.True(t, updatedFirst.IsDefault)

	modelConfigs, err = expClient.ListChatModelConfigs(ctx)
	require.NoError(t, err)
	firstStored = findChatModelConfigByID(t, modelConfigs, firstModel.ID)
	secondStored = findChatModelConfigByID(t, modelConfigs, secondModel.ID)
	require.True(t, firstStored.IsDefault)
	require.False(t, secondStored.IsDefault)

	updatedFirst, err = expClient.UpdateChatModelConfig(
		ctx,
		firstModel.ID,
		codersdk.UpdateChatModelConfigRequest{
			IsDefault: &falseValue,
		},
	)
	require.NoError(t, err)
	require.False(t, updatedFirst.IsDefault)

	modelConfigs, err = expClient.ListChatModelConfigs(ctx)
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

// cookieOnlySessionTokenProvider authenticates HTTP requests via the
// Coder-Session-Token header (for regular API calls) but
// authenticates WebSocket dials via Cookie only, matching how
// browsers behave (the native WebSocket constructor cannot set
// custom headers).
type cookieOnlySessionTokenProvider struct {
	token     string
	targetURL *url.URL
	// hostPrefix, when true, sends the cookie with the
	// __Host- prefix as browsers do with secure cookies.
	hostPrefix bool
}

func (p cookieOnlySessionTokenProvider) AsRequestOption() codersdk.RequestOption {
	return func(req *http.Request) {
		req.Header.Set(codersdk.SessionTokenHeader, p.token)
	}
}

func (p cookieOnlySessionTokenProvider) GetSessionToken() string {
	return p.token
}

func (p cookieOnlySessionTokenProvider) SetDialOption(opts *websocket.DialOptions) {
	// Browsers send cookies automatically on WebSocket upgrades
	// but cannot send custom headers. Simulate this by setting
	// only the Cookie header.
	if opts.HTTPHeader == nil {
		opts.HTTPHeader = make(http.Header)
	}
	cookieName := codersdk.SessionTokenCookie
	if p.hostPrefix {
		cookieName = "__Host-" + cookieName
	}
	opts.HTTPHeader.Set("Cookie", cookieName+"="+p.token)
}

func TestCreateChatNonDefaultOrg(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: func() *codersdk.DeploymentValues {
				v := coderdtest.DeploymentValues(t)
				v.Experiments = []string{string(codersdk.ExperimentAgents)}
				return v
			}(),
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	expClient := codersdk.NewExperimentalClient(client)

	// Set up a chat provider and model config.
	provider, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseURL:     "https://example.com",
	})
	require.NoError(t, err)
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:             provider.Provider,
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		IsDefault:            ptr.Ref(true),
		ContextLimit:         ptr.Ref(int64(1000)),
		CompressionThreshold: ptr.Ref(int32(70)),
	})
	require.NoError(t, err)

	// Create a second (non-default) org via the API.
	secondOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

	// Create a member with agents-access in both orgs.
	memberClientRaw, member := coderdtest.CreateAnotherUser(
		t, client, firstUser.OrganizationID,
		rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID),
		rbac.ScopedRoleAgentsAccess(secondOrg.ID),
	)
	memberClient := codersdk.NewExperimentalClient(memberClientRaw)
	// Create a chat in the non-default org.
	chat, err := memberClient.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: secondOrg.ID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello from non-default org",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, secondOrg.ID, chat.OrganizationID)
	require.Equal(t, member.ID, chat.OwnerID)

	// Verify the chat is visible when listing.
	chats, err := memberClient.ListChats(ctx, nil)
	require.NoError(t, err)
	var found bool
	for _, c := range chats {
		if c.ID == chat.ID {
			found = true
			require.Equal(t, secondOrg.ID, c.OrganizationID)
			break
		}
	}
	require.True(t, found, "chat should be visible in list")
}

func TestListChats_OrgAdminOnlySeesOwnChats(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	client, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: func() *codersdk.DeploymentValues {
				v := coderdtest.DeploymentValues(t)
				v.Experiments = []string{string(codersdk.ExperimentAgents)}
				return v
			}(),
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureMultipleOrganizations: 1,
			},
		},
	})
	expClient := codersdk.NewExperimentalClient(client)

	// Set up a chat provider and model config.
	provider, err := expClient.CreateChatProvider(ctx, codersdk.CreateChatProviderConfigRequest{
		Provider:    "openai",
		DisplayName: "OpenAI",
		APIKey:      "test-key",
		BaseURL:     "https://example.com",
	})
	require.NoError(t, err)
	_, err = expClient.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
		Provider:             provider.Provider,
		Model:                "gpt-4o-mini",
		DisplayName:          "Test Model",
		IsDefault:            ptr.Ref(true),
		ContextLimit:         ptr.Ref(int64(1000)),
		CompressionThreshold: ptr.Ref(int32(70)),
	})
	require.NoError(t, err)

	// Create a second (non-default) org.
	secondOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

	// Create a member with agents-access in both orgs.
	memberClientRaw, _ := coderdtest.CreateAnotherUser(
		t, client, firstUser.OrganizationID,
		rbac.ScopedRoleAgentsAccess(firstUser.OrganizationID),
		rbac.ScopedRoleAgentsAccess(secondOrg.ID),
	)
	memberExp := codersdk.NewExperimentalClient(memberClientRaw)
	// Member creates a chat in the second org.
	memberChat, err := memberExp.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: secondOrg.ID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello from member",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, secondOrg.ID, memberChat.OrganizationID)

	// Create an org admin in the second org with agents access.
	adminClientRaw, _ := coderdtest.CreateAnotherUser(
		t, client, firstUser.OrganizationID,
		rbac.ScopedRoleOrgAdmin(secondOrg.ID), rbac.ScopedRoleAgentsAccess(secondOrg.ID),
	)
	adminExp := codersdk.NewExperimentalClient(adminClientRaw)

	// Admin creates a chat in the second org.
	adminChat, err := adminExp.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: secondOrg.ID,
		Content: []codersdk.ChatInputPart{
			{
				Type: codersdk.ChatInputPartTypeText,
				Text: "hello from admin",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, secondOrg.ID, adminChat.OrganizationID)

	// Admin lists chats -- should only see their own chat.
	// TODO: The handler currently filters by OwnerID (the
	// authenticated user), so org admins cannot see other
	// users' chats even though RBAC would allow it. If the
	// handler gains an owner filter parameter, update this
	// test to verify cross-user visibility.
	adminChats, err := adminExp.ListChats(ctx, nil)
	require.NoError(t, err)

	var foundAdmin, foundMember bool
	for _, c := range adminChats {
		if c.ID == adminChat.ID {
			foundAdmin = true
		}
		if c.ID == memberChat.ID {
			foundMember = true
		}
	}
	require.True(t, foundAdmin, "admin should see own chat")
	require.False(t, foundMember, "admin should NOT see member chat (OwnerID filter)")

	// Positive control: member can list their own chat.
	memberChats, err := memberExp.ListChats(ctx, nil)
	require.NoError(t, err)
	var memberSeeOwn bool
	for _, c := range memberChats {
		if c.ID == memberChat.ID {
			memberSeeOwn = true
		}
	}
	require.True(t, memberSeeOwn, "member should see own chat")
}
