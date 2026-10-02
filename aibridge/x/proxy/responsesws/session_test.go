package responsesws_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/credential"
	"github.com/coder/coder/v2/aibridge/extract"
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
	toClient chan []byte
	// readErrs makes the next Read return the error.
	readErrs  chan error
	written   chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	// onWrite, when set, replaces Write.
	onWrite atomic.Pointer[func(ctx context.Context) error]
}

func (c *fakeConn) Read(ctx context.Context) ([]byte, error) {
	select {
	case f, ok := <-c.toClient:
		if !ok {
			return nil, io.EOF
		}
		return f, nil
	case err := <-c.readErrs:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, net.ErrClosed
	}
}

func (c *fakeConn) Write(ctx context.Context, msg []byte) error {
	if f := c.onWrite.Load(); f != nil {
		return (*f)(ctx)
	}
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
	// blockEnd makes RecordInterceptionEnded wait until its ctx ends.
	blockEnd atomic.Bool
	// blockUsage makes RecordTokenUsage wait until its ctx ends.
	blockUsage atomic.Bool
	// usageGate, when set, makes RecordTokenUsage wait until it is closed.
	// Each wait first sends on usageEntered.
	usageGate    atomic.Pointer[chan struct{}]
	usageEntered chan struct{}

	mu sync.Mutex
	// endCounts counts end records per interception ID.
	endCounts map[string]int
	// endDeadlines holds the ctx deadline of each end record.
	endDeadlines map[string]time.Time
	// usageReturned is when the last token usage record returned.
	usageReturned time.Time
}

