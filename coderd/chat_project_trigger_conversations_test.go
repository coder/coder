package coderd_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func decodeTriggerWebhookResponse(t *testing.T, status int, body []byte) codersdk.ChatProjectTriggerWebhookResponse {
	t.Helper()
	require.Equal(t, http.StatusAccepted, status, string(body))
	var resp codersdk.ChatProjectTriggerWebhookResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}

func TestChatProjectTriggerSlackConversations(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)
	project := createChatProject(t, client, orgID, "Helper")

	created, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:   "Mentions",
		Source: codersdk.ChatProjectTriggerSourceSlack,
		Events: []string{codersdk.ChatProjectTriggerSlackEventAppMention},
	})
	require.NoError(t, err)
	// Slack triggers continue threads by default.
	require.True(t, created.Trigger.ContinueConversations)
	require.Equal(t, codersdk.ChatProjectTriggerBusyBehaviorQueue, created.Trigger.BusyBehavior)

	post := func(event string, headers map[string]string) (int, []byte) {
		return postTriggerWebhook(ctx, t, created.WebhookURL, "application/json",
			`{"type":"event_callback","team_id":"T1","authorizations":[{"user_id":"UBOT","is_bot":true}],`+event+`}`, headers)
	}

	status, body := post(`"event_id":"Ev1","event":{"type":"app_mention","text":"<@UBOT> why is CI red?","user":"U1","channel":"C1","ts":"1.0"}`, nil)
	started := decodeTriggerWebhookResponse(t, status, body)
	require.False(t, started.Continued)

	// The built-in Slack tools find the thread from the chat.
	//nolint:gocritic // Reads the mapping the way chatd tools would.
	conversation, err := db.GetChatProjectTriggerConversationByChatID(dbauthz.AsSystemRestricted(ctx), started.ChatID)
	require.NoError(t, err)
	require.Equal(t, "T1/C1/1.0", conversation.ConversationKey)
	require.Equal(t, string(codersdk.ChatProjectTriggerSourceSlack), conversation.Source)

	// A reply in the thread arrives as a message event, which the trigger
	// does not start chats for, and continues the thread's chat.
	reply := `"event_id":"Ev2","event":{"type":"message","text":"it fails on lint","user":"U2","channel":"C1","ts":"2.0","thread_ts":"1.0"}`
	status, body = post(reply, nil)
	continued := decodeTriggerWebhookResponse(t, status, body)
	require.True(t, continued.Continued)
	require.Equal(t, started.ChatID, continued.ChatID)

	// The chat is still busy with the first event, so the reply waits in
	// the queue.
	messages, err := client.GetChatMessages(ctx, started.ChatID, nil)
	require.NoError(t, err)
	require.Len(t, messages.QueuedMessages, 1)

	// Slack retries are handled once and are not recorded, so the
	// trigger's last delivery still shows the reply that continued the
	// chat.
	status, body = post(reply, map[string]string{"X-Slack-Retry-Num": "1"})
	require.Equal(t, http.StatusOK, status, string(body))
	require.Contains(t, string(body), "already handled")
	status, body = post(`"event":{"type":"message","text":"it fails on lint","user":"U2","channel":"C1","ts":"2.0","thread_ts":"1.0"}`, map[string]string{"X-Slack-Retry-Num": "2"})
	require.Equal(t, http.StatusOK, status, string(body))
	require.Contains(t, string(body), "without an event ID")
	deliveries, err := client.ChatProjectTriggerDeliveries(ctx, orgID, project.ID, created.Trigger.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 2)
	require.Equal(t, codersdk.ChatProjectTriggerDeliveryStatusContinued, deliveries[0].Status)
	triggers, err := client.ListChatProjectTriggers(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Equal(t, deliveries[0].ID, triggers[0].LastDelivery.ID)

	// The app's own messages and other bots never continue a chat.
	for _, event := range []string{
		`"event_id":"Ev3","event":{"type":"message","text":"Looking into it","user":"UBOT","channel":"C1","ts":"3.0","thread_ts":"1.0"}`,
		`"event_id":"Ev4","event":{"type":"message","text":"beep","bot_id":"B2","channel":"C1","ts":"4.0","thread_ts":"1.0"}`,
		`"event_id":"Ev5","event":{"type":"message","subtype":"bot_message","text":"beep","channel":"C1","ts":"5.0","thread_ts":"1.0"}`,
	} {
		status, body = post(event, nil)
		require.Equal(t, http.StatusOK, status, string(body))
		require.Contains(t, string(body), "Event ignored", event)
	}

	// Replies in threads the agent has no chat for are ignored unless they
	// mention the app.
	status, body = post(`"event_id":"Ev6","event":{"type":"message","text":"unrelated","user":"U1","channel":"C1","ts":"7.0","thread_ts":"6.0"}`, nil)
	require.Equal(t, http.StatusOK, status, string(body))
	require.Contains(t, string(body), "has no chat for")
	status, body = post(`"event_id":"Ev7","event":{"type":"app_mention","text":"<@UBOT> help here","user":"U1","channel":"C1","ts":"8.0","thread_ts":"6.0"}`, nil)
	other := decodeTriggerWebhookResponse(t, status, body)
	require.False(t, other.Continued)
	require.NotEqual(t, started.ChatID, other.ChatID)

	runs, err := client.ListChatProjectRuns(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Len(t, runs, 2)

	deliveries, err = client.ChatProjectTriggerDeliveries(ctx, orgID, project.ID, created.Trigger.ID)
	require.NoError(t, err)
	var continuedDelivery *codersdk.ChatProjectTriggerDelivery
	for i := range deliveries {
		if deliveries[i].Status == codersdk.ChatProjectTriggerDeliveryStatusContinued {
			continuedDelivery = &deliveries[i]
		}
	}
	require.NotNil(t, continuedDelivery)
	require.Equal(t, &started.ChatID, continuedDelivery.ChatID)

	// Archiving the chat starts a new one for the thread.
	// The chat never finishes here because no worker runs, so it is
	// archived directly.
	//nolint:gocritic // Test setup.
	_, err = db.ArchiveChatByID(dbauthz.AsSystemRestricted(ctx), started.ChatID)
	require.NoError(t, err)
	status, body = post(`"event_id":"Ev8","event":{"type":"app_mention","text":"<@UBOT> still red","user":"U1","channel":"C1","ts":"9.0","thread_ts":"1.0"}`, nil)
	restarted := decodeTriggerWebhookResponse(t, status, body)
	require.False(t, restarted.Continued)
	require.NotEqual(t, started.ChatID, restarted.ChatID)
}

func TestChatProjectTriggerConversationOptions(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)
	project := createChatProject(t, client, orgID, "Webhooks")

	_, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:         "Bad busy behavior",
		BusyBehavior: "wait",
	})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())

	// Other sources start a chat per call unless asked to continue.
	plain, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{Name: "Plain"})
	require.NoError(t, err)
	require.False(t, plain.Trigger.ContinueConversations)

	enabled := true
	created, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:                  "Conversations",
		ContinueConversations: &enabled,
	})
	require.NoError(t, err)
	require.True(t, created.Trigger.ContinueConversations)

	post := func(conversation, body string) (int, []byte) {
		headers := map[string]string{}
		if conversation != "" {
			headers[codersdk.ChatProjectTriggerConversationHeader] = conversation
		}
		return postTriggerWebhook(ctx, t, created.WebhookURL, "application/json", body, headers)
	}
	postAccepted := func(conversation, body string) codersdk.ChatProjectTriggerWebhookResponse {
		status, data := post(conversation, body)
		return decodeTriggerWebhookResponse(t, status, data)
	}

	first := postAccepted("deploy-42", `{"step":"build"}`)
	second := postAccepted("deploy-42", `{"step":"test"}`)
	require.True(t, second.Continued)
	require.Equal(t, first.ChatID, second.ChatID)
	text := triggerChatUserText(ctx, t, client, first.ChatID)
	require.Contains(t, text, `"step": "build"`)

	// Queued: the follow-up waits behind the running turn.
	messages, err := client.GetChatMessages(ctx, first.ChatID, nil)
	require.NoError(t, err)
	require.Len(t, messages.QueuedMessages, 1)
	require.Contains(t, messages.QueuedMessages[0].Content[0].Text, "called again in this conversation")
	require.Contains(t, messages.QueuedMessages[0].Content[0].Text, `"step": "test"`)

	// Interrupt: the follow-up runs next instead of waiting.
	interrupt := codersdk.ChatProjectTriggerBusyBehaviorInterrupt
	updated, err := client.UpdateChatProjectTrigger(ctx, orgID, project.ID, created.Trigger.ID, codersdk.UpdateChatProjectTriggerRequest{
		BusyBehavior: &interrupt,
	})
	require.NoError(t, err)
	require.Equal(t, interrupt, updated.BusyBehavior)
	third := postAccepted("deploy-42", `{"step":"deploy"}`)
	require.Equal(t, first.ChatID, third.ChatID)
	messages, err = client.GetChatMessages(ctx, first.ChatID, nil)
	require.NoError(t, err)
	require.Len(t, messages.QueuedMessages, 1, "an interrupting message does not join the queue")
	require.Contains(t, triggerChatUserText(ctx, t, client, first.ChatID), `"step": "deploy"`)

	// Calls without the header, or with another ID, start new chats.
	fourth := postAccepted("", `{}`)
	require.False(t, fourth.Continued)
	fifth := postAccepted("deploy-43", `{}`)
	require.False(t, fifth.Continued)
	require.NotEqual(t, first.ChatID, fifth.ChatID)

	// Concurrent first events in one conversation share one chat.
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		chats = map[uuid.UUID]int{}
	)
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, body := post("race", fmt.Sprintf(`{"n":%d}`, i))
			if status != http.StatusAccepted {
				t.Errorf("unexpected status %d: %s", status, body)
				return
			}
			var resp codersdk.ChatProjectTriggerWebhookResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Errorf("decode: %v", err)
				return
			}
			mu.Lock()
			chats[resp.ChatID]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	require.Len(t, chats, 1)

	// Turning conversations off starts a chat per call again.
	disabled := false
	_, err = client.UpdateChatProjectTrigger(ctx, orgID, project.ID, created.Trigger.ID, codersdk.UpdateChatProjectTriggerRequest{
		ContinueConversations: &disabled,
	})
	require.NoError(t, err)
	sixth := postAccepted("deploy-42", `{}`)
	require.False(t, sixth.Continued)
	require.NotEqual(t, first.ChatID, sixth.ChatID)
}

