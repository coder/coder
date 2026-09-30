package responsesws_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/x/proxy/responsesws"
	codertestutil "github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakeConn is an in-memory upstream. Tests push server frames to toClient
// and read forwarded client frames from written.
type fakeConn struct {
	toClient  chan []byte
	written   chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func (c *fakeConn) Read(ctx context.Context) ([]byte, error) {
	select {
	case f, ok := <-c.toClient:
		if !ok {
			return nil, io.EOF
		}
		return f, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, net.ErrClosed
	}
}

func (c *fakeConn) Write(ctx context.Context, msg []byte) error {
	select {
	case c.written <- append([]byte(nil), msg...):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return net.ErrClosed
	}
}

func (c *fakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

type testRecorder struct {
	testutil.MockRecorder
	failStart atomic.Bool
}

func (r *testRecorder) RecordInterception(ctx context.Context, rec *recorder.InterceptionRecord) error {
	if r.failStart.Load() {
		return xerrors.New("database unavailable")
	}
	return r.MockRecorder.RecordInterception(ctx, rec)
}

type harness struct {
	t    *testing.T
	ctx  context.Context
	sess *responsesws.Session
	conn *fakeConn
	rec  *testRecorder
}

func newHarness(ctx context.Context, t *testing.T, admit responsesws.AdmitFunc) *harness {
	t.Helper()
	conn := &fakeConn{toClient: make(chan []byte, 16), written: make(chan []byte, 16), closed: make(chan struct{})}
	rec := &testRecorder{}
	sessionID := "client-session"
	sess, err := responsesws.NewSession(aibcontext.AsActor(ctx, "user-1", recorder.Metadata{"k": "v"}), conn, responsesws.Options{
		Provider:        provider.NewOpenAI(config.OpenAI{}),
		Recorder:        rec,
		Admit:           admit,
		Logger:          slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		Client:          "codex",
		ClientSessionID: &sessionID,
		UserAgent:       "codex-cli/1.0",
		CredentialKind:  recorder.CredentialKindCentralized,
		CredentialHint:  "sk-...abcd",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(nil) })
	return &harness{t: t, ctx: ctx, sess: sess, conn: conn, rec: rec}
}

// send forwards a client frame and returns what reached upstream, or nil.
func (h *harness) send(frame string) []byte {
	h.t.Helper()
	require.NoError(h.t, h.sess.Send(h.ctx, []byte(frame)))
	select {
	case got := <-h.conn.written:
		return got
	default:
		return nil
	}
}

// relay pushes server frames upstream and requires the client receives them
// unchanged. Recording for a frame completes before Recv returns it.
func (h *harness) relay(frames ...string) {
	h.t.Helper()
	for _, f := range frames {
		codertestutil.RequireSend(h.ctx, h.t, h.conn.toClient, []byte(f))
		got, err := h.sess.Recv(h.ctx)
		require.NoError(h.t, err)
		require.Equal(h.t, f, string(got))
	}
}

func (h *harness) interceptionFor(model string) *recorder.InterceptionRecord {
	h.t.Helper()
	for _, ic := range h.rec.RecordedInterceptions() {
		if ic.Model == model {
			return ic
		}
	}
	h.t.Fatalf("no interception for model %q", model)
	return nil
}

func lane(streamID string) string {
	if streamID == "" {
		return ""
	}
	return fmt.Sprintf(`,"stream_id":%q`, streamID)
}

func create(streamID, model, prompt string) string {
	return fmt.Sprintf(`{"type":"response.create"%s,"model":%q,"input":[{"role":"user","content":[{"type":"input_text","text":%q}]}]}`, lane(streamID), model, prompt)
}

func created(streamID, id, model string) string {
	return fmt.Sprintf(`{"type":"response.created"%s,"response":{"id":%q,"model":%q,"status":"in_progress"}}`, lane(streamID), id, model)
}

func delta(streamID string) string {
	return fmt.Sprintf(`{"type":"response.output_text.delta"%s,"delta":"hi"}`, lane(streamID))
}

func terminal(streamID, eventType, id, extra string) string {
	return fmt.Sprintf(`{"type":%q%s,"response":{"id":%q%s,"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":30,"cache_write_tokens":20},"output_tokens":7,"output_tokens_details":{"reasoning_tokens":3},"total_tokens":107}}}`, eventType, lane(streamID), id, extra)
}

func completed(streamID, id string) string {
	return terminal(streamID, "response.completed", id, `,"status":"completed"`)
}

func TestFramesForwardedUnchanged(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)

	for _, frame := range []string{
		"{ \"type\" : \"response.create\", \"model\":\"gpt-6\", \"input\":\"hi\", \"x\":[1,2] }",
		`{"type":"response.inject","response_id":"resp_1","input":"more"}`,
		`{"type":"response.future","opaque":{"a":1}}`,
		`not json`,
	} {
		require.Equal(t, frame, string(h.send(frame)))
	}
	h.relay(created("", "resp_1", "gpt-6"), `{"type":"response.future","z":true}`, `{"type":"response.inject.created","item":{}}`, completed("", "resp_1"))
}

func TestCreateRecordsInterceptionAndUsage(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)

	require.NotNil(t, h.send(create("", "gpt-6", "hello")))
	ics := h.rec.RecordedInterceptions()
	require.Len(t, ics, 1)
	ic := ics[0]
	require.NotEmpty(t, ic.ID)
	require.Equal(t, "user-1", ic.InitiatorID)
	require.Equal(t, recorder.Metadata{"k": "v"}, ic.Metadata)
	require.Equal(t, config.ProviderOpenAI, ic.Provider)
	require.Equal(t, config.ProviderOpenAI, ic.ProviderName)
	require.Equal(t, "gpt-6", ic.Model)
	require.Equal(t, "codex", ic.Client)
	require.Equal(t, "client-session", *ic.ClientSessionID)
	require.Equal(t, "codex-cli/1.0", ic.UserAgent)
	require.Equal(t, recorder.CredentialKindCentralized, ic.CredentialKind)
	require.Equal(t, "sk-...abcd", ic.CredentialHint)

	h.relay(created("", "resp_1", "gpt-6"), delta(""))
	require.Nil(t, h.rec.RecordedInterceptionEnd(ic.ID))
	h.relay(completed("", "resp_1"))

	usages := h.rec.RecordedTokenUsages()
	require.Len(t, usages, 1)
	require.Equal(t, recorder.TokenUsageRecord{
		InterceptionID: ic.ID, MsgID: "resp_1", Input: 50, Output: 7,
		CacheReadInputTokens: 30, CacheWriteInputTokens: 20,
		ExtraTokenTypes: map[string]int64{"output_reasoning": 3, "total_tokens": 107},
	}, *usages[0])
	prompts := h.rec.RecordedPromptUsages()
	require.Len(t, prompts, 1)
	require.Equal(t, recorder.PromptUsageRecord{InterceptionID: ic.ID, MsgID: "resp_1", Prompt: "hello"}, *prompts[0])
	end := h.rec.RecordedInterceptionEnd(ic.ID)
	require.NotNil(t, end)
	require.Empty(t, end.ErrorType)
	require.Equal(t, "sk-...abcd", end.CredentialHint)
}