func (r *testRecorder) RecordTokenUsage(ctx context.Context, rec *recorder.TokenUsageRecord) error {
	if gate := r.usageGate.Load(); gate != nil {
		select {
		case r.usageEntered <- struct{}{}:
		default:
		}
		select {
		case <-*gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if r.blockUsage.Load() {
		<-ctx.Done()
		return ctx.Err()
	}
	err := r.MockRecorder.RecordTokenUsage(ctx, rec)
	r.mu.Lock()
	r.usageReturned = time.Now()
	r.mu.Unlock()
	return err
}

func (r *testRecorder) RecordInterceptionEnded(ctx context.Context, rec *recorder.InterceptionRecordEnded) error {
	deadline, _ := ctx.Deadline()
	r.mu.Lock()
	if r.endCounts == nil {
		r.endCounts, r.endDeadlines = map[string]int{}, map[string]time.Time{}
	}
	r.endCounts[rec.ID]++
	r.endDeadlines[rec.ID] = deadline
	r.mu.Unlock()
	if r.blockEnd.Load() {
		<-ctx.Done()
		return ctx.Err()
	}
	return r.MockRecorder.RecordInterceptionEnded(ctx, rec)
}

func (r *testRecorder) endCount(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.endCounts[id]
}

// logSink captures log messages.
type logSink struct {
	mu       sync.Mutex
	messages []string
}

func (s *logSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, e.Message)
}

func (*logSink) Sync() {}

func (s *logSink) count(message string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, m := range s.messages {
		if m == message {
			n++
		}
	}
	return n
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
	logs *logSink
}

func newHarness(ctx context.Context, t *testing.T, admit responsesws.AdmitFunc) *harness {
	t.Helper()
	conn := &fakeConn{toClient: make(chan []byte, 16), readErrs: make(chan error, 1), written: make(chan []byte, 16), closed: make(chan struct{})}
	rec := &testRecorder{usageEntered: make(chan struct{}, 64)}
	logs := &logSink{}
	sessionID := "client-session"
	// Production recorders refuse records with unset timestamps. The
	// validating middleware logs each refusal at error level, which fails
	// the test.
	validating := recorder.NewValidatingRecorder(slogtest.Make(t, nil), rec)
	sess, err := responsesws.NewSession(aibcontext.AsActor(ctx, "user-1", "", recorder.Metadata{"k": "v"}), conn, responsesws.Options{
		Provider:        provider.NewOpenAI(config.OpenAI{}),
		Recorder:        validating,
		Admit:           admit,
		Logger:          slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).AppendSinks(logs),
		Client:          "codex",
		ClientSessionID: &sessionID,
		UserAgent:       "codex-cli/1.0",
		CredentialKind:  credential.KindCentralized,
		CredentialHint:  "sk-...abcd",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(nil) })
	return &harness{t: t, ctx: ctx, sess: sess, conn: conn, rec: rec, logs: logs}
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
// unchanged, then waits until their accounting completed.
func (h *harness) relay(frames ...string) {
	h.t.Helper()
	h.forward(frames...)
	require.NoError(h.t, responsesws.Drain(h.ctx, h.sess))
}

// forward pushes server frames upstream and requires the client receives
// them unchanged, without waiting for their accounting.
func (h *harness) forward(frames ...string) {
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
	require.Equal(t, credential.KindCentralized, ic.CredentialKind)
	require.Equal(t, "sk-...abcd", ic.CredentialHint)

	h.relay(created("", "resp_1", "gpt-6"), delta(""))
	require.Nil(t, h.rec.RecordedInterceptionEnd(ic.ID))
	h.relay(terminal("", "response.completed", "resp_1", `,"status":"completed","model":"gpt-6-2026-01-01"`))

	usages := h.rec.RecordedTokenUsages()
	require.Len(t, usages, 1)
	usage := *usages[0]
	require.False(t, usage.CreatedAt.IsZero())
	usage.CreatedAt = time.Time{}
	require.Equal(t, recorder.TokenUsageRecord{
		InterceptionID: ic.ID, MsgID: "resp_1", ProviderModel: "gpt-6-2026-01-01", Input: 50, Output: 7,
		CacheReadInputTokens: 30, CacheWriteInputTokens: 20,
		ExtraTokenTypes: map[string]int64{"output_reasoning": 3, "total_tokens": 107},
	}, usage)
	prompts := h.rec.RecordedPromptUsages()
	require.Len(t, prompts, 1)
	prompt := *prompts[0]
	require.False(t, prompt.CreatedAt.IsZero())
	prompt.CreatedAt = time.Time{}
	require.Equal(t, recorder.PromptUsageRecord{InterceptionID: ic.ID, MsgID: "resp_1", Prompt: "hello"}, prompt)
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
		require.False(t, gjson.GetBytes(ev, "param").Exists())
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

// TestSteerRecordsPromptOnOwner covers the steering contract: the steer is
// relayed and its input recorded as a prompt on the interception owning the
// steered response, while the automatic continuation is recorded like any
// other response, not promised to share that interception.
func TestSteerRecordsPromptOnOwner(t *testing.T) {
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
		created("", "resp_2", "gpt-6-continuation"), completed("", "resp_2"),
	)

	require.Len(t, h.rec.RecordedInterceptions(), 2)
	require.Equal(t, map[string][]string{
		"resp_1": {id},
		"resp_2": {h.interceptionFor("gpt-6-continuation").ID},
	}, usagesByResponse(h))
	prompts := h.rec.RecordedPromptUsages()
	require.Len(t, prompts, 2)
	require.Equal(t, "keep it short", prompts[1].Prompt)
	require.Equal(t, "resp_1", prompts[1].MsgID)
	require.Equal(t, id, prompts[1].InterceptionID)
	h.rec.VerifyAllInterceptionsEnded(t)
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

func TestErrorsAndTerminalEvents(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)
	errEvent := func(streamID string) string {
		return fmt.Sprintf(`{"type":"error","status":400%s,"error":{"type":"invalid_request_error","code":"previous_response_not_found","message":"not found"}}`, lane(streamID))
	}

	h.send(create("a", "model-a1", "a1"))
	h.relay(created("a", "resp_a1", "model-a1"))
	h.send(create("a", "model-a2", "a2"))
	h.send(create("b", "model-b", "b"))
	h.send(create("", "model-d", "d"))
	h.relay(
		// Errors reject the oldest pending create on their lane, never the
		// in-flight response.
		errEvent("a"), errEvent(""), errEvent(""),
		completed("a", "resp_unknown"),
		terminal("a", "response.incomplete", "resp_a1", `,"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}`),
		created("b", "resp_b", "model-b"),
		terminal("b", "response.failed", "resp_b", `,"status":"failed","error":{"code":"rate_limit_exceeded","message":"slow down"}`),
	)
	for model, want := range map[string]recorder.ErrorType{
		"model-a1": "", "model-a2": recorder.ErrorTypeBadRequest,
		"model-d": recorder.ErrorTypeBadRequest, "model-b": recorder.ErrorTypeRateLimited,
	} {
		end := h.rec.RecordedInterceptionEnd(h.interceptionFor(model).ID)
		require.NotNil(t, end, model)
		require.Equal(t, want, end.ErrorType, model)
	}
	require.Equal(t, "slow down", h.rec.RecordedInterceptionEnd(h.interceptionFor("model-b").ID).ErrorMessage)
	// Incomplete and failed responses are billed, so their usage is recorded,
	// and so is the usage of a response no interception owns.
	usages := usagesByResponse(h)
	require.Len(t, usages, 3)
	require.Equal(t, []string{h.interceptionFor("model-a1").ID}, usages["resp_a1"])
	require.Equal(t, []string{h.interceptionFor("model-b").ID}, usages["resp_b"])
	require.Len(t, usages["resp_unknown"], 1)
	require.NotNil(t, h.rec.RecordedInterceptionEnd(usages["resp_unknown"][0]))
}

// TestConcurrentSendRecvClose fills every buffer (upstream writes, relayed
// frames, synthesized errors) and closes the session while all sides are
// blocked or busy.
func TestConcurrentSendRecvClose(t *testing.T) {
	t.Parallel()
	var n atomic.Int32
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	h := newHarness(ctx, t, func(context.Context, string) error {
		if n.Add(1)%2 == 0 {
			return xerrors.New("refused")
		}
		return nil
	})

	var wg sync.WaitGroup
	loop := func(step func(i int) bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				if !step(i) {
					return
				}
			}
		}()
	}
	loop(func(i int) bool {
		return h.sess.Send(ctx, []byte(create(fmt.Sprint(i%3), "gpt-6", "hi"))) == nil
	})
	loop(func(int) bool {
		select {
		case <-h.conn.written:
			return true
		case <-h.conn.closed:
			return false
		}
	})
	loop(func(i int) bool {
		select {
		case h.conn.toClient <- []byte(created(fmt.Sprint(i%3), fmt.Sprintf("resp_%d", i), "gpt-6")):
			return true
		case <-h.conn.closed:
			return false
		}
	})
	// Receive a few frames, then stop until Close so every buffer fills.
	paused, closed := make(chan struct{}), make(chan struct{})
	loop(func(i int) bool {
		if i == 20 {
			close(paused)
			<-closed
		}
		_, err := h.sess.Recv(ctx)
		return err == nil
	})
	codertestutil.TryReceive(ctx, t, paused)
	require.Eventually(t, func() bool { return len(h.conn.toClient) == cap(h.conn.toClient) }, testutil.WaitShort, testutil.IntervalFast)
	require.NoError(t, h.sess.Close(nil))
	close(closed)
	wg.Wait()
	require.NotEmpty(t, h.rec.RecordedInterceptions())
	h.rec.VerifyAllInterceptionsEnded(t)
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
		// The accountant ends open interceptions after the reader exits;
		// Close waits for it.
		require.NoError(t, h.sess.Close(nil))
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
		require.NoError(t, h.sess.Close(nil))
		requireEnded(t, h, context.Canceled.Error())
	})
}