func TestChatProjectTriggerGitHubConversations(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)
	project := createChatProject(t, client, orgID, "Triage")

	enabled := true
	created, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:                  "Issues",
		Prompt:                "Triage the issue.",
		Source:                codersdk.ChatProjectTriggerSourceGitHub,
		Events:                []string{"issues.opened", "issue_comment.created"},
		ContinueConversations: &enabled,
	})
	require.NoError(t, err)
	post := func(event, body string) (int, []byte) {
		return postTriggerWebhook(ctx, t, created.WebhookURL, "application/json", body, map[string]string{"X-GitHub-Event": event})
	}
	postAccepted := func(event, body string) codersdk.ChatProjectTriggerWebhookResponse {
		status, data := post(event, body)
		return decodeTriggerWebhookResponse(t, status, data)
	}

	opened := postAccepted("issues", `{"action":"opened","repository":{"full_name":"Coder/Coder"},"issue":{"number":5,"title":"Flaky test"}}`)
	comment := postAccepted("issue_comment", `{"action":"created","repository":{"full_name":"coder/coder"},"issue":{"number":5,"title":"Flaky test"},"comment":{"body":"Still failing"}}`)
	require.True(t, comment.Continued)
	require.Equal(t, opened.ChatID, comment.ChatID)
	messages, err := client.GetChatMessages(ctx, opened.ChatID, nil)
	require.NoError(t, err)
	require.Len(t, messages.QueuedMessages, 1)
	followUp := messages.QueuedMessages[0].Content[0].Text
	require.Contains(t, followUp, "A new event arrived in this conversation")
	require.Contains(t, followUp, "Still failing")
	require.NotContains(t, followUp, "Triage the issue.", "the trigger prompt is not repeated")

	other := postAccepted("issue_comment", `{"action":"created","repository":{"full_name":"coder/coder"},"issue":{"number":6,"title":"Other"},"comment":{"body":"hi"}}`)
	require.False(t, other.Continued)
	require.NotEqual(t, opened.ChatID, other.ChatID)
}

