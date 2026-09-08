package toolsdk_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

// stallTransport hangs every request until its context is canceled.
type stallTransport struct{}

func (stallTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

type signalPathTransport struct {
	path    string
	seen    chan struct{}
	release chan struct{}
	once    sync.Once
}

func (t *signalPathTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil || req.URL.Path != t.path {
		return res, err
	}
	t.once.Do(func() { close(t.seen) })
	select {
	case <-t.release:
		return res, nil
	case <-req.Context().Done():
		_ = res.Body.Close()
		return nil, req.Context().Err()
	}
}

// Chat tools need a chat-enabled coderd (provider keys, a default model
// config, and an AI bridge daemon), so they are tested separately from
// TestTools. Subtests run sequentially and share the deployment.
// nolint:tparallel,paralleltest
func TestChatTools(t *testing.T) {
	providerKeys := coderdtest.FakeOpenAICompatProviderAPIKeys(t)
	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues:      coderdtest.DeploymentValues(t),
		ChatProviderAPIKeys:   &providerKeys,
		ChatFileTokenKeyCache: jwtutils.StaticKey{ID: "1", Key: bytes.Repeat([]byte("k"), 64)},
	})
	firstUser := coderdtest.CreateFirstUser(t, client)
	expClient := codersdk.NewExperimentalClient(client)
	defaultModelConfig := coderdtest.CreateOpenAICompatChatModel(t, expClient, "")
	aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)

	tb, err := toolsdk.NewDeps(client)
	require.NoError(t, err)

	t.Run("ListChatModelConfigs", func(t *testing.T) {
		result, err := testTool(t, toolsdk.ListChatModelConfigs, tb, toolsdk.ListChatModelConfigsArgs{})
		require.NoError(t, err)
		require.Equal(t, firstUser.OrganizationID.String(), result.OrganizationID)
		require.Len(t, result.ModelConfigs, 1)
		require.Equal(t, defaultModelConfig.ID.String(), result.ModelConfigs[0].ID)
		require.Equal(t, coderdtest.TestChatModelOpenAICompat, result.ModelConfigs[0].Model)
		require.True(t, result.ModelConfigs[0].IsDefault)
	})

	t.Run("ListChatModelConfigsForOrganization", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		organization := dbgen.Organization(t, api.Database, database.Organization{})
		dbgen.OrganizationMember(t, api.Database, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         firstUser.UserID,
		})

		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAICompat,
			Name:    "other-org-" + uuid.NewString(),
			BaseURL: chattest.OpenAI(t),
			Enabled: true,
			APIKeys: []string{"test-api-key"},
		})
		require.NoError(t, err)
		contextLimit := int64(4096)
		model, err := expClient.CreateChatModel(ctx, organization.ID, codersdk.CreateChatModelRequest{
			AIProviderID: &provider.ID,
			Model:        "gpt-4o-other-org",
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)

		result, err := testTool(t, toolsdk.ListChatModelConfigs, tb, toolsdk.ListChatModelConfigsArgs{
			OrganizationID: organization.ID.String(),
		})
		require.NoError(t, err)
		require.Equal(t, organization.ID.String(), result.OrganizationID)
		require.Len(t, result.ModelConfigs, 1)
		require.Equal(t, model.ID.String(), result.ModelConfigs[0].ID)
	})

	t.Run("Lifecycle", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := testTool(t, toolsdk.CreateChat, tb, toolsdk.CreateChatArgs{
			Prompt:         "Say hello.",
			OrganizationID: firstUser.OrganizationID.String(),
			Labels:         map[string]string{"purpose": "toolsdk-test"},
		})
		require.NoError(t, err)
		chatID, err := uuid.Parse(created.ID)
		require.NoError(t, err)
		require.Equal(t, client.URL.String()+"/agents/"+created.ID, created.URL)

		coderdtest.WaitForChatSettled(ctx, t, api, chatID)

		got, err := testTool(t, toolsdk.GetChat, tb, toolsdk.GetChatArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.Equal(t, created.ID, got.ID)
		require.Equal(t, codersdk.ChatStatusWaiting, got.Status)
		require.Nil(t, got.LastError)
		require.False(t, got.Archived)

		sent, err := testTool(t, toolsdk.SendChatMessage, tb, toolsdk.SendChatMessageArgs{
			ChatID: created.ID,
			Text:   "Say hello again.",
		})
		require.NoError(t, err)
		require.False(t, sent.Queued)

		coderdtest.WaitForChatSettled(ctx, t, api, chatID)

		messages, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.False(t, messages.HasMore)
		var texts []string
		for _, msg := range messages.Messages {
			texts = append(texts, string(msg.Role)+": "+msg.Text)
		}
		require.Contains(t, texts, "user: Say hello.")
		require.Contains(t, texts, "user: Say hello again.")
		require.Contains(t, texts, "assistant: Hello from test server.")
		require.Equal(t, "user: Say hello.", texts[0])

		hookNoticeContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
			Type: codersdk.ChatMessagePartTypeHookNotice,
			Text: "Command denied by policy.",
		}})
		require.NoError(t, err)
		dbgen.ChatMessage(t, api.Database, database.ChatMessage{
			ChatID:        chatID,
			ModelConfigID: uuid.NullUUID{UUID: defaultModelConfig.ID, Valid: true},
			Role:          database.ChatMessageRoleUser,
			Content:       hookNoticeContent,
		})
		withNotice, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{ChatID: created.ID})
		require.NoError(t, err)
		var noticeTexts []string
		for _, msg := range withNotice.Messages {
			noticeTexts = append(noticeTexts, msg.Text)
		}
		require.Contains(t, noticeTexts, "Command denied by policy.")

		// A tool-call-only message filters to an empty page, so the
		// cursor must come from the unfiltered API page.
		toolCallContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: "call-1",
			ToolName:   "execute",
		}})
		require.NoError(t, err)
		toolCallMsg := dbgen.ChatMessage(t, api.Database, database.ChatMessage{
			ChatID:        chatID,
			ModelConfigID: uuid.NullUUID{UUID: defaultModelConfig.ID, Valid: true},
			Role:          database.ChatMessageRoleAssistant,
			Content:       toolCallContent,
		})

		firstPage, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{
			ChatID: created.ID,
			Limit:  1,
		})
		require.NoError(t, err)
		require.True(t, firstPage.HasMore)
		require.Empty(t, firstPage.Messages)
		require.Equal(t, toolCallMsg.ID, firstPage.NextBeforeID)
		olderPage, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{
			ChatID:   created.ID,
			BeforeID: firstPage.NextBeforeID,
		})
		require.NoError(t, err)
		require.NotEmpty(t, olderPage.Messages)
		for _, msg := range olderPage.Messages {
			require.Less(t, msg.ID, firstPage.NextBeforeID)
		}
		require.False(t, olderPage.HasMore)
		require.Zero(t, olderPage.NextBeforeID)

		archived, err := testTool(t, toolsdk.ArchiveChat, tb, toolsdk.ArchiveChatArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.NotEmpty(t, archived.Message)

		got, err = testTool(t, toolsdk.GetChat, tb, toolsdk.GetChatArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.True(t, got.Archived)
	})

	t.Run("DownloadChatFile", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		data := []byte("MCP chat UAT evidence\n")
		uploaded, err := expClient.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "evidence.txt", bytes.NewReader(data))
		require.NoError(t, err)
		chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "Review the evidence."},
				{Type: codersdk.ChatInputPartTypeFile, FileID: uploaded.ID},
			},
		})
		require.NoError(t, err)

		assertDownload := func(t *testing.T, got toolsdk.DownloadChatFileResponse) {
			t.Helper()
			require.Equal(t, uploaded.ID.String(), got.FileID)
			require.Equal(t, "evidence.txt", got.Name)
			require.Equal(t, "text/plain", got.MimeType)
			require.Equal(t, int64(len(data)), got.SizeBytes)
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(data)), got.SHA256)
			require.NotEmpty(t, got.URL)
			require.False(t, got.ExpiresAt.IsZero())

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, got.URL, nil)
			require.NoError(t, err)
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, http.StatusOK, res.StatusCode)
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Equal(t, data, body)
		}

		byID, err := testTool(t, toolsdk.DownloadChatFile, tb, toolsdk.DownloadChatFileArgs{FileID: uploaded.ID.String()})
		require.NoError(t, err)
		assertDownload(t, byID)
		byName, err := testTool(t, toolsdk.DownloadChatFile, tb, toolsdk.DownloadChatFileArgs{
			ChatID:   chat.ID.String(),
			FileName: "evidence.txt",
		})
		require.NoError(t, err)
		assertDownload(t, byName)

		status, err := testTool(t, toolsdk.GetChat, tb, toolsdk.GetChatArgs{ChatID: chat.ID.String()})
		require.NoError(t, err)
		require.Len(t, status.Files, 1)
		require.Equal(t, int64(len(data)), status.Files[0].SizeBytes)
		require.False(t, status.Files[0].CreatedAt.IsZero())

		messages, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{ChatID: chat.ID.String()})
		require.NoError(t, err)
		var attachingMessage *toolsdk.ChatToolMessage
		for i := range messages.Messages {
			if messages.Messages[i].Text == "Review the evidence." {
				attachingMessage = &messages.Messages[i]
				break
			}
		}
		require.NotNil(t, attachingMessage)
		require.Equal(t, []toolsdk.ChatToolMessageFile{{
			ID:       uploaded.ID.String(),
			Name:     "evidence.txt",
			MimeType: "text/plain",
		}}, attachingMessage.Files)

		coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)
		fileOnly, err := expClient.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "file-only.txt", bytes.NewReader([]byte("file only")))
		require.NoError(t, err)
		_, err = expClient.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeFile, FileID: fileOnly.ID}},
		})
		require.NoError(t, err)
		messages, err = testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{ChatID: chat.ID.String()})
		require.NoError(t, err)
		var fileOnlyMessage *toolsdk.ChatToolMessage
		for i := range messages.Messages {
			if len(messages.Messages[i].Files) == 1 && messages.Messages[i].Files[0].ID == fileOnly.ID.String() {
				fileOnlyMessage = &messages.Messages[i]
				break
			}
		}
		require.NotNil(t, fileOnlyMessage)
		require.Empty(t, fileOnlyMessage.Text)
		require.Equal(t, []toolsdk.ChatToolMessageFile{{
			ID:       fileOnly.ID.String(),
			Name:     "file-only.txt",
			MimeType: "text/plain",
		}}, fileOnlyMessage.Files)

		duplicateA, err := expClient.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "duplicate.txt", bytes.NewReader([]byte("a")))
		require.NoError(t, err)
		duplicateB, err := expClient.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "duplicate.txt", bytes.NewReader([]byte("bb")))
		require.NoError(t, err)
		ambiguousChat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "Compare these files."},
				{Type: codersdk.ChatInputPartTypeFile, FileID: duplicateA.ID},
				{Type: codersdk.ChatInputPartTypeFile, FileID: duplicateB.ID},
			},
		})
		require.NoError(t, err)
		_, err = testTool(t, toolsdk.DownloadChatFile, tb, toolsdk.DownloadChatFileArgs{
			ChatID:   ambiguousChat.ID.String(),
			FileName: "duplicate.txt",
		})
		require.ErrorContains(t, err, "multiple chat files")
		require.ErrorContains(t, err, duplicateA.ID.String())
		require.ErrorContains(t, err, duplicateB.ID.String())
		require.ErrorContains(t, err, "mime_type")
		require.ErrorContains(t, err, "size_bytes")

		_, err = testTool(t, toolsdk.DownloadChatFile, tb, toolsdk.DownloadChatFileArgs{
			ChatID:   ambiguousChat.ID.String(),
			FileName: "missing.txt",
		})
		require.ErrorContains(t, err, "no chat file")
		require.ErrorContains(t, err, "duplicate.txt")

		coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)
		coderdtest.WaitForChatSettled(ctx, t, api, ambiguousChat.ID)
	})

	t.Run("ForwardMessagePagination", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Create a pagination baseline."}},
		})
		require.NoError(t, err)
		coderdtest.WaitForChatSettled(ctx, t, api, chat.ID)

		existing, err := expClient.GetChatMessages(ctx, chat.ID, nil)
		require.NoError(t, err)
		var baselineID int64
		for _, msg := range existing.Messages {
			baselineID = max(baselineID, msg.ID)
		}
		require.Positive(t, baselineID)

		textContent := func(text string) database.ChatMessage {
			content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{Type: codersdk.ChatMessagePartTypeText, Text: text}})
			require.NoError(t, err)
			return database.ChatMessage{
				ChatID:        chat.ID,
				ModelConfigID: uuid.NullUUID{UUID: defaultModelConfig.ID, Valid: true},
				Role:          database.ChatMessageRoleUser,
				Content:       content,
			}
		}
		first := dbgen.ChatMessage(t, api.Database, textContent("forward one"))
		toolContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
			Type:       codersdk.ChatMessagePartTypeToolCall,
			ToolCallID: "forward-call",
			ToolName:   "execute",
		}})
		require.NoError(t, err)
		toolOnly := dbgen.ChatMessage(t, api.Database, database.ChatMessage{
			ChatID:        chat.ID,
			ModelConfigID: uuid.NullUUID{UUID: defaultModelConfig.ID, Valid: true},
			Role:          database.ChatMessageRoleAssistant,
			Content:       toolContent,
		})
		last := dbgen.ChatMessage(t, api.Database, textContent("forward two"))

		firstPage, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{
			ChatID:  chat.ID.String(),
			AfterID: baselineID,
			Limit:   2,
		})
		require.NoError(t, err)
		require.True(t, firstPage.HasMore)
		require.Equal(t, toolOnly.ID, firstPage.NextAfterID)
		require.Zero(t, firstPage.NextBeforeID)
		require.Len(t, firstPage.Messages, 1)
		require.Equal(t, first.ID, firstPage.Messages[0].ID)

		secondPage, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{
			ChatID:  chat.ID.String(),
			AfterID: firstPage.NextAfterID,
			Limit:   2,
		})
		require.NoError(t, err)
		require.False(t, secondPage.HasMore)
		require.Equal(t, last.ID, secondPage.NextAfterID)
		require.Len(t, secondPage.Messages, 1)
		require.Equal(t, last.ID, secondPage.Messages[0].ID)
		require.Greater(t, secondPage.Messages[0].ID, firstPage.Messages[0].ID)

		finalPage, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{
			ChatID:  chat.ID.String(),
			AfterID: secondPage.NextAfterID,
			Limit:   2,
		})
		require.NoError(t, err)
		require.False(t, finalPage.HasMore)
		require.Zero(t, finalPage.NextAfterID)
		require.Empty(t, finalPage.Messages)
	})

	t.Run("ListChatsByLabel", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		labelValue := uuid.NewString()
		matching, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Matching chat."}},
			Labels:         map[string]string{"uat-evidence": labelValue},
		})
		require.NoError(t, err)
		nonmatching, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Nonmatching chat."}},
			Labels:         map[string]string{"uat-evidence": uuid.NewString()},
		})
		require.NoError(t, err)

		result, err := testTool(t, toolsdk.ListChats, tb, toolsdk.ListChatsArgs{
			Labels: map[string]string{"uat-evidence": labelValue},
			Limit:  100,
		})
		require.NoError(t, err)
		require.Len(t, result.Chats, 1)
		require.Equal(t, matching.ID.String(), result.Chats[0].ID)
		require.Equal(t, map[string]string{"uat-evidence": labelValue}, result.Chats[0].Labels)

		coderdtest.WaitForChatSettled(ctx, t, api, matching.ID)
		coderdtest.WaitForChatSettled(ctx, t, api, nonmatching.ID)
	})

	t.Run("AwaitChat", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		settled, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Settle immediately."}},
		})
		require.NoError(t, err)
		coderdtest.WaitForChatSettled(ctx, t, api, settled.ID)
		immediate, err := testTool(t, toolsdk.AwaitChat, tb, toolsdk.AwaitChatArgs{ChatID: settled.ID.String()})
		require.NoError(t, err)
		require.False(t, immediate.TimedOut)
		require.Equal(t, codersdk.ChatStatusWaiting, immediate.Chat.Status)

		streamStarted := make(chan struct{})
		providerRelease := make(chan struct{})
		var providerStartedOnce sync.Once
		var providerReleaseOnce sync.Once
		releaseProvider := func() { providerReleaseOnce.Do(func() { close(providerRelease) }) }
		t.Cleanup(releaseProvider)
		blockingURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if req.Stream {
				providerStartedOnce.Do(func() { close(streamStarted) })
				select {
				case <-providerRelease:
				case <-req.Context().Done():
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Released.")...)
			}
			return chattest.OpenAINonStreamingResponse(`{"title": "Await Test"}`)
		})
		blockingModel := coderdtest.CreateOpenAICompatChatModel(t, expClient, blockingURL)
		awaitFile, err := expClient.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "await.txt", bytes.NewReader([]byte("await evidence")))
		require.NoError(t, err)
		running, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content: []codersdk.ChatInputPart{
				{Type: codersdk.ChatInputPartTypeText, Text: "Wait for release."},
				{Type: codersdk.ChatInputPartTypeFile, FileID: awaitFile.ID},
			},
			ModelConfigID: &blockingModel.ID,
		})
		require.NoError(t, err)
		select {
		case <-streamStarted:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}

		getSeen := make(chan struct{})
		getRelease := make(chan struct{})
		transport := &signalPathTransport{
			path:    "/api/v2/chats/" + running.ID.String(),
			seen:    getSeen,
			release: getRelease,
		}
		awaitClient := codersdk.New(client.URL)
		awaitClient.SetSessionToken(client.SessionToken())
		awaitClient.HTTPClient = &http.Client{Transport: transport}
		t.Cleanup(awaitClient.HTTPClient.CloseIdleConnections)
		awaitDeps, err := toolsdk.NewDeps(awaitClient)
		require.NoError(t, err)
		type awaitResult struct {
			response toolsdk.AwaitChatResponse
			err      error
		}
		result := make(chan awaitResult, 1)
		go func() {
			response, err := toolsdk.AwaitChat.Handler(ctx, awaitDeps, toolsdk.AwaitChatArgs{
				ChatID:   running.ID.String(),
				WaitSecs: 10,
			})
			result <- awaitResult{response: response, err: err}
		}()
		select {
		case <-getSeen:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		close(getRelease)
		releaseProvider()
		select {
		case awaited := <-result:
			require.NoError(t, awaited.err)
			require.False(t, awaited.response.TimedOut)
			require.Equal(t, codersdk.ChatStatusWaiting, awaited.response.Chat.Status)
			require.Len(t, awaited.response.Chat.Files, 1)
			require.Equal(t, awaitFile.ID.String(), awaited.response.Chat.Files[0].ID)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		coderdtest.WaitForChatSettled(ctx, t, api, running.ID)

		sharedClient, sharedUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		sharedStreamStarted := make(chan struct{})
		sharedProviderRelease := make(chan struct{})
		var sharedProviderStartedOnce sync.Once
		var sharedProviderReleaseOnce sync.Once
		releaseSharedProvider := func() { sharedProviderReleaseOnce.Do(func() { close(sharedProviderRelease) }) }
		t.Cleanup(releaseSharedProvider)
		sharedBlockingURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if req.Stream {
				sharedProviderStartedOnce.Do(func() { close(sharedStreamStarted) })
				select {
				case <-sharedProviderRelease:
				case <-req.Context().Done():
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Released.")...)
			}
			return chattest.OpenAINonStreamingResponse(`{"title": "Shared Await Test"}`)
		})
		sharedBlockingModel := coderdtest.CreateOpenAICompatChatModel(t, expClient, sharedBlockingURL)
		sharedRunning, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Wait for shared release."}},
			ModelConfigID:  &sharedBlockingModel.ID,
		})
		require.NoError(t, err)
		select {
		case <-sharedStreamStarted:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		err = expClient.UpdateChatACL(ctx, sharedRunning.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatRole{sharedUser.ID.String(): codersdk.ChatRoleRead},
		})
		require.NoError(t, err)
		acl, err := expClient.GetChatACL(ctx, sharedRunning.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 1)
		require.Equal(t, sharedUser.ID, acl.Users[0].ID)
		require.Equal(t, codersdk.ChatRoleRead, acl.Users[0].Role)

		sharedGetSeen := make(chan struct{})
		sharedGetRelease := make(chan struct{})
		sharedAwaitClient := codersdk.New(sharedClient.URL)
		sharedAwaitClient.SetSessionToken(sharedClient.SessionToken())
		sharedAwaitClient.HTTPClient = &http.Client{Transport: &signalPathTransport{
			path:    "/api/v2/chats/" + sharedRunning.ID.String(),
			seen:    sharedGetSeen,
			release: sharedGetRelease,
		}}
		t.Cleanup(sharedAwaitClient.HTTPClient.CloseIdleConnections)
		sharedAwaitDeps, err := toolsdk.NewDeps(sharedAwaitClient)
		require.NoError(t, err)
		sharedAwaitCtx := testutil.Context(t, testutil.WaitMedium)
		sharedResult := make(chan awaitResult, 1)
		go func() {
			response, err := toolsdk.AwaitChat.Handler(sharedAwaitCtx, sharedAwaitDeps, toolsdk.AwaitChatArgs{
				ChatID:   sharedRunning.ID.String(),
				WaitSecs: 20,
			})
			sharedResult <- awaitResult{response: response, err: err}
		}()
		select {
		case <-sharedGetSeen:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		close(sharedGetRelease)
		releaseSharedProvider()
		select {
		case awaited := <-sharedResult:
			require.NoError(t, awaited.err)
			require.False(t, awaited.response.TimedOut)
			require.Equal(t, codersdk.ChatStatusWaiting, awaited.response.Chat.Status)
		case <-sharedAwaitCtx.Done():
			t.Fatal(sharedAwaitCtx.Err())
		}
		coderdtest.WaitForChatSettled(ctx, t, api, sharedRunning.ID)

		timeoutStarted := make(chan struct{})
		timeoutRelease := make(chan struct{})
		var timeoutStartedOnce sync.Once
		var timeoutReleaseOnce sync.Once
		releaseTimeout := func() { timeoutReleaseOnce.Do(func() { close(timeoutRelease) }) }
		t.Cleanup(releaseTimeout)
		timeoutURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if req.Stream {
				timeoutStartedOnce.Do(func() { close(timeoutStarted) })
				select {
				case <-timeoutRelease:
				case <-req.Context().Done():
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Released.")...)
			}
			return chattest.OpenAINonStreamingResponse(`{"title": "Await Timeout Test"}`)
		})
		timeoutModel := coderdtest.CreateOpenAICompatChatModel(t, expClient, timeoutURL)
		busy, err := expClient.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "Stay busy."}},
			ModelConfigID:  &timeoutModel.ID,
		})
		require.NoError(t, err)
		select {
		case <-timeoutStarted:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		timedOut, err := testTool(t, toolsdk.AwaitChat, tb, toolsdk.AwaitChatArgs{
			ChatID:   busy.ID.String(),
			WaitSecs: 1,
		})
		require.NoError(t, err)
		require.True(t, timedOut.TimedOut)
		require.Equal(t, codersdk.ChatStatusRunning, timedOut.Chat.Status)
		releaseTimeout()
		coderdtest.WaitForChatSettled(ctx, t, api, busy.ID)

		// An initial status request that outlives the wait window
		// errors within the wait_secs bound; the old post-deadline
		// fallback fetch added up to 15 extra seconds.
		stallClient := codersdk.New(client.URL)
		stallClient.SetSessionToken(client.SessionToken())
		stallClient.HTTPClient = &http.Client{Transport: stallTransport{}}
		t.Cleanup(stallClient.HTTPClient.CloseIdleConnections)
		stallDeps, err := toolsdk.NewDeps(stallClient)
		require.NoError(t, err)
		stallStart := time.Now()
		_, err = toolsdk.AwaitChat.Handler(ctx, stallDeps, toolsdk.AwaitChatArgs{
			ChatID:   busy.ID.String(),
			WaitSecs: 1,
		})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Less(t, time.Since(stallStart), 10*time.Second)
	})

	t.Run("ListChatModelConfigsSkipsDisabledProviders", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		disabledProviderConfig := coderdtest.CreateOpenAICompatChatModel(t, expClient, chattest.OpenAI(t))
		provider, err := client.UpdateAIProvider(ctx, disabledProviderConfig.AIProviderID.String(), codersdk.UpdateAIProviderRequest{
			Enabled: new(false),
		})
		require.NoError(t, err)
		require.False(t, provider.Enabled)

		result, err := testTool(t, toolsdk.ListChatModelConfigs, tb, toolsdk.ListChatModelConfigsArgs{
			OrganizationID: firstUser.OrganizationID.String(),
		})
		require.NoError(t, err)
		var ids []string
		for _, config := range result.ModelConfigs {
			ids = append(ids, config.ID)
		}
		require.NotContains(t, ids, disabledProviderConfig.ID.String())
		require.Contains(t, ids, defaultModelConfig.ID.String())
	})

	t.Run("ListChatModelConfigsSkipsDeletedProviders", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		deletedProviderConfig := coderdtest.CreateOpenAICompatChatModel(t, expClient, chattest.OpenAI(t))
		err := client.DeleteAIProvider(ctx, deletedProviderConfig.AIProviderID.String())
		require.NoError(t, err)

		result, err := testTool(t, toolsdk.ListChatModelConfigs, tb, toolsdk.ListChatModelConfigsArgs{
			OrganizationID: firstUser.OrganizationID.String(),
		})
		require.NoError(t, err)
		var ids []string
		for _, config := range result.ModelConfigs {
			ids = append(ids, config.ID)
		}
		require.NotContains(t, ids, deletedProviderConfig.ID.String())
		require.Contains(t, ids, defaultModelConfig.ID.String())
	})

	t.Run("Interrupt", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		// Block the chat turn so the interrupt has a deterministic target.
		release := make(chan struct{})
		blockingURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
			if req.Stream {
				select {
				case <-release:
				case <-req.Context().Done():
				}
				return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("Released.")...)
			}
			return chattest.OpenAINonStreamingResponse(`{"title": "Interrupt Test"}`)
		})
		blockingModelConfig := coderdtest.CreateOpenAICompatChatModel(t, expClient, blockingURL)

		created, err := testTool(t, toolsdk.CreateChat, tb, toolsdk.CreateChatArgs{
			Prompt:         "Block forever.",
			OrganizationID: firstUser.OrganizationID.String(),
			ModelConfigID:  blockingModelConfig.ID.String(),
		})
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatStatusRunning, created.Status)

		sent, err := testTool(t, toolsdk.SendChatMessage, tb, toolsdk.SendChatMessageArgs{
			ChatID: created.ID,
			Text:   "Queued while busy.",
		})
		require.NoError(t, err)
		require.True(t, sent.Queued)
		// A queued prompt carrying only a file must surface in
		// queued_messages rather than being dropped.
		queuedFile, err := expClient.UploadChatFile(ctx, firstUser.OrganizationID, "text/plain", "queued-only.txt", bytes.NewReader([]byte("queued file")))
		require.NoError(t, err)
		_, err = expClient.CreateChatMessage(ctx, uuid.MustParse(created.ID), codersdk.CreateChatMessageRequest{
			Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeFile, FileID: queuedFile.ID}},
			BusyBehavior: codersdk.ChatBusyBehaviorQueue,
		})
		require.NoError(t, err)
		transcript, err := testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.Contains(t, transcript.QueuedMessages, "Queued while busy.")
		require.Contains(t, transcript.QueuedMessages, "(attached files: queued-only.txt)")

		interrupted, err := testTool(t, toolsdk.InterruptChat, tb, toolsdk.InterruptChatArgs{ChatID: created.ID})
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatStatusInterrupting, interrupted.Status)

		close(release)
		coderdtest.WaitForChatSettled(ctx, t, api, uuid.MustParse(created.ID))
	})

	t.Run("ListChatModelConfigsMemberAndAuditor", func(t *testing.T) {
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		memberDeps, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.ListChatModelConfigs, memberDeps, toolsdk.ListChatModelConfigsArgs{})
		require.NoError(t, err)
		var ids []string
		for _, config := range result.ModelConfigs {
			ids = append(ids, config.ID)
		}
		require.Contains(t, ids, defaultModelConfig.ID.String())

		auditorClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID, rbac.RoleAuditor())
		auditorDeps, err := toolsdk.NewDeps(auditorClient)
		require.NoError(t, err)
		auditorResult, err := testTool(t, toolsdk.ListChatModelConfigs, auditorDeps, toolsdk.ListChatModelConfigsArgs{})
		require.NoError(t, err)
		var auditorIDs []string
		for _, config := range auditorResult.ModelConfigs {
			auditorIDs = append(auditorIDs, config.ID)
		}
		require.Contains(t, auditorIDs, defaultModelConfig.ID.String())
	})

	t.Run("CreateChatUsesLastUpdatedOrganization", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		model := coderdtest.CreateOpenAICompatChatModel(t, expClient, "")
		organization := dbgen.Organization(t, api.Database, database.Organization{})
		dbgen.OrganizationMember(t, api.Database, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         firstUser.UserID,
		})
		defaultOrgChat := dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: model.ID,
		})
		dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    organization.ID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: model.ID,
		})

		err := expClient.UpdateChat(ctx, defaultOrgChat.ID, codersdk.UpdateChatRequest{
			Archived: new(true),
		})
		require.NoError(t, err)
		err = expClient.UpdateChat(ctx, defaultOrgChat.ID, codersdk.UpdateChatRequest{
			Archived: new(false),
		})
		require.NoError(t, err)

		created, err := testTool(t, toolsdk.CreateChat, tb, toolsdk.CreateChatArgs{
			Prompt:        "Reuse the last updated organization.",
			ModelConfigID: model.ID.String(),
		})
		require.NoError(t, err)
		chat, err := expClient.GetChat(ctx, uuid.MustParse(created.ID))
		require.NoError(t, err)
		require.Equal(t, firstUser.OrganizationID, chat.OrganizationID)
	})

	t.Run("CreateChatRequiresOrganizationForNewMultiOrgUser", func(t *testing.T) {
		organization := dbgen.Organization(t, api.Database, database.Organization{})
		memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		dbgen.OrganizationMember(t, api.Database, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         member.ID,
		})
		memberDeps, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)

		_, err = testTool(t, toolsdk.CreateChat, memberDeps, toolsdk.CreateChatArgs{Prompt: "hi"})
		require.ErrorContains(t, err, "belongs to multiple organizations")
		require.ErrorContains(t, err, "organization_id is required")
	})

	t.Run("ListChatModelConfigsUsesLastUpdatedOrganization", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		organization := dbgen.Organization(t, api.Database, database.Organization{})
		memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		dbgen.OrganizationMember(t, api.Database, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         member.ID,
		})

		provider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderTypeOpenAICompat,
			Name:    "recent-org-" + uuid.NewString(),
			BaseURL: chattest.OpenAI(t),
			Enabled: true,
			APIKeys: []string{"test-api-key"},
		})
		require.NoError(t, err)
		contextLimit := int64(4096)
		model, err := expClient.CreateChatModel(ctx, organization.ID, codersdk.CreateChatModelRequest{
			AIProviderID: &provider.ID,
			Model:        "gpt-4o-recent-org",
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)
		dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    organization.ID,
			OwnerID:           member.ID,
			LastModelConfigID: model.ID,
		})

		memberDeps, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.ListChatModelConfigs, memberDeps, toolsdk.ListChatModelConfigsArgs{})
		require.NoError(t, err)
		require.Equal(t, organization.ID.String(), result.OrganizationID)
		require.Len(t, result.ModelConfigs, 1)
		require.Equal(t, model.ID.String(), result.ModelConfigs[0].ID)
	})

	t.Run("ListChatModelConfigsRequiresOrganizationForNewMultiOrgUser", func(t *testing.T) {
		organization := dbgen.Organization(t, api.Database, database.Organization{})
		memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		dbgen.OrganizationMember(t, api.Database, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         member.ID,
		})
		memberDeps, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)

		_, err = testTool(t, toolsdk.ListChatModelConfigs, memberDeps, toolsdk.ListChatModelConfigsArgs{})
		require.ErrorContains(t, err, "belongs to multiple organizations")
		require.ErrorContains(t, err, "organization_id is required")
	})

	t.Run("DefaultOrganizationAllChatsArchived", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		organization := dbgen.Organization(t, api.Database, database.Organization{})
		memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		dbgen.OrganizationMember(t, api.Database, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         member.ID,
		})
		chat := dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    organization.ID,
			OwnerID:           member.ID,
			LastModelConfigID: defaultModelConfig.ID,
		})
		memberExpClient := codersdk.NewExperimentalClient(memberClient)
		err := memberExpClient.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{
			Archived: new(true),
		})
		require.NoError(t, err)

		memberDeps, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.ListChatModelConfigs, memberDeps, toolsdk.ListChatModelConfigsArgs{})
		require.NoError(t, err)
		require.Equal(t, organization.ID.String(), result.OrganizationID)
	})

	t.Run("DefaultOrganizationPrefersNewerArchivedChat", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		organization := dbgen.Organization(t, api.Database, database.Organization{})
		memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		dbgen.OrganizationMember(t, api.Database, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         member.ID,
		})
		activeChat := dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           member.ID,
			LastModelConfigID: defaultModelConfig.ID,
		})
		archivedChat := dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    organization.ID,
			OwnerID:           member.ID,
			LastModelConfigID: defaultModelConfig.ID,
		})
		require.True(t, archivedChat.UpdatedAt.After(activeChat.UpdatedAt))
		// Archiving bumps updated_at, so the archived chat stays newest.
		memberExpClient := codersdk.NewExperimentalClient(memberClient)
		err := memberExpClient.UpdateChat(ctx, archivedChat.ID, codersdk.UpdateChatRequest{
			Archived: new(true),
		})
		require.NoError(t, err)

		memberDeps, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.ListChatModelConfigs, memberDeps, toolsdk.ListChatModelConfigsArgs{})
		require.NoError(t, err)
		require.Equal(t, organization.ID.String(), result.OrganizationID)
	})

	t.Run("DefaultOrganizationConsidersChildChats", func(t *testing.T) {
		organization := dbgen.Organization(t, api.Database, database.Organization{})
		memberClient, member := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		dbgen.OrganizationMember(t, api.Database, database.OrganizationMember{
			OrganizationID: organization.ID,
			UserID:         member.ID,
		})
		childOrgRoot := dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    organization.ID,
			OwnerID:           member.ID,
			LastModelConfigID: defaultModelConfig.ID,
		})
		rootOrgChat := dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           member.ID,
			LastModelConfigID: defaultModelConfig.ID,
		})
		child := dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    organization.ID,
			OwnerID:           member.ID,
			LastModelConfigID: defaultModelConfig.ID,
			ParentChatID:      uuid.NullUUID{UUID: childOrgRoot.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: childOrgRoot.ID, Valid: true},
		})
		// The child must be the newest chat while its root stays older than
		// the other organization's root, or the scenario is not exercised.
		require.True(t, child.UpdatedAt.After(rootOrgChat.UpdatedAt))
		require.True(t, rootOrgChat.UpdatedAt.After(childOrgRoot.UpdatedAt))

		memberDeps, err := toolsdk.NewDeps(memberClient)
		require.NoError(t, err)
		result, err := testTool(t, toolsdk.ListChatModelConfigs, memberDeps, toolsdk.ListChatModelConfigsArgs{})
		require.NoError(t, err)
		require.Equal(t, organization.ID.String(), result.OrganizationID)
	})

	t.Run("CreateChatZeroOrgUser", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)

		orphanClient, orphan := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		require.NoError(t, client.DeleteOrganizationMember(ctx, firstUser.OrganizationID, orphan.ID.String()))

		orphanDeps, err := toolsdk.NewDeps(orphanClient)
		require.NoError(t, err)
		_, err = testTool(t, toolsdk.CreateChat, orphanDeps, toolsdk.CreateChatArgs{Prompt: "hi"})
		require.ErrorContains(t, err, "belongs to no organization")
	})

	t.Run("Validation", func(t *testing.T) {
		_, err := testTool(t, toolsdk.CreateChat, tb, toolsdk.CreateChatArgs{})
		require.ErrorContains(t, err, "prompt is required")

		_, err = testTool(t, toolsdk.ListChatModelConfigs, tb, toolsdk.ListChatModelConfigsArgs{OrganizationID: "not-a-uuid"})
		require.ErrorContains(t, err, "organization_id must be a valid UUID")

		_, err = testTool(t, toolsdk.GetChat, tb, toolsdk.GetChatArgs{ChatID: "not-a-uuid"})
		require.ErrorContains(t, err, "chat_id must be a valid UUID")

		_, err = testTool(t, toolsdk.SendChatMessage, tb, toolsdk.SendChatMessageArgs{
			ChatID:       uuid.NewString(),
			Text:         "hi",
			BusyBehavior: codersdk.ChatBusyBehavior("bogus"),
		})
		require.ErrorContains(t, err, "busy_behavior")

		_, err = testTool(t, toolsdk.DownloadChatFile, tb, toolsdk.DownloadChatFileArgs{})
		require.ErrorContains(t, err, "exactly one addressing mode")

		_, err = testTool(t, toolsdk.AwaitChat, tb, toolsdk.AwaitChatArgs{ChatID: "not-a-uuid"})
		require.ErrorContains(t, err, "chat_id must be a valid UUID")

		listed, err := testTool(t, toolsdk.ListChats, tb, toolsdk.ListChatsArgs{Limit: 1})
		require.NoError(t, err)
		require.LessOrEqual(t, len(listed.Chats), 1)

		_, err = testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{
			ChatID:   uuid.NewString(),
			BeforeID: 1,
			AfterID:  2,
		})
		require.ErrorContains(t, err, "before_id and after_id cannot be used together")

		for _, limit := range []int{-1, 201} {
			_, err = testTool(t, toolsdk.GetChatMessages, tb, toolsdk.GetChatMessagesArgs{
				ChatID: uuid.NewString(),
				Limit:  limit,
			})
			require.ErrorContains(t, err, "limit must be between 1 and 200")
		}
	})
}