// TestCloseDuringAdmission covers a session that ends while admission is
// still deciding a create, whether by Close or by the upstream reader
// ending: Send must return promptly and nothing may be recorded for the
// frame, which is never forwarded. Recv reports why the session ended.
func TestCloseDuringAdmission(t *testing.T) {
	t.Parallel()
	errReset := xerrors.New("connection reset by peer")
	admits := []struct {
		name  string
		admit func(ctx context.Context) error
	}{
		// Admission honors its ctx, which ending the session must cancel.
		{"Canceled", func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}},
		// Admission returns success only after the session ended.
		{"LateSuccess", func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}},
	}
	ends := []struct {
		name string
		end  func(t *testing.T, h *harness)
		// recvErr is what Recv reports once the session ended.
		recvErr error
	}{
		{"Close", func(t *testing.T, h *harness) { require.NoError(t, h.sess.Close(nil)) }, responsesws.ErrClosed},
		{"UpstreamEOF", func(_ *testing.T, h *harness) { close(h.conn.toClient) }, io.EOF},
		{"UpstreamError", func(t *testing.T, h *harness) {
			codertestutil.RequireSend(h.ctx, t, h.conn.readErrs, errReset)
		}, errReset},
	}
	for _, end := range ends {
		for _, tc := range admits {
			t.Run(end.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := codertestutil.Context(t, codertestutil.WaitShort)
				entered := make(chan struct{})
				h := newHarness(ctx, t, func(ctx context.Context, _ string) error {
					close(entered)
					return tc.admit(ctx)
				})
				// The caller's ctx outlives the test's wait, so only the
				// session ending can unblock admission in time.
				sendCtx, cancelSend := context.WithCancel(context.WithoutCancel(ctx))
				defer cancelSend()
				sent := make(chan error, 1)
				go func() { sent <- h.sess.Send(sendCtx, []byte(create("", "gpt-6", "hi"))) }()
				_ = codertestutil.TryReceive(ctx, t, entered)
				end.end(t, h)

				err := codertestutil.TryReceive(ctx, t, sent)
				require.ErrorIs(t, err, responsesws.ErrClosed)
				require.Empty(t, h.rec.RecordedInterceptions())
				select {
				case frame := <-h.conn.written:
					t.Fatalf("create forwarded after the session ended: %s", frame)
				default:
				}
				_, err = h.sess.Recv(ctx)
				require.ErrorIs(t, err, end.recvErr)
				require.NoError(t, h.sess.Close(nil))
				_, err = h.sess.Recv(ctx)
				require.ErrorIs(t, err, end.recvErr)
			})
		}
	}
}

// TestLaneStateReleased requires that completed requests on distinct
// stream_ids leave no per-lane state behind, so a client using a fresh
// stream_id per request does not grow the session without bound.
func TestLaneStateReleased(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)

	for i := range 20 {
		streamID := fmt.Sprintf("lane-%d", i)
		id := fmt.Sprintf("resp_%d", i)
		require.NotNil(t, h.send(create(streamID, "gpt-6", "hi")))
		h.relay(created(streamID, id, "gpt-6"), delta(streamID), completed(streamID, id))
	}
	// A steered response and its continuation on their own lane.
	require.NotNil(t, h.send(create("steer", "gpt-6", "hi")))
	h.relay(created("steer", "resp_s1", "gpt-6"),
		terminal("steer", "response.incomplete", "resp_s1", `,"status":"incomplete","incomplete_details":{"reason":"steered"}`),
		created("steer", "resp_s2", "gpt-6"), completed("steer", "resp_s2"))

	require.Len(t, h.rec.RecordedInterceptions(), 22)
	require.Equal(t, map[string]int{}, responsesws.StateSizes(h.sess))
}

// TestAdmitCallerCanceledIsNotRefusal requires that a caller canceling while
// admission runs is reported as the cancellation, never as a refusal event.
func TestAdmitCallerCanceledIsNotRefusal(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	var cancelCaller atomic.Pointer[context.CancelFunc]
	h := newHarness(ctx, t, func(ctx context.Context, _ string) error {
		(*cancelCaller.Load())()
		<-ctx.Done()
		return ctx.Err()
	})
	for range 200 {
		sendCtx, cancel := context.WithCancel(ctx)
		cancelCaller.Store(&cancel)
		err := h.sess.Send(sendCtx, []byte(create("", "gpt-6", "hi")))
		cancel()
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, responsesws.QueuedEvents(h.sess))
	}
	require.Empty(t, h.rec.RecordedInterceptions())
	require.Empty(t, h.conn.written)
}