func signSlackRequest(secret, timestamp, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + timestamp + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func TestChatProjectTriggerSlackSigningSecret(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)
	project := createChatProject(t, client, orgID, "Signed")

	_, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:               "Webhook with secret",
		SlackSigningSecret: "s3cret",
	})
	require.Equal(t, http.StatusBadRequest, coderdtest.SDKError(t, err).StatusCode())

	const secret = "8f742231b10e8888abcd99yyyzzz85a5"
	created, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:               "Signed mentions",
		Source:             codersdk.ChatProjectTriggerSourceSlack,
		SlackSigningSecret: " " + secret + " ",
	})
	require.NoError(t, err)
	require.True(t, created.Trigger.HasSlackSigningSecret)
	// The secret is never returned.
	raw, err := json.Marshal(created)
	require.NoError(t, err)
	require.NotContains(t, string(raw), secret)

	verification := `{"type":"url_verification","challenge":"abc"}`
	post := func(timestamp, signature string) (int, []byte) {
		headers := map[string]string{}
		if timestamp != "" {
			headers["X-Slack-Request-Timestamp"] = timestamp
		}
		if signature != "" {
			headers["X-Slack-Signature"] = signature
		}
		return postTriggerWebhook(ctx, t, created.WebhookURL, "application/json", verification, headers)
	}
	now := strconv.FormatInt(time.Now().Unix(), 10)
	stale := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)

	status, _ := post("", "")
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = post(now, signSlackRequest("wrong", now, verification))
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = post(stale, signSlackRequest(secret, stale, verification))
	require.Equal(t, http.StatusUnauthorized, status)
	status, body := post(now, signSlackRequest(secret, now, verification))
	require.Equal(t, http.StatusOK, status, string(body))
	require.JSONEq(t, `{"challenge":"abc"}`, string(body))

	deliveries, err := client.ChatProjectTriggerDeliveries(ctx, orgID, project.ID, created.Trigger.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 4)
	require.Equal(t, codersdk.ChatProjectTriggerDeliveryStatusHandshake, deliveries[0].Status)
	require.Equal(t, codersdk.ChatProjectTriggerDeliveryStatusRejected, deliveries[1].Status)
	require.Contains(t, deliveries[1].Message, "Slack signature does not match")

	//nolint:gocritic // Checks the stored row directly.
	stored, err := db.GetChatProjectTriggerByID(dbauthz.AsSystemRestricted(ctx), created.Trigger.ID)
	require.NoError(t, err)
	require.Equal(t, secret, stored.SlackSigningSecret)

	// Removing the secret turns verification off.
	empty := ""
	updated, err := client.UpdateChatProjectTrigger(ctx, orgID, project.ID, created.Trigger.ID, codersdk.UpdateChatProjectTriggerRequest{
		SlackSigningSecret: &empty,
	})
	require.NoError(t, err)
	require.False(t, updated.HasSlackSigningSecret)
	status, _ = post("", "")
	require.Equal(t, http.StatusOK, status)
}

