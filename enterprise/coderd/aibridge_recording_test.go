package coderd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

// aiGatewayChatCompletion is the request sent through the embedded gateway. The
// prompt is asserted against what was, or was not, recorded.
const aiGatewayChatCompletion = `{"messages":[{"role":"user","content":"why is the sky blue?"}],"model":"gpt-4.1","tools":[{"type":"function","function":{"name":"read_file","description":"Read a file.","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}]}`

// aiGatewayUpstreamCompletion is the fixed upstream response. It carries a tool
// call and token usage, so one request produces an interception, a prompt, a
// token usage and a tool usage record.
const aiGatewayUpstreamCompletion = `{
  "id": "chatcmpl-embedded",
  "object": "chat.completion",
  "created": 1753343279,
  "model": "gpt-4.1",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_embedded",
            "type": "function",
            "function": {"name": "read_file", "arguments": "{\"path\":\"README.md\"}"}
          }
        ],
        "finish_reason": "tool_calls"
      },
      "finish_reason": "tool_calls"
    }
  ],
  "usage": {"prompt_tokens": 60, "completion_tokens": 15, "total_tokens": 75}
}`

// logSink collects log entries so a test can assert on the structured
// interception records the deployment emitted.
type logSink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *logSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
}

func (*logSink) Sync() {}

// interceptionRecords returns the record_type of every interception record
// logged, in order.
func (s *logSink) interceptionRecords(t *testing.T) []string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()

	var types []string
	for _, entry := range s.entries {
		if entry.Message != recorder.InterceptionLogMarker {
			continue
		}
		for _, field := range entry.Fields {
			if field.Name != "record_type" {
				continue
			}
			recordType, ok := field.Value.(string)
			require.True(t, ok, "record_type should be a string")
			types = append(types, recordType)
		}
	}
	return types
}

// contains reports whether any logged interception record carries value.
func (s *logSink) contains(value string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, entry := range s.entries {
		if entry.Message != recorder.InterceptionLogMarker {
			continue
		}
		encoded, err := json.Marshal(entry.Fields)
		if err != nil {
			continue
		}
		if bytes.Contains(encoded, []byte(value)) {
			return true
		}
	}
	return false
}

// aiGatewayDeployment is a coderd with the embedded AI Gateway running, a
// provider pointing at a mock upstream, and a user to make requests as.
type aiGatewayDeployment struct {
	db         database.Store
	userClient *codersdk.Client
	logs       *logSink
}

// startEmbeddedAIGateway boots coderd with the embedded gateway under the
// supplied AI Gateway configuration, through the same constructor cli/server.go
// uses, so a record policy that never reaches the serving pool is visible here.
func startEmbeddedAIGateway(ctx context.Context, t *testing.T, mutate func(*codersdk.AIBridgeConfig)) *aiGatewayDeployment {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(aiGatewayUpstreamCompletion))
	}))
	t.Cleanup(upstream.Close)

	dv := coderdtest.DeploymentValues(t)
	dv.AI.BridgeConfig.Enabled = serpent.Bool(true)
	mutate(&dv.AI.BridgeConfig)

	sink := &logSink{}
	db, ps := dbtestutil.NewDB(t)
	client, _, api, firstUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dv,
			Database:         db,
			Pubsub:           ps,
			Logger:           aiGatewayLoggerPtr(testutil.Logger(t).AppendSinks(sink)),
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureAIBridge: 1},
		},
	})

	//nolint:gocritic // Owner role is needed for provider management.
	_, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
		Type:    codersdk.AIProviderTypeOpenAI,
		Name:    "openai",
		Enabled: true,
		BaseURL: upstream.URL,
		APIKeys: []string{"sk-embedded"},
	})
	require.NoError(t, err)

	aibridgedtest.StartTestAIBridgeDaemon(ctx, t, api.AGPL, nil)

	userClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	return &aiGatewayDeployment{db: db, userClient: userClient, logs: sink}
}

func aiGatewayLoggerPtr(logger slog.Logger) *slog.Logger { return &logger }