// TestSendFailureClassification covers how Send reports a step that fails or
// is cut short: a session end wins over a caller cancellation, which wins
// over the step's own error. Only a step's own error synthesizes an event,
// and an interception Send started is always ended.
func TestSendFailureClassification(t *testing.T) {
	t.Parallel()
	errBroken := xerrors.New("broken pipe")
	type control struct {
		h            *harness
		cancelCaller context.CancelFunc
	}
	endUpstream := func(c control) { close(c.h.conn.toClient) }
	for _, tc := range []struct {
		name  string
		frame string
		// admit and write, when set, run in place of admission and the
		// upstream write. Each receives the Send operation's ctx.
		admit func(ctx context.Context, c control) error
		write func(ctx context.Context, c control) error
		// expire gives the caller's ctx a short deadline.
		expire  bool
		sendErr error
		// event is the code of the error event Send queues, if any.
		event string
		// ended is the recorded end of the one started interception, or
		// nil when Send must start none.
		ended *recorder.ErrorType
	}{
		{
			name: "AdmitRefused", frame: create("", "gpt-6", "hi"),
			admit: func(context.Context, control) error { return xerrors.New("no budget") },
			event: "request_refused",
		},
		{
			name: "CallerCanceledDuringAdmit", frame: create("", "gpt-6", "hi"),
			admit: func(ctx context.Context, c control) error {
				c.cancelCaller()
				<-ctx.Done()
				return ctx.Err()
			},
			sendErr: context.Canceled,
		},
		{
			name: "CallerCanceledAdmitLateSuccess", frame: create("", "gpt-6", "hi"),
			admit: func(ctx context.Context, c control) error {
				c.cancelCaller()
				<-ctx.Done()
				return nil
			},
			sendErr: context.Canceled,
		},
		{
			name: "CallerCanceledDuringWrite", frame: create("", "gpt-6", "hi"),
			write: func(ctx context.Context, c control) error {
				c.cancelCaller()
				<-ctx.Done()
				return ctx.Err()
			},
			sendErr: context.Canceled, ended: ptr(recorder.ErrorTypeUnknown),
		},
		{
			name: "CallerDeadlineDuringWrite", frame: create("", "gpt-6", "hi"), expire: true,
			write: func(ctx context.Context, _ control) error {
				<-ctx.Done()
				return ctx.Err()
			},
			sendErr: context.DeadlineExceeded, ended: ptr(recorder.ErrorTypeTimeout),
		},
		{
			name: "UpstreamEndedDuringCreateWrite", frame: create("", "gpt-6", "hi"),
			write: func(ctx context.Context, c control) error {
				endUpstream(c)
				<-ctx.Done()
				return ctx.Err()
			},
			sendErr: responsesws.ErrClosed, ended: ptr(recorder.ErrorTypeUnknown),
		},
		{
			name: "UpstreamEndedDuringWrite", frame: `{"type":"response.inject","input":"more"}`,
			write: func(ctx context.Context, c control) error {
				endUpstream(c)
				<-ctx.Done()
				return ctx.Err()
			},
			sendErr: responsesws.ErrClosed,
		},
		{
			name: "CallerCanceledDuringPlainWrite", frame: `{"type":"response.inject","input":"more"}`,
			write: func(ctx context.Context, c control) error {
				c.cancelCaller()
				<-ctx.Done()
				return ctx.Err()
			},
			sendErr: context.Canceled,
		},
		{
			name: "CreateWriteFailed", frame: create("", "gpt-6", "hi"),
			write:   func(context.Context, control) error { return errBroken },
			sendErr: errBroken, ended: ptr(recorder.ErrorTypeUnknown),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := codertestutil.Context(t, codertestutil.WaitShort)
			var c control
			var admit responsesws.AdmitFunc
			if tc.admit != nil {
				admit = func(ctx context.Context, _ string) error { return tc.admit(ctx, c) }
			}
			c.h = newHarness(ctx, t, admit)
			if tc.write != nil {
				write := func(ctx context.Context) error { return tc.write(ctx, c) }
				c.h.conn.onWrite.Store(&write)
			}
			// The caller's ctx outlives the test's wait, so a step that only
			// honors it cannot end in time.
			sendCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
			if tc.expire {
				sendCtx, cancel = context.WithDeadline(context.WithoutCancel(ctx), time.Now().Add(codertestutil.IntervalFast))
			}
			defer cancel()
			c.cancelCaller = cancel
			sent := make(chan error, 1)
			go func() { sent <- c.h.sess.Send(sendCtx, []byte(tc.frame)) }()

			err := codertestutil.TryReceive(ctx, t, sent)
			if tc.sendErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.sendErr)
			}
			if tc.event == "" {
				require.Zero(t, responsesws.QueuedEvents(c.h.sess))
			} else {
				require.Equal(t, 1, responsesws.QueuedEvents(c.h.sess))
				ev, err := c.h.sess.Recv(ctx)
				require.NoError(t, err)
				require.Equal(t, tc.event, gjson.GetBytes(ev, "error.code").String())
			}
			// When upstream ends, Send can return ErrClosed before the
			// reader records the session's end. Close waits for the reader.
			_ = c.h.sess.Close(nil)
			ics := c.h.rec.RecordedInterceptions()
			if tc.ended == nil {
				require.Empty(t, ics)
				return
			}
			require.Len(t, ics, 1)
			end := c.h.rec.RecordedInterceptionEnd(ics[0].ID)
			require.NotNil(t, end, "started interception left open")
			require.Equal(t, *tc.ended, end.ErrorType)
		})
	}
}

func ptr[T any](v T) *T { return &v }

// usagesByResponse maps each response ID with recorded usage to the IDs of
// the interceptions it was recorded on.
func usagesByResponse(h *harness) map[string][]string {
	byResponse := map[string][]string{}
	for _, u := range h.rec.RecordedTokenUsages() {
		byResponse[u.MsgID] = append(byResponse[u.MsgID], u.InterceptionID)
	}
	return byResponse
}