func signGitHubRequest(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestChatProjectTriggerGitHubWebhookSecret(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)
	project := createChatProject(t, client, orgID, "Signed")

	_, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:                "Slack with GitHub secret",
		Source:              codersdk.ChatProjectTriggerSourceSlack,
		GitHubWebhookSecret: "s3cret",
	})
	requireChatProjectFieldError(t, err, "github_webhook_secret", "Only GitHub triggers have a webhook secret.")

	//nolint:gosec // Test fixture, not a real credential.
	const secret = "github-webhook-secret"
	created, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:                "Signed issues",
		Source:              codersdk.ChatProjectTriggerSourceGitHub,
		Events:              []string{"issues.opened"},
		GitHubWebhookSecret: secret,
	})
	require.NoError(t, err)
	require.True(t, created.Trigger.HasGitHubWebhookSecret)
	require.False(t, created.Trigger.HasSlackSigningSecret)
	raw, err := json.Marshal(created)
	require.NoError(t, err)
	require.NotContains(t, string(raw), secret)

	body := `{"action":"opened","repository":{"full_name":"coder/coder"},"issue":{"number":7,"title":"Flaky test"},"sender":{"login":"octocat"}}`
	post := func(delivery, signature string) (int, []byte) {
		headers := map[string]string{"X-GitHub-Event": "issues", "X-GitHub-Delivery": delivery}
		if signature != "" {
			headers["X-Hub-Signature-256"] = signature
		}
		return postTriggerWebhook(ctx, t, created.WebhookURL, "application/json", body, headers)
	}

	status, _ := post("d-1", "")
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = post("d-1", signGitHubRequest("wrong", body))
	require.Equal(t, http.StatusUnauthorized, status)
	status, resp := post("d-1", signGitHubRequest(secret, body))
	started := decodeTriggerWebhookResponse(t, status, resp)

	// GitHub redelivers with the same delivery ID; it is handled once and
	// not recorded.
	status, resp = post("d-1", signGitHubRequest(secret, body))
	require.Equal(t, http.StatusOK, status, string(resp))
	require.Contains(t, string(resp), "already handled")
	status, resp = post("not a delivery id", signGitHubRequest(secret, body))
	require.Equal(t, http.StatusOK, status, string(resp))
	require.Contains(t, string(resp), "not a valid delivery ID")

	deliveries, err := client.ChatProjectTriggerDeliveries(ctx, orgID, project.ID, created.Trigger.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 4)
	require.Equal(t, codersdk.ChatProjectTriggerDeliveryStatusIgnored, deliveries[0].Status)
	require.Equal(t, codersdk.ChatProjectTriggerDeliveryStatusStarted, deliveries[1].Status)
	require.Equal(t, &started.ChatID, deliveries[1].ChatID)
	require.Equal(t, codersdk.ChatProjectTriggerDeliveryStatusRejected, deliveries[2].Status)
	require.Contains(t, deliveries[2].Message, "GitHub signature does not match")

	//nolint:gocritic // Checks the stored row directly.
	stored, err := db.GetChatProjectTriggerByID(dbauthz.AsSystemRestricted(ctx), created.Trigger.ID)
	require.NoError(t, err)
	require.Equal(t, secret, stored.GitHubWebhookSecret)

	// Removing the secret turns verification off.
	empty := ""
	updated, err := client.UpdateChatProjectTrigger(ctx, orgID, project.ID, created.Trigger.ID, codersdk.UpdateChatProjectTriggerRequest{
		GitHubWebhookSecret: &empty,
	})
	require.NoError(t, err)
	require.False(t, updated.HasGitHubWebhookSecret)
	status, resp = post("d-2", "")
	require.Equal(t, http.StatusAccepted, status, string(resp))
}