func TestAdmissionRefused(t *testing.T) {
	t.Parallel()
	var refuse atomic.Pointer[error]
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, func(_ context.Context, model string) error {
		if err := refuse.Load(); err != nil {
			return *err
		}
		require.Equal(t, "gpt-6", model)
		return nil
	})

	for _, tc := range []struct {
		err             error
		status          int64
		code, message   string
		streamID, field string
	}{
		{intercept.NewResponseError("budget exceeded", intercept.OpenAIErrTypeRateLimit, intercept.OpenAIErrCodeRateLimit, 429, 0), 429, "rate_limit_exceeded", "budget exceeded", "lane-a", "lane-a"},
		{xerrors.New("internal detail"), 403, "request_refused", "request refused by AI Gateway", "", ""},
	} {
		refuse.Store(&tc.err)
		require.Nil(t, h.send(create(tc.streamID, "gpt-6", "hi")))
		ev, err := h.sess.Recv(h.ctx)
		require.NoError(t, err)
		require.Equal(t, "error", gjson.GetBytes(ev, "type").String())
		require.Equal(t, tc.code, gjson.GetBytes(ev, "code").String())
		require.Equal(t, tc.message, gjson.GetBytes(ev, "message").String())
		require.Equal(t, tc.code, gjson.GetBytes(ev, "error.code").String())
		require.Equal(t, tc.status, gjson.GetBytes(ev, "status").Int())
		require.Equal(t, tc.field, gjson.GetBytes(ev, "stream_id").String())
	}
	require.Empty(t, h.rec.RecordedInterceptions())

	refuse.Store(nil)
	require.NotNil(t, h.send(create("", "gpt-6", "hi")))
	require.Len(t, h.rec.RecordedInterceptions(), 1)
}

func TestRecorderStartFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)

	h.rec.failStart.Store(true)
	require.Nil(t, h.send(create("s1", "gpt-6", "hi")))
	ev, err := h.sess.Recv(h.ctx)
	require.NoError(t, err)
	require.Equal(t, "failed to record interception", gjson.GetBytes(ev, "message").String())
	require.Equal(t, "s1", gjson.GetBytes(ev, "stream_id").String())

	h.rec.failStart.Store(false)
	require.NotNil(t, h.send(create("s1", "gpt-6", "hi")))
	require.Len(t, h.rec.RecordedInterceptions(), 1)
}

func TestLanesBindIndependently(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)

	h.send(create("a", "model-a1", "a1"))
	h.send(create("b", "model-b1", "b1"))
	h.send(create("a", "model-a2", "a2"))
	h.relay(
		created("b", "resp_b1", "model-b1"), created("a", "resp_a1", "model-a1"),
		delta("a"), delta("b"),
		completed("a", "resp_a1"), created("a", "resp_a2", "model-a2"),
		completed("b", "resp_b1"), completed("a", "resp_a2"),
	)

	byMsg := map[string]string{}
	for _, u := range h.rec.RecordedTokenUsages() {
		byMsg[u.MsgID] = u.InterceptionID
	}
	require.Equal(t, map[string]string{
		"resp_a1": h.interceptionFor("model-a1").ID,
		"resp_a2": h.interceptionFor("model-a2").ID,
		"resp_b1": h.interceptionFor("model-b1").ID,
	}, byMsg)
	require.Len(t, h.rec.RecordedInterceptions(), 3)
	h.rec.VerifyAllInterceptionsEnded(t)
}

func TestSteerContinuationStaysInInterception(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)

	h.send(create("", "gpt-6", "draft a plan"))
	h.relay(created("", "resp_1", "gpt-6"))
	steer := `{"type":"response.steer","previous_response_id":"resp_1","input":"keep it short"}`
	require.Equal(t, steer, string(h.send(steer)))
	id := h.interceptionFor("gpt-6").ID
	h.relay(
		`{"type":"response.steer.accepted","steer":{"id":"steer_1","previous_response_id":"resp_1"}}`,
		terminal("", "response.incomplete", "resp_1", `,"status":"incomplete","incomplete_details":{"reason":"steered"}`),
	)
	require.Nil(t, h.rec.RecordedInterceptionEnd(id))
	h.relay(created("", "resp_2", "gpt-6"), completed("", "resp_2"))

	require.Len(t, h.rec.RecordedInterceptions(), 1)
	require.NotNil(t, h.rec.RecordedInterceptionEnd(id))
	require.Empty(t, h.rec.RecordedInterceptionEnd(id).ErrorType)
	usages := h.rec.RecordedTokenUsages()
	require.Len(t, usages, 2)
	for _, u := range usages {
		require.Equal(t, id, u.InterceptionID)
	}
	prompts := h.rec.RecordedPromptUsages()
	require.Len(t, prompts, 2)
	require.Equal(t, "keep it short", prompts[1].Prompt)
	require.Equal(t, "resp_1", prompts[1].MsgID)
	require.Equal(t, id, prompts[1].InterceptionID)
}