// TestResponseDuringBlockedWrite covers a response.created that the reader
// handles while a create's upstream write has not returned, as when the
// upstream answers before Send marks the write done.
//
// Before the session marked creates in flight, such a response never bound
// to the create: it got its own interception without a prompt, and the
// create ended without its response. Under load that lost the prompt of a
// first turn answered at once. Upstream answers only creates, and creates
// on a lane are written one at a time, so the response answers the create
// being written, and binds to it.
func TestResponseDuringBlockedWrite(t *testing.T) {
	t.Parallel()
	// blockWrite makes the next upstream write wait until the returned
	// release receives its result, and returns once the write started.
	blockWrite := func(ctx context.Context, t *testing.T, h *harness, frame string) (chan<- error, <-chan error) {
		t.Helper()
		entered, release := make(chan struct{}), make(chan error, 1)
		write := func(ctx context.Context) error {
			close(entered)
			select {
			case err := <-release:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		h.conn.onWrite.Store(&write)
		sent := make(chan error, 1)
		go func() { sent <- h.sess.Send(ctx, []byte(frame)) }()
		_ = codertestutil.TryReceive(ctx, t, entered)
		return release, sent
	}

	t.Run("WriteSucceeds", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := newHarness(ctx, t, nil)
		release, sent := blockWrite(ctx, t, h, create("", "model-create", "first turn"))
		h.relay(created("", "resp_1", "model-create"))
		release <- nil
		require.NoError(t, codertestutil.TryReceive(ctx, t, sent))
		h.conn.onWrite.Store(nil)

		h.relay(completed("", "resp_1"))
		own := h.interceptionFor("model-create")
		require.Len(t, h.rec.RecordedInterceptions(), 1)
		require.Equal(t, map[string][]string{"resp_1": {own.ID}}, usagesByResponse(h))
		prompts := h.rec.RecordedPromptUsages()
		require.Len(t, prompts, 1)
		require.Equal(t, "first turn", prompts[0].Prompt)
		require.Equal(t, own.ID, prompts[0].InterceptionID)
		require.Empty(t, h.rec.RecordedInterceptionEnd(own.ID).ErrorType)

		// The next create binds the next response.
		require.NotNil(t, h.send(create("", "model-next", "next")))
		h.relay(created("", "resp_2", "model-next"), completed("", "resp_2"))
		require.Equal(t, []string{h.interceptionFor("model-next").ID}, usagesByResponse(h)["resp_2"])
		require.NoError(t, h.sess.Close(nil))
		h.rec.VerifyAllInterceptionsEnded(t)
	})
	// A write that fails after its create bound a response ends the create
	// with the write error. The response's events queued before keep their
	// records on the create; later ones open their own interception. The
	// usage is recorded exactly once either way.
	t.Run("WriteFailsBeforeTerminal", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := newHarness(ctx, t, nil)
		release, sent := blockWrite(ctx, t, h, create("", "model-create", "first turn"))
		h.relay(created("", "resp_1", "model-create"))
		release <- errWrite
		require.ErrorIs(t, codertestutil.TryReceive(ctx, t, sent), errWrite)
		h.conn.onWrite.Store(nil)
		h.relay(completed("", "resp_1"))

		own := h.interceptionFor("model-create")
		end := h.rec.RecordedInterceptionEnd(own.ID)
		require.NotNil(t, end)
		require.Contains(t, end.ErrorMessage, errWrite.Error())
		usages := usagesByResponse(h)["resp_1"]
		require.Len(t, usages, 1)
		require.NotEqual(t, own.ID, usages[0])
		require.Len(t, h.rec.RecordedPromptUsages(), 1)
		require.NoError(t, h.sess.Close(nil))
		h.rec.VerifyAllInterceptionsEnded(t)
	})
	t.Run("WriteFailsAfterTerminal", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := newHarness(ctx, t, nil)
		// The accountant blocks on another lane, so resp_1's jobs are still
		// queued when the write fails.
		gate := make(chan struct{})
		h.rec.usageGate.Store(&gate)
		h.forward(created("z", "resp_0", "model-z"), completed("z", "resp_0"))
		_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)
		release, sent := blockWrite(ctx, t, h, create("", "model-create", "first turn"))
		h.forward(created("", "resp_1", "model-create"), completed("", "resp_1"))
		release <- errWrite
		require.ErrorIs(t, codertestutil.TryReceive(ctx, t, sent), errWrite)
		close(gate)
		require.NoError(t, responsesws.Drain(ctx, h.sess))

		own := h.interceptionFor("model-create")
		require.Equal(t, []string{own.ID}, usagesByResponse(h)["resp_1"])
		end := h.rec.RecordedInterceptionEnd(own.ID)
		require.NotNil(t, end)
		require.Contains(t, end.ErrorMessage, errWrite.Error())
		require.Equal(t, 1, h.rec.endCount(own.ID))
		require.NoError(t, h.sess.Close(nil))
		h.rec.VerifyAllInterceptionsEnded(t)
	})
}

