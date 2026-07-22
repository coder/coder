package chathooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

const (
	testSecret       = "test-hook-secret-32-bytes-minimum!!"
	testDeploymentID = "test-deployment"
	testVersion      = "test-version"
)

func TestDispatcherSuccess(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	event := newTestEvent(t, db, agenthooks.EventSessionStart, agenthooks.SessionStartData{Source: "new"})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "coderd/"+testVersion, r.Header.Get("User-Agent"))

		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte(testSecret))
		assert.NoError(t, err)
		assert.Equal(t, testDeploymentID, claims.Issuer)
		assert.Equal(t, serverURL(r), claims.Audience)
		assert.Equal(t, event.Type, claims.Type)
		chatID, err := claims.ChatID()
		assert.NoError(t, err)
		assert.Equal(t, event.ChatID, chatID)
		digest := sha256.Sum256(body)
		assert.Equal(t, hex.EncodeToString(digest[:]), claims.BodySHA256)
		assert.Equal(t, claims.IssuedAt-int64(clockSkewLeeway/time.Second), claims.NotBefore)

		var request agenthooks.Request
		assert.NoError(t, json.Unmarshal(body, &request))
		assert.Equal(t, claims.JTI, request.Meta.DispatchID)
		assert.Equal(t, agenthooks.SchemaVersion, request.Meta.SchemaVersion)
		decoded, err := request.Decode()
		assert.NoError(t, err)
		assert.Equal(t, &agenthooks.SessionStartData{Source: "new"}, decoded)

		rows, err := db.ListChatHookDispatchesByChatID(r.Context(), event.ChatID)
		assert.NoError(t, err)
		if assert.Len(t, rows, 1) {
			assert.Equal(t, "pending", rows[0].Result)
			assert.False(t, rows[0].FinishedAt.Valid)
		}
		_, err = w.Write([]byte(`{}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	dispatcher := newTestDispatcher(t, db, server.Client(), server.URL, 2*time.Second)
	response, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitLong), event)
	require.NoError(t, err)
	require.Equal(t, agenthooks.Response{}, response)

	row := singleDispatch(t, db, event.ChatID)
	require.Equal(t, resultOK, row.Result)
	require.True(t, row.FinishedAt.Valid)
	require.Equal(t, int32(http.StatusOK), row.HttpStatus.Int32)
	require.False(t, row.Error.Valid)
}

func TestDispatcherDeny(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	event := newTestEvent(t, db, agenthooks.EventUserPromptSubmit, agenthooks.UserPromptSubmitData{Prompt: "delete everything"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"permission":{"decision":"deny","reason":"blocked"},"user_message":"not allowed"}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	response, err := newTestDispatcher(t, db, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.NotNil(t, response.Permission)
	require.Equal(t, agenthooks.PermissionDeny, response.Permission.Decision)
	require.Equal(t, "not allowed", response.UserMessage)

	row := singleDispatch(t, db, event.ChatID)
	require.Equal(t, resultDenied, row.Result)
	require.Equal(t, string(agenthooks.PermissionDeny), row.Decision.String)
	require.Equal(t, "blocked", row.DecisionReason.String)
	require.Equal(t, "not allowed", row.UserMessage.String)
	require.JSONEq(t, `"delete everything"`, string(row.OriginalInput.RawMessage))
}

func TestDispatcherAllowInputOverride(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	toolInput := json.RawMessage(`{"path":"before"}`)
	toolUseID := "call_" + uuid.NewString()
	event := newTestEvent(t, db, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{
		ToolUseID: toolUseID,
		ToolName:  "edit",
		ToolInput: toolInput,
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"permission":{"decision":"allow","input_override":{"path":"after"}}}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	response, err := newTestDispatcher(t, db, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.NotNil(t, response.Permission)
	require.Equal(t, agenthooks.PermissionAllow, response.Permission.Decision)
	require.JSONEq(t, `{"path":"after"}`, string(response.Permission.InputOverride))

	row := singleDispatch(t, db, event.ChatID)
	require.Equal(t, resultOK, row.Result)
	require.Equal(t, toolUseID, row.ToolUseID.String)
	require.JSONEq(t, `{"path":"after"}`, string(row.InputOverride.RawMessage))
	require.JSONEq(t, `{"path":"before"}`, string(row.OriginalInput.RawMessage))
}

func TestDispatcherTimeoutNoRetry(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	event := newTestEvent(t, db, agenthooks.EventStop, agenthooks.StopData{})
	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
		assert.NoError(t, http.NewResponseController(w).Flush())
		<-release
	}))
	t.Cleanup(server.Close)

	_, err := newTestDispatcher(t, db, server.Client(), server.URL, 50*time.Millisecond).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	close(release)
	require.Error(t, err)
	require.Equal(t, int32(1), requests.Load())

	row := singleDispatch(t, db, event.ChatID)
	require.Equal(t, resultTimeout, row.Result)
	require.Equal(t, int32(http.StatusOK), row.HttpStatus.Int32)
	require.True(t, row.Error.Valid)
}

func TestDispatcherRetriesConnectionErrorWithSameJTI(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	event := newTestEvent(t, db, agenthooks.EventPostCompact, agenthooks.PostCompactData{})
	claimsCh := make(chan agenthooks.Claims, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte(testSecret))
		assert.NoError(t, err)
		claimsCh <- claims
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	var attempts atomic.Int32
	baseTransport := server.Client().Transport
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		claims, err := agenthooks.Verify(req.Header.Get("Authorization"), []byte(testSecret))
		if err != nil {
			return nil, err
		}
		if attempts.Add(1) == 1 {
			claimsCh <- claims
			_, err = io.Copy(io.Discard, req.Body)
			if err != nil {
				return nil, err
			}
			return nil, io.EOF
		}
		return baseTransport.RoundTrip(req)
	})}

	_, err := newTestDispatcher(t, db, client, server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.NoError(t, err)
	require.Equal(t, int32(2), attempts.Load())
	first := <-claimsCh
	second := <-claimsCh
	require.Equal(t, first.JTI, second.JTI)
	chatID, err := first.ChatID()
	require.NoError(t, err)
	require.Equal(t, event.ChatID, chatID)

	row := singleDispatch(t, db, event.ChatID)
	require.Equal(t, resultOK, row.Result)
	require.Equal(t, first.JTI, row.ID)
}

func TestDispatcherTLSFailureNoRetry(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	event := newTestEvent(t, db, agenthooks.EventStop, agenthooks.StopData{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request with an untrusted certificate reached the handler")
	}))
	t.Cleanup(server.Close)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	var attempts atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts.Add(1)
		return transport.RoundTrip(req)
	})}

	_, err := newTestDispatcher(t, db, client, server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.Error(t, err)
	require.Equal(t, int32(1), attempts.Load())

	row := singleDispatch(t, db, event.ChatID)
	require.Equal(t, resultProtocolError, row.Result)
}

func TestDispatcherNon2xxNoRetry(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	event := newTestEvent(t, db, agenthooks.EventPreCompact, agenthooks.PreCompactData{})
	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		assert.NoError(t, http.NewResponseController(w).Flush())
		<-release
	}))
	t.Cleanup(server.Close)

	_, err := newTestDispatcher(t, db, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	close(release)
	require.Error(t, err)
	require.Equal(t, int32(1), requests.Load())

	row := singleDispatch(t, db, event.ChatID)
	require.Equal(t, resultHTTPError, row.Result)
	require.Equal(t, int32(http.StatusServiceUnavailable), row.HttpStatus.Int32)
}

func TestDispatcherProtocolErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		eventType    agenthooks.EventType
		data         any
		responseBody []byte
		assertRow    func(*testing.T, database.ChatHookDispatch)
	}{
		{
			name:         "malformed JSON",
			eventType:    agenthooks.EventStop,
			data:         agenthooks.StopData{},
			responseBody: []byte(`{"end_chat":`),
		},
		{
			name:         "oversized model context",
			eventType:    agenthooks.EventStop,
			data:         agenthooks.StopData{},
			responseBody: mustJSON(t, agenthooks.Response{ModelContext: string(bytes.Repeat([]byte("x"), maxModelContextBytes+1))}),
			assertRow: func(t *testing.T, row database.ChatHookDispatch) {
				require.True(t, row.ModelContext.Valid)
				require.Len(t, row.ModelContext.String, maxModelContextBytes+1)
			},
		},
		{
			name:      "invalid user prompt override shape",
			eventType: agenthooks.EventUserPromptSubmit,
			data:      agenthooks.UserPromptSubmitData{Prompt: "question"},
			responseBody: mustJSON(t, agenthooks.Response{Permission: &agenthooks.Permission{
				Decision:      agenthooks.PermissionAllow,
				InputOverride: json.RawMessage(`{"unexpected":"value"}`),
			}}),
		},
		{
			name:      "ask decision",
			eventType: agenthooks.EventUserPromptSubmit,
			data:      agenthooks.UserPromptSubmitData{Prompt: "question"},
			responseBody: mustJSON(t, agenthooks.Response{Permission: &agenthooks.Permission{
				Decision: agenthooks.PermissionAsk,
			}}),
			assertRow: func(t *testing.T, row database.ChatHookDispatch) {
				require.False(t, row.Decision.Valid, "rejected responses must not persist a reusable decision")
			},
		},
		{
			name:      "pre_tool_use allow without input_override",
			eventType: agenthooks.EventPreToolUse,
			data:      agenthooks.PreToolUseData{ToolName: "run_command", ToolInput: json.RawMessage(`{"cmd":"ls"}`)},
			responseBody: mustJSON(t, agenthooks.Response{Permission: &agenthooks.Permission{
				Decision: agenthooks.PermissionAllow,
			}}),
			assertRow: func(t *testing.T, row database.ChatHookDispatch) {
				require.False(t, row.Decision.Valid, "rejected responses must not persist a reusable decision")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			db, _ := dbtestutil.NewDB(t)
			event := newTestEvent(t, db, test.eventType, test.data)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write(test.responseBody)
				assert.NoError(t, err)
			}))
			t.Cleanup(server.Close)

			_, err := newTestDispatcher(t, db, server.Client(), server.URL, time.Second).Dispatch(
				testutil.Context(t, testutil.WaitLong), event,
			)
			require.Error(t, err)
			row := singleDispatch(t, db, event.ChatID)
			require.Equal(t, resultProtocolError, row.Result)
			require.True(t, row.Error.Valid)
			if test.assertRow != nil {
				test.assertRow(t, row)
			}
		})
	}
}

func TestDispatcherOverCapacity(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	event := newTestEvent(t, db, agenthooks.EventStop, agenthooks.StopData{})
	dispatcher := newTestDispatcher(t, db, nil, "http://unused.test", 10*time.Millisecond)
	for range maxConcurrentDispatches {
		dispatcher.semaphore <- struct{}{}
	}
	defer func() {
		for range maxConcurrentDispatches {
			<-dispatcher.semaphore
		}
	}()

	_, err := dispatcher.Dispatch(testutil.Context(t, testutil.WaitLong), event)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	row := singleDispatch(t, db, event.ChatID)
	require.Equal(t, resultOverCapacity, row.Result)
	require.False(t, row.HttpStatus.Valid)
	require.True(t, row.FinishedAt.Valid)
}

func TestDispatcherInvalidToolInputFinalizesProtocolError(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	toolUseID := "call_" + uuid.NewString()
	event := newTestEvent(t, db, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{
		ToolUseID: toolUseID,
		ToolName:  "edit",
		ToolInput: json.RawMessage(`{"path":`),
	})

	var hookRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hookRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	_, err := newTestDispatcher(t, db, server.Client(), server.URL, time.Second).Dispatch(
		testutil.Context(t, testutil.WaitLong), event,
	)
	require.Error(t, err)
	require.Zero(t, hookRequests.Load())

	row := singleDispatch(t, db, event.ChatID)
	require.Equal(t, resultProtocolError, row.Result)
	require.True(t, row.FinishedAt.Valid, "dispatch must finalize instead of staying pending")
	require.False(t, row.OriginalInput.Valid, "malformed input must not persist as jsonb")
}

func newTestDispatcher(
	t *testing.T,
	db database.Store,
	client *http.Client,
	hookURL string,
	timeout time.Duration,
) *Dispatcher {
	t.Helper()
	return New(
		testutil.Logger(t),
		db,
		client,
		hookURL,
		testSecret,
		timeout,
		testDeploymentID,
		testVersion,
		prometheus.NewRegistry(),
	)
}

func newTestEvent(t *testing.T, db database.Store, eventType agenthooks.EventType, data any) Event {
	t.Helper()
	user := dbgen.User(t, db, database.User{})
	organization := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    organization.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})
	return Event{
		Type:    eventType,
		ChatID:  chat.ID,
		OwnerID: user.ID,
		Data:    data,
	}
}

func singleDispatch(t *testing.T, db database.Store, chatID uuid.UUID) database.ChatHookDispatch {
	t.Helper()
	rows, err := db.ListChatHookDispatchesByChatID(testutil.Context(t, testutil.WaitLong), chatID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	return rows[0]
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