func TestUnexplainedResponseOpensInterception(t *testing.T) {
	t.Parallel()
	var admitted atomic.Int32
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, func(context.Context, string) error {
		admitted.Add(1)
		return nil
	})

	h.send(`{"type":"response.future_create","model":"gpt-x"}`)
	h.relay(created("z", "resp_9", "gpt-x"), completed("z", "resp_9"))
	ic := h.interceptionFor("gpt-x")
	require.Equal(t, "resp_9", h.rec.RecordedTokenUsages()[0].MsgID)
	require.Equal(t, ic.ID, h.rec.RecordedTokenUsages()[0].InterceptionID)
	require.NotNil(t, h.rec.RecordedInterceptionEnd(ic.ID))
	require.Zero(t, admitted.Load())
}

func TestErrorsEndInterceptions(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)

	h.send(create("a", "model-a", "a"))
	h.send(create("b", "model-b", "b"))
	h.relay(
		`{"type":"error","status":400,"stream_id":"a","error":{"type":"invalid_request_error","code":"previous_response_not_found","message":"not found"}}`,
		created("b", "resp_b", "model-b"),
		terminal("b", "response.failed", "resp_b", `,"status":"failed","error":{"code":"rate_limit_exceeded","message":"slow down"}`),
	)
	endA := h.rec.RecordedInterceptionEnd(h.interceptionFor("model-a").ID)
	require.Equal(t, recorder.ErrorTypeBadRequest, endA.ErrorType)
	require.Equal(t, "not found", endA.ErrorMessage)
	endB := h.rec.RecordedInterceptionEnd(h.interceptionFor("model-b").ID)
	require.Equal(t, recorder.ErrorTypeRateLimited, endB.ErrorType)
	require.Equal(t, "slow down", endB.ErrorMessage)
}

func TestSessionEndEndsOpenInterceptions(t *testing.T) {
	t.Parallel()

	start := func(ctx context.Context, t *testing.T) *harness {
		h := newHarness(ctx, t, nil)
		h.send(create("", "model-active", "a"))
		h.send(create("", "model-pending", "b"))
		h.relay(created("", "resp_1", "model-active"))
		return h
	}
	requireEnded := func(t *testing.T, h *harness, message string) {
		for _, ic := range h.rec.RecordedInterceptions() {
			end := h.rec.RecordedInterceptionEnd(ic.ID)
			require.NotNil(t, end, ic.Model)
			require.Equal(t, recorder.ErrorTypeUnknown, end.ErrorType)
			require.Equal(t, message, end.ErrorMessage)
		}
	}

	t.Run("UpstreamClose", func(t *testing.T) {
		t.Parallel()
		h := start(codertestutil.Context(t, codertestutil.WaitShort), t)
		close(h.conn.toClient)
		_, err := h.sess.Recv(h.ctx)
		require.ErrorIs(t, err, io.EOF)
		requireEnded(t, h, "EOF")
		require.ErrorIs(t, h.sess.Send(h.ctx, []byte(create("", "gpt-6", "x"))), responsesws.ErrClosed)
	})

	t.Run("Close", func(t *testing.T) {
		t.Parallel()
		h := start(codertestutil.Context(t, codertestutil.WaitShort), t)
		require.NoError(t, h.sess.Close(xerrors.New("client went away")))
		requireEnded(t, h, "client went away")
		require.ErrorIs(t, h.sess.Send(h.ctx, []byte(create("", "gpt-6", "x"))), responsesws.ErrClosed)
		_, err := h.sess.Recv(h.ctx)
		require.Error(t, err)
		require.NoError(t, h.sess.Close(nil))
	})

	t.Run("ContextCanceled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(codertestutil.Context(t, codertestutil.WaitShort))
		h := start(ctx, t)
		cancel()
		_, err := h.sess.Recv(context.Background())
		require.ErrorIs(t, err, context.Canceled)
		requireEnded(t, h, context.Canceled.Error())
	})
}