// TestContinuationNeverBindsToCreate requires that a steer's automatic
// continuation, which answers no create, never binds to a create, so the
// create keeps its own answer, also while the create is being written.
func TestContinuationNeverBindsToCreate(t *testing.T) {
	t.Parallel()
	const steerAccepted = `{"type":"response.steer.accepted","steer":{"id":"steer_1","previous_response_id":"resp_1"}}`
	// steered opens a session with resp_1 in flight on create "model-a" and
	// a steer of resp_1 accepted.
	steered := func(ctx context.Context, t *testing.T) *harness {
		t.Helper()
		h := newHarness(ctx, t, nil)
		require.NotNil(t, h.send(create("", "model-a", "draft a plan")))
		h.relay(created("", "resp_1", "model-a"))
		require.NotNil(t, h.send(`{"type":"response.steer","previous_response_id":"resp_1","input":"shorter"}`))
		h.relay(steerAccepted, terminal("", "response.incomplete", "resp_1", `,"status":"incomplete","incomplete_details":{"reason":"steered"}`))
		return h
	}
	// writeDuring sends frame with its upstream write blocked while frames
	// arrive from upstream.
	writeDuring := func(ctx context.Context, t *testing.T, h *harness, frame string, frames ...string) {
		t.Helper()
		entered, release := make(chan struct{}), make(chan struct{})
		write := func(context.Context) error {
			close(entered)
			<-release
			return nil
		}
		h.conn.onWrite.Store(&write)
		sent := make(chan error, 1)
		go func() { sent <- h.sess.Send(ctx, []byte(frame)) }()
		_ = codertestutil.TryReceive(ctx, t, entered)
		h.relay(frames...)
		close(release)
		require.NoError(t, codertestutil.TryReceive(ctx, t, sent))
		h.conn.onWrite.Store(nil)
	}

	t.Run("WhileWriting", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := steered(ctx, t)
		writeDuring(ctx, t, h, create("", "model-b", "next"), continuationCreated("", "resp_2", "model-cont", "resp_1"))
		h.relay(created("", "resp_3", "model-b"), completed("", "resp_2"), completed("", "resp_3"))
		require.Equal(t, []string{h.interceptionFor("model-cont").ID}, usagesByResponse(h)["resp_2"])
		require.Equal(t, []string{h.interceptionFor("model-b").ID}, usagesByResponse(h)["resp_3"])
	})
	t.Run("Written", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := steered(ctx, t)
		require.NotNil(t, h.send(create("", "model-b", "next")))
		h.relay(continuationCreated("", "resp_2", "model-cont", "resp_1"), created("", "resp_3", "model-b"),
			completed("", "resp_2"), completed("", "resp_3"))
		require.Equal(t, []string{h.interceptionFor("model-cont").ID}, usagesByResponse(h)["resp_2"])
		require.Equal(t, []string{h.interceptionFor("model-b").ID}, usagesByResponse(h)["resp_3"])
	})
	// Without a previous_response_id, a response arriving while a steer
	// awaits its continuation may be either, so it does not bind to the
	// create being written, which ends without a response.
	t.Run("UnmarkedWhileWriting", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := steered(ctx, t)
		writeDuring(ctx, t, h, create("", "model-b", "next"), created("", "resp_2", "model-cont"))
		h.relay(completed("", "resp_2"))
		require.Equal(t, []string{h.interceptionFor("model-cont").ID}, usagesByResponse(h)["resp_2"])
		require.NotNil(t, h.rec.RecordedInterceptionEnd(h.interceptionFor("model-b").ID))
	})
	// A create continuing the steered response could be answered by the
	// response, which is then either its answer or the continuation. It
	// does not bind to the create being written, which ends without a
	// response.
	t.Run("AmbiguousWhileWriting", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := steered(ctx, t)
		writeDuring(ctx, t, h, `{"type":"response.create","model":"model-b","previous_response_id":"resp_1","input":"next"}`,
			continuationCreated("", "resp_2", "model-cont", "resp_1"))
		h.relay(completed("", "resp_2"))
		require.Equal(t, []string{h.interceptionFor("model-cont").ID}, usagesByResponse(h)["resp_2"])
		require.NotNil(t, h.rec.RecordedInterceptionEnd(h.interceptionFor("model-b").ID))
	})
}

// TestTerminalForUnknownResponseRecordsUsage requires that a terminal
// response no interception owns still has its usage recorded.
func TestTerminalForUnknownResponseRecordsUsage(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)

	h.relay(terminal("z", "response.completed", "resp_orphan", `,"status":"completed","model":"model-orphan"`))
	ic := h.interceptionFor("model-orphan")
	require.Equal(t, map[string][]string{"resp_orphan": {ic.ID}}, usagesByResponse(h))
	end := h.rec.RecordedInterceptionEnd(ic.ID)
	require.NotNil(t, end)
	require.Empty(t, end.ErrorType)
}

// TestCreateRecordsCorrelatingToolCallID requires the call ID of a trailing
// function_call_output input item on the interception, as the HTTP Responses
// interceptor records it.
func TestCreateRecordsCorrelatingToolCallID(t *testing.T) {
	t.Parallel()
	h := newHarness(codertestutil.Context(t, codertestutil.WaitShort), t, nil)

	h.send(`{"type":"response.create","model":"tool-result","input":[{"type":"function_call","call_id":"call_1","name":"f","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"42"}]}`)
	h.send(`{"type":"response.create","model":"tool-then-user","input":[{"type":"function_call_output","call_id":"call_2","output":"42"},{"role":"user","content":"thanks"}]}`)
	h.send(create("", "plain", "hi"))
	require.Equal(t, ptr("call_1"), h.interceptionFor("tool-result").CorrelatingToolCallID)
	require.Nil(t, h.interceptionFor("tool-then-user").CorrelatingToolCallID)
	require.Nil(t, h.interceptionFor("plain").CorrelatingToolCallID)
}

// TestToolResultCreateAwaitsToolCallRecord requires that a create answering
// a tool call is recorded only after the tool call, which the client
// received in an earlier response: the interception's parent is found
// through the tool call record, so a client answering at once must not
// overtake the accountant.
func TestToolResultCreateAwaitsToolCallRecord(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	h := newHarness(ctx, t, nil)
	require.NotNil(t, h.send(create("", "gpt-6", "hi")))
	h.relay(created("", "resp_1", "gpt-6"))

	// The accountant records the tool call after the token usage, which
	// blocks.
	gate := make(chan struct{})
	h.rec.usageGate.Store(&gate)
	h.forward(terminal("", "response.completed", "resp_1", `,"status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"f","arguments":"{}"}]`))
	_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)

	sent := make(chan error, 1)
	go func() {
		sent <- h.sess.Send(ctx, []byte(`{"type":"response.create","model":"tool-result","input":[{"type":"function_call_output","call_id":"call_1","output":"42"}]}`))
	}()
	select {
	case err := <-sent:
		t.Fatalf("create recorded before the tool call it answers: %v", err)
	case <-time.After(codertestutil.IntervalMedium):
	}
	close(gate)
	require.NoError(t, codertestutil.TryReceive(ctx, t, sent))
	require.Len(t, h.rec.RecordedToolUsages(), 1)
	require.Equal(t, ptr("call_1"), h.interceptionFor("tool-result").CorrelatingToolCallID)
}