// postChatCompletion sends a chat completion through the embedded gateway.
func (d *aiGatewayDeployment) postChatCompletion(ctx context.Context, t *testing.T) {
	t.Helper()

	url := d.userClient.URL.String() + "/api/v2/ai-gateway/openai/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(aiGatewayChatCompletion))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+d.userClient.SessionToken())
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// interception polls until the request's interception is recorded, and returns
// it. Records are written asynchronously from the request.
func (d *aiGatewayDeployment) interception(ctx context.Context, t *testing.T) database.AIBridgeInterception {
	t.Helper()

	var interceptions []database.AIBridgeInterception
	require.Eventually(t, func() bool {
		var err error
		interceptions, err = d.db.GetAIBridgeInterceptions(ctx)
		return err == nil && len(interceptions) == 1 && interceptions[0].EndedAt.Valid
	}, testutil.WaitLong, testutil.IntervalFast, "the interception should be recorded and ended")
	return interceptions[0]
}

// TestEmbeddedAIGatewayRecordsContentByDefault pins the default: a request
// through the embedded gateway records its content.
func TestEmbeddedAIGatewayRecordsContentByDefault(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	dep := startEmbeddedAIGateway(ctx, t, func(*codersdk.AIBridgeConfig) {})

	dep.postChatCompletion(ctx, t)
	intc := dep.interception(ctx, t)

	prompts, err := dep.db.GetAIBridgeUserPromptsByInterceptionID(ctx, intc.ID)
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	require.Equal(t, "why is the sky blue?", prompts[0].Prompt)

	tools, err := dep.db.GetAIBridgeToolUsagesByInterceptionID(ctx, intc.ID)
	require.NoError(t, err)
	require.Len(t, tools, 1)

	tokens, err := dep.db.GetAIBridgeTokenUsagesByInterceptionID(ctx, intc.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)

	// Structured logging is off by default, so nothing is exported.
	require.Empty(t, dep.logs.interceptionRecords(t))
}

// TestEmbeddedAIGatewayDisableContentRecording is the end-to-end guard for the
// option: content records are dropped, the records cost control depends on are
// kept, and the request still succeeds.
func TestEmbeddedAIGatewayDisableContentRecording(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	dep := startEmbeddedAIGateway(ctx, t, func(cfg *codersdk.AIBridgeConfig) {
		cfg.DisableContentRecording = serpent.Bool(true)
	})

	dep.postChatCompletion(ctx, t)
	intc := dep.interception(ctx, t)

	// Then: the records cost control depends on are untouched.
	tokens, err := dep.db.GetAIBridgeTokenUsagesByInterceptionID(ctx, intc.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.EqualValues(t, 60, tokens[0].InputTokens)
	require.EqualValues(t, 15, tokens[0].OutputTokens)

	// Then: no conversation content was recorded.
	prompts, err := dep.db.GetAIBridgeUserPromptsByInterceptionID(ctx, intc.ID)
	require.NoError(t, err)
	require.Empty(t, prompts, "prompts must not be recorded")

	tools, err := dep.db.GetAIBridgeToolUsagesByInterceptionID(ctx, intc.ID)
	require.NoError(t, err)
	require.Empty(t, tools, "tool usage must not be recorded")

	thoughts, err := dep.db.ListAIBridgeModelThoughtsByInterceptionIDs(ctx, []uuid.UUID{intc.ID})
	require.NoError(t, err)
	require.Empty(t, thoughts, "model thoughts must not be recorded")
}

// TestEmbeddedAIGatewayExportsDroppedContent covers the combination the docs
// recommend: content is kept out of the database but still exported, which only
// works if the gateway is the emitter.
func TestEmbeddedAIGatewayExportsDroppedContent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	dep := startEmbeddedAIGateway(ctx, t, func(cfg *codersdk.AIBridgeConfig) {
		cfg.DisableContentRecording = serpent.Bool(true)
		cfg.StructuredLogging = serpent.Bool(true)
		cfg.StructuredLoggingSource = string(codersdk.AIStructuredLoggingSourceGateway)
	})

	dep.postChatCompletion(ctx, t)
	intc := dep.interception(ctx, t)

	prompts, err := dep.db.GetAIBridgeUserPromptsByInterceptionID(ctx, intc.ID)
	require.NoError(t, err)
	require.Empty(t, prompts, "prompts must not be recorded")

	// Then: every record type reached the logs, including the dropped ones.
	require.Eventually(t, func() bool {
		types := dep.logs.interceptionRecords(t)
		for _, want := range []string{
			recorder.RecordTypeInterceptionStart,
			recorder.RecordTypeInterceptionEnd,
			recorder.RecordTypeTokenUsage,
			recorder.RecordTypePromptUsage,
			recorder.RecordTypeToolUsage,
		} {
			if !slicesContains(types, want) {
				return false
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast, "every record type should be logged, got %v", dep.logs.interceptionRecords(t))

	// Then: the prompt the database refused is in the log line instead.
	require.True(t, dep.logs.contains("why is the sky blue?"), "the dropped prompt should be exported")
}

// TestEmbeddedAIGatewayCoderdRemainsDefaultEmitter guards existing deployments:
// with structured logging on and no source configured, coderd emits and the
// gateway stays silent, so each record is reported once.
func TestEmbeddedAIGatewayCoderdRemainsDefaultEmitter(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	dep := startEmbeddedAIGateway(ctx, t, func(cfg *codersdk.AIBridgeConfig) {
		cfg.StructuredLogging = serpent.Bool(true)
	})

	dep.postChatCompletion(ctx, t)
	dep.interception(ctx, t)

	require.Eventually(t, func() bool {
		return slicesContains(dep.logs.interceptionRecords(t), recorder.RecordTypePromptUsage)
	}, testutil.WaitLong, testutil.IntervalFast, "coderd should emit the records")

	types := dep.logs.interceptionRecords(t)
	require.Equal(t, 1, countOf(types, recorder.RecordTypePromptUsage), "the prompt should be reported once: %v", types)
	require.Equal(t, 1, countOf(types, recorder.RecordTypeInterceptionStart), "the interception should be reported once: %v", types)
}

func slicesContains(values []string, want string) bool {
	return countOf(values, want) > 0
}

func countOf(values []string, want string) int {
	var count int
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}