func TestChatProjectTriggerMalformedIDs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	_ = createChatModel(t, client)
	project := createChatProject(t, client, orgID, "Strict")

	slack, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:   "Mentions",
		Source: codersdk.ChatProjectTriggerSourceSlack,
		Events: []string{codersdk.ChatProjectTriggerSlackEventAppMention, codersdk.ChatProjectTriggerSlackEventSlashCommand},
	})
	require.NoError(t, err)
	mention := func(team, channel, ts string) string {
		return fmt.Sprintf(`{"type":"event_callback","team_id":%q,"event_id":"Ev1","event":{"type":"app_mention","text":"hi","user":"U1","channel":%q,"ts":%q}}`, team, channel, ts)
	}
	for _, payload := range []string{
		mention("T1/../x", "C1", "1.0"),
		mention("T1", "c1; drop", "1.0"),
		mention("T1", "C1", "1.0/../2"),
		mention("T1", "C1", "123456789012345.1"),
		`{"type":"event_callback","team_id":"T1","event_id":"Ev/1","event":{"type":"app_mention","text":"hi","user":"U1","channel":"C1","ts":"1.0"}}`,
	} {
		status, body := postTriggerWebhook(ctx, t, slack.WebhookURL, "application/json", payload, nil)
		require.Equal(t, http.StatusOK, status, payload)
		require.Contains(t, string(body), "malformed", payload)
	}

	// Slash commands are handled once per trigger_id.
	form := url.Values{"command": {"/ask"}, "text": {"why"}, "user_id": {"U1"}, "channel_id": {"C1"}, "team_id": {"T1"}, "trigger_id": {"1334.7384.808d"}}.Encode()
	status, body := postTriggerWebhook(ctx, t, slack.WebhookURL, "application/x-www-form-urlencoded", form, nil)
	require.Equal(t, http.StatusOK, status, string(body))
	require.Contains(t, string(body), "Started a chat")
	status, body = postTriggerWebhook(ctx, t, slack.WebhookURL, "application/x-www-form-urlencoded", form, nil)
	require.Equal(t, http.StatusOK, status, string(body))
	require.Contains(t, string(body), "already handled")
	runs, err := client.ListChatProjectRuns(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)

	github, err := client.CreateChatProjectTrigger(ctx, orgID, project.ID, codersdk.CreateChatProjectTriggerRequest{
		Name:   "Issues",
		Source: codersdk.ChatProjectTriggerSourceGitHub,
	})
	require.NoError(t, err)
	long := strings.Repeat("a", 101)
	for _, repo := range []string{"coder", "coder/coder/extra", "-coder/coder", "coder/" + long} {
		payload := fmt.Sprintf(`{"action":"opened","repository":{"full_name":%q},"issue":{"number":1}}`, repo)
		status, body := postTriggerWebhook(ctx, t, github.WebhookURL, "application/json", payload, map[string]string{"X-GitHub-Event": "issues"})
		require.Equal(t, http.StatusOK, status, repo)
		require.Contains(t, string(body), "not a valid owner/name", repo)
	}
}