// toolCallCompleted is a response.completed for response id that announces
// the tool call callID.
func toolCallCompleted(streamID, id, callID string) string {
	return terminal(streamID, "response.completed", id, fmt.Sprintf(`,"status":"completed","output":[{"type":"function_call","id":"fc_%s","call_id":%q,"name":"f","arguments":"{}"}]`, callID, callID))
}

// toolResult is a create answering the tool call callID.
func toolResult(model, callID string) string {
	return fmt.Sprintf(`{"type":"response.create","model":%q,"input":[{"type":"function_call_output","call_id":%q,"output":"42"}]}`, model, callID)
}

const toolResultTimeoutLog = "recording a tool result before the tool call it answers"

// TestToolResultCreateWaitIsBounded requires that a create waiting for the
// record of the tool call it answers never waits longer than one record
// timeout, and stops waiting as soon as its caller or the session ends.
func TestToolResultCreateWaitIsBounded(t *testing.T) {
	t.Parallel()
	t.Run("RecordTimeout", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitMedium)
		h := newHarness(ctx, t, nil)
		require.NotNil(t, h.send(create("a", "model-a", "hi")))
		require.NotNil(t, h.send(create("b", "model-b", "hi")))
		h.relay(created("a", "resp_a", "model-a"), created("b", "resp_b", "model-b"))
		gate := make(chan struct{})
		t.Cleanup(func() { close(gate) })
		h.rec.usageGate.Store(&gate)
		// Each usage record blocks for a full record timeout, and the tool
		// call is recorded after resp_b's usage, which is queued behind
		// resp_a's: about two record timeouts from now.
		h.forward(completed("a", "resp_a"), toolCallCompleted("b", "resp_b", "call_1"))
		_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)

		require.NoError(t, h.sess.Send(ctx, []byte(toolResult("tool-result", "call_1"))))
		require.Empty(t, h.rec.RecordedToolUsages(), "create waited for the tool call record past the record timeout")
		require.Equal(t, 1, h.logs.count(toolResultTimeoutLog))
		require.Equal(t, ptr("call_1"), h.interceptionFor("tool-result").CorrelatingToolCallID)
	})
	// blockedToolCall announces call_1 in a response whose accounting
	// blocks until the returned gate is closed.
	blockedToolCall := func(ctx context.Context, t *testing.T, h *harness) chan struct{} {
		t.Helper()
		require.NotNil(t, h.send(create("", "gpt-6", "hi")))
		h.relay(created("", "resp_1", "gpt-6"))
		gate := make(chan struct{})
		h.rec.usageGate.Store(&gate)
		h.forward(toolCallCompleted("", "resp_1", "call_1"))
		_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)
		return gate
	}
	t.Run("CallerCanceled", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := newHarness(ctx, t, nil)
		gate := blockedToolCall(ctx, t, h)
		defer close(gate)
		sendCtx, cancel := context.WithCancel(ctx)
		sent := make(chan error, 1)
		go func() { sent <- h.sess.Send(sendCtx, []byte(toolResult("tool-result", "call_1"))) }()
		select {
		case err := <-sent:
			t.Fatalf("create did not wait for the tool call record: %v", err)
		case <-time.After(codertestutil.IntervalMedium):
		}
		cancel()
		require.ErrorIs(t, codertestutil.TryReceive(ctx, t, sent), context.Canceled)
		require.Zero(t, h.logs.count(toolResultTimeoutLog))
	})
	t.Run("SessionClosed", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := newHarness(ctx, t, nil)
		gate := blockedToolCall(ctx, t, h)
		sent := make(chan error, 1)
		go func() { sent <- h.sess.Send(ctx, []byte(toolResult("tool-result", "call_1"))) }()
		select {
		case err := <-sent:
			t.Fatalf("create did not wait for the tool call record: %v", err)
		case <-time.After(codertestutil.IntervalMedium):
		}
		start := time.Now()
		closed := make(chan error, 1)
		go func() { closed <- h.sess.Close(nil) }()
		require.ErrorIs(t, codertestutil.TryReceive(ctx, t, sent), responsesws.ErrClosed)
		require.Less(t, time.Since(start), recorder.DefaultAsyncTimeout)
		close(gate)
		require.NoError(t, codertestutil.TryReceive(ctx, t, closed))
		require.Zero(t, h.logs.count(toolResultTimeoutLog))
	})
}

// TestToolResultCreateWaitsOnlyForAccountedToolCalls requires that a create
// waits only for a tool call this session is still accounting for. A call
// ID the session never saw, or one whose interception ended, recorded or
// not, never delays a create, and an interception that loses its terminal
// job to overload still releases the creates waiting for it.
func TestToolResultCreateWaitsOnlyForAccountedToolCalls(t *testing.T) {
	t.Parallel()
	t.Run("NotAccounted", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := newHarness(ctx, t, nil)
		// call_done was recorded and its interception ended.
		require.NotNil(t, h.send(create("a", "model-a", "hi")))
		h.relay(created("a", "resp_a", "model-a"), toolCallCompleted("a", "resp_a", "call_done"))
		// call_lost belongs to a response whose interception could never
		// be recorded, so its tool call never will be.
		h.rec.failStart.Store(true)
		h.relay(created("z", "resp_z", "model-z"), toolCallCompleted("z", "resp_z", "call_lost"))
		h.rec.failStart.Store(false)
		require.Equal(t, map[string]int{}, responsesws.StateSizes(h.sess))

		// The accountant blocks on an unrelated response from here on.
		require.NotNil(t, h.send(create("b", "model-b", "hi")))
		h.relay(created("b", "resp_b", "model-b"))
		gate := make(chan struct{})
		defer close(gate)
		h.rec.usageGate.Store(&gate)
		h.forward(completed("b", "resp_b"))
		_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)

		// The blocked usage record gives up after one record timeout, so a
		// create that waited for the accountant would take about that long.
		start := time.Now()
		for _, callID := range []string{"call_unknown", "call_done", "call_lost"} {
			require.NotNil(t, h.send(toolResult("tool-result-"+callID, callID)), callID)
		}
		require.Less(t, time.Since(start), recorder.DefaultAsyncTimeout/2)
		require.Zero(t, h.logs.count(toolResultTimeoutLog))
	})
	t.Run("Overloaded", func(t *testing.T) {
		t.Parallel()
		ctx := codertestutil.Context(t, codertestutil.WaitShort)
		h := newHarness(ctx, t, nil)
		responsesws.SetQueueBounds(h.sess, 4, extract.MaxEventBytes)
		gate := make(chan struct{})
		h.rec.usageGate.Store(&gate)
		h.forward(created("z", "resp_0", "model-0"), completed("z", "resp_0"))
		// The accountant is blocked on resp_0's usage with an empty queue.
		_ = codertestutil.TryReceive(ctx, t, h.rec.usageEntered)
		// The start and created jobs of two responses fill the queue, so
		// resp_1's terminal job is dropped and its interception is handed
		// off.
		h.forward(created("y", "resp_1", "model-1"), created("x", "resp_2", "model-2"))
		h.forward(toolCallCompleted("y", "resp_1", "call_1"))
		require.Equal(t, 1, h.logs.count("accounting queue full: dropped event"))

		sent := make(chan error, 1)
		go func() { sent <- h.sess.Send(ctx, []byte(toolResult("tool-result", "call_1"))) }()
		select {
		case err := <-sent:
			t.Fatalf("create did not wait for the handed-off interception: %v", err)
		case <-time.After(codertestutil.IntervalMedium):
		}
		close(gate)
		require.NoError(t, codertestutil.TryReceive(ctx, t, sent))
		require.Zero(t, h.logs.count(toolResultTimeoutLog))
		require.NoError(t, responsesws.Drain(ctx, h.sess))
		require.NotContains(t, responsesws.StateSizes(h.sess), "toolCalls")
		end := h.rec.RecordedInterceptionEnd(h.interceptionFor("model-1").ID)
		require.NotNil(t, end)
		require.Contains(t, end.ErrorMessage, "accounting queue overloaded")
	})
}

// TestCloseBoundedWhenRecorderBlocks requires that shutdown shares one
// cleanup deadline: with 16 open interceptions, queued accounting, and a
// recorder whose usage and end records block until their ctx ends, Close
// returns within the bound and the accountant exits (checked by goleak).
func TestCloseBoundedWhenRecorderBlocks(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitMedium)
	h := newHarness(ctx, t, nil)
	for i := range 16 {
		require.NotNil(t, h.send(create(fmt.Sprint(i), "gpt-6", "hi")))
	}
	for i := range 8 {
		h.relay(created(fmt.Sprint(i), fmt.Sprintf("resp_%d", i), "gpt-6"))
	}
	h.rec.blockEnd.Store(true)
	h.rec.blockUsage.Store(true)
	// Queue terminal events behind a blocked usage record.
	for i := range 8 {
		h.forward(completed(fmt.Sprint(i), fmt.Sprintf("resp_%d", i)))
	}

	closed := make(chan error, 1)
	start := time.Now()
	go func() { closed <- h.sess.Close(nil) }()
	require.NoError(t, codertestutil.TryReceive(ctx, t, closed))
	require.Less(t, time.Since(start), recorder.DefaultAsyncTimeout+codertestutil.WaitShort)
	for _, ic := range h.rec.RecordedInterceptions() {
		require.Equal(t, 1, h.rec.endCount(ic.ID))
	}
}

// TestSlowClientBackpressure requires that upstream frames read ahead of
// Recv are bounded by bytes as well as by count: once the buffered frames
// reach the byte bound the reader stops reading upstream, so a client that
// reads slowly slows its own upstream instead of growing the buffer. A frame
// larger than the bound still passes on its own.
func TestSlowClientBackpressure(t *testing.T) {
	t.Parallel()
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	h := newHarness(ctx, t, nil)

	// Two frames fit the byte bound, three do not.
	pad := strings.Repeat("x", responsesws.MaxBufferedFrameBytes/3)
	frames := make([]string, 6)
	for i := range frames {
		frames[i] = fmt.Sprintf(`{"type":"response.output_text.delta","delta":"%d%s"}`, i, pad)
		codertestutil.RequireSend(ctx, t, h.conn.toClient, []byte(frames[i]))
	}
	size := len(frames[0])
	// requireState waits until the reader read all but unread frames and
	// holds buffered of them for Recv. The reader always holds one more
	// frame it read, waiting for room.
	requireState := func(unread, buffered int) {
		t.Helper()
		require.True(t, codertestutil.Eventually(ctx, t, func(context.Context) bool {
			return len(h.conn.toClient) == unread && responsesws.BufferedFrameBytes(h.sess) == buffered*size
		}, codertestutil.IntervalFast), "want %d unread and %d buffered frames", unread, buffered)
		require.LessOrEqual(t, responsesws.BufferedFrameBytes(h.sess), responsesws.MaxBufferedFrameBytes)
	}

	requireState(3, 2)
	for i, want := range frames {
		got, err := h.sess.Recv(ctx)
		require.NoError(t, err)
		require.Equal(t, want, string(got))
		received := i + 1
		requireState(max(0, len(frames)-received-3), min(2, len(frames)-received))
	}

	big := fmt.Sprintf(`{"type":"response.output_text.delta","delta":%q}`, strings.Repeat("y", 2*responsesws.MaxBufferedFrameBytes))
	h.forward(big)
}
