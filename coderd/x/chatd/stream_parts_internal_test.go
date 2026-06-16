package chatd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestStreamPartsEndpointReplayLiveAndReselect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	endpoint := streamPartsEndpoint{
		chatID: chatID,
		buffer: buffer,
		logger: slogtest.Make(t, nil),
	}
	serverTransport, clientTransport := newStreamPartsChannelTransportPair()
	serveDone := serveStreamPartsEndpoint(ctx, t, endpoint, serverTransport)
	defer func() {
		require.NoError(t, clientTransport.Close())
		<-serveDone
	}()

	firstKey := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 1, GenerationAttempt: 1}
	secondKey := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 2, GenerationAttempt: 1}
	require.NoError(t, buffer.CreateEpisode(firstKey))
	require.NoError(t, buffer.AddPart(firstKey, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("replayed")))
	require.NoError(t, buffer.CreateEpisode(secondKey))
	require.NoError(t, buffer.AddPart(secondKey, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("second")))

	require.NoError(t, clientTransport.WriteControl(ctx, streamPartsControl{HistoryVersion: 1, GenerationAttempt: 1}))
	got := readStreamPartsTransportBatch(ctx, t, clientTransport)
	require.Len(t, got, 1)
	require.Equal(t, "replayed", got[0].MessagePart.Part.Text)
	require.Equal(t, int64(1), got[0].MessagePart.Seq)
	require.Equal(t, int64(1), got[0].MessagePart.HistoryVersion)
	require.Equal(t, int64(1), got[0].MessagePart.GenerationAttempt)

	require.NoError(t, buffer.AddPart(firstKey, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
	got = readStreamPartsTransportBatch(ctx, t, clientTransport)
	require.Len(t, got, 1)
	require.Equal(t, "live", got[0].MessagePart.Part.Text)
	require.Equal(t, int64(2), got[0].MessagePart.Seq)

	require.NoError(t, clientTransport.WriteControl(ctx, streamPartsControl{HistoryVersion: 2, GenerationAttempt: 1}))
	got = readStreamPartsTransportBatch(ctx, t, clientTransport)
	require.Len(t, got, 1)
	require.Equal(t, "second", got[0].MessagePart.Part.Text)
	require.Equal(t, int64(2), got[0].MessagePart.HistoryVersion)

	require.NoError(t, buffer.AddPart(firstKey, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("ignored")))
	select {
	case <-ctx.Done():
		t.Fatal("timed out waiting to verify previous episode was canceled")
	default:
	}
	require.NoError(t, buffer.AddPart(secondKey, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("second-live")))
	got = readStreamPartsTransportBatch(ctx, t, clientTransport)
	require.Equal(t, "second-live", got[0].MessagePart.Part.Text)
}

func TestStreamPartsEndpointWaitsForMissingEpisode(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	endpoint := streamPartsEndpoint{
		chatID: chatID,
		buffer: buffer,
		logger: slogtest.Make(t, nil),
	}
	serverTransport, clientTransport := newStreamPartsChannelTransportPair()
	serveDone := serveStreamPartsEndpoint(ctx, t, endpoint, serverTransport)
	defer func() {
		require.NoError(t, clientTransport.Close())
		<-serveDone
	}()

	key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 9, GenerationAttempt: 2}
	require.NoError(t, clientTransport.WriteControl(ctx, streamPartsControl{HistoryVersion: key.HistoryVersion, GenerationAttempt: key.GenerationAttempt}))
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("eventual")))

	got := readStreamPartsTransportBatch(ctx, t, clientTransport)
	require.Len(t, got, 1)
	require.Equal(t, "eventual", got[0].MessagePart.Part.Text)
	require.Equal(t, int64(9), got[0].MessagePart.HistoryVersion)
	require.Equal(t, int64(2), got[0].MessagePart.GenerationAttempt)
}

func TestStreamPartsEndpointReselectsWhileEpisodeMissing(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	endpoint := streamPartsEndpoint{
		chatID: chatID,
		buffer: buffer,
		logger: slogtest.Make(t, nil),
	}
	serverTransport, clientTransport := newStreamPartsChannelTransportPair()
	serveDone := serveStreamPartsEndpoint(ctx, t, endpoint, serverTransport)
	defer func() {
		require.NoError(t, clientTransport.Close())
		<-serveDone
	}()

	missingKey := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 10, GenerationAttempt: 1}
	selectedKey := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 11, GenerationAttempt: 1}
	require.NoError(t, clientTransport.WriteControl(ctx, streamPartsControl{HistoryVersion: missingKey.HistoryVersion, GenerationAttempt: missingKey.GenerationAttempt}))
	require.NoError(t, clientTransport.WriteControl(ctx, streamPartsControl{HistoryVersion: selectedKey.HistoryVersion, GenerationAttempt: selectedKey.GenerationAttempt}))
	require.NoError(t, buffer.CreateEpisode(selectedKey))
	require.NoError(t, buffer.AddPart(selectedKey, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("selected")))

	got := readStreamPartsTransportBatch(ctx, t, clientTransport)
	require.Len(t, got, 1)
	require.Equal(t, "selected", got[0].MessagePart.Part.Text)
	require.Equal(t, selectedKey.HistoryVersion, got[0].MessagePart.HistoryVersion)
}

func TestStreamPartsEndpointClientDisconnectCancels(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	endpoint := streamPartsEndpoint{
		chatID: chatID,
		buffer: buffer,
		logger: slogtest.Make(t, nil),
	}
	serverTransport, clientTransport := newStreamPartsChannelTransportPair()
	serveDone := serveStreamPartsEndpoint(ctx, t, endpoint, serverTransport)
	require.NoError(t, clientTransport.Close())

	select {
	case <-serveDone:
	case <-ctx.Done():
		t.Fatal("stream parts endpoint did not exit after client disconnect")
	}
}

func TestStreamPartsEndpointWebSocket(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	endpoint := streamPartsEndpoint{
		chatID: chatID,
		buffer: buffer,
		logger: slogtest.Make(t, nil),
	}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_ = endpoint.serveWebSocket(rw, r)
	}))
	defer server.Close()

	key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 1, GenerationAttempt: 1}
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("websocket")))

	conn, resp, err := websocket.Dial(ctx, server.URL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		require.NoError(t, resp.Body.Close())
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	require.NoError(t, wsjson.Write(ctx, conn, streamPartsControl{HistoryVersion: 1, GenerationAttempt: 1}))
	got := readStreamPartsWebSocketBatch(ctx, t, conn)
	require.Len(t, got, 1)
	require.Equal(t, "websocket", got[0].MessagePart.Part.Text)
}

func TestStreamPartsWebSocketSession(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	endpoint := streamPartsEndpoint{
		chatID: chatID,
		buffer: buffer,
		logger: slogtest.Make(t, nil),
	}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_ = endpoint.serveWebSocket(rw, r)
	}))
	defer server.Close()

	key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 4, GenerationAttempt: 2}
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("session")))

	conn, resp, err := websocket.Dial(ctx, server.URL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		require.NoError(t, resp.Body.Close())
	}
	session := NewStreamPartsJSONSession(ctx, conn)
	defer session.Close()

	require.NoError(t, session.SelectEpisode(ctx, key.HistoryVersion, key.GenerationAttempt))
	part := readStreamPart(ctx, t, session.Parts())
	require.Equal(t, key.HistoryVersion, part.HistoryVersion)
	require.Equal(t, key.GenerationAttempt, part.GenerationAttempt)
	require.Equal(t, int64(1), part.Seq)
	require.Equal(t, "session", part.Part.Text)
}

func TestLocalStreamPartsDialerReplayLiveAndClose(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := uuid.New()
	buffer := messagepartbuffer.New(messagepartbuffer.Options{})
	defer buffer.Close()
	dialer := NewLocalStreamPartsDialer(LocalStreamPartsDialerConfig{
		Buffer: buffer,
		Logger: slogtest.Make(t, nil),
	})
	key := messagepartbuffer.Key{ChatID: chatID, HistoryVersion: 3, GenerationAttempt: 1}
	require.NoError(t, buffer.CreateEpisode(key))
	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("replayed")))

	session, err := dialer(ctx, StreamPartsDialInput{ChatID: chatID, WorkerID: uuid.New()})
	require.NoError(t, err)
	require.NoError(t, session.SelectEpisode(ctx, key.HistoryVersion, key.GenerationAttempt))

	part := readStreamPart(ctx, t, session.Parts())
	require.Equal(t, int64(1), part.Seq)
	require.Equal(t, "replayed", part.Part.Text)

	require.NoError(t, buffer.AddPart(key, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageText("live")))
	part = readStreamPart(ctx, t, session.Parts())
	require.Equal(t, int64(2), part.Seq)
	require.Equal(t, "live", part.Part.Text)

	require.NoError(t, session.Close())
	select {
	case _, ok := <-session.Parts():
		require.False(t, ok)
	case <-ctx.Done():
		t.Fatal("stream parts session did not close")
	}
}

func TestStreamPartsDialerForServer(t *testing.T) {
	t.Parallel()

	serverWorkerID := uuid.New()
	remoteWorkerID := uuid.New()

	cases := []struct {
		name     string
		remote   bool
		workerID uuid.UUID
		want     string
	}{
		{name: "no remote uses local", workerID: remoteWorkerID, want: "local"},
		{name: "same worker uses local", remote: true, workerID: serverWorkerID, want: "local"},
		{name: "different worker uses remote", remote: true, workerID: remoteWorkerID, want: "remote"},
		{name: "nil worker uses local", remote: true, workerID: uuid.Nil, want: "local"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			called := make(chan string, 1)
			local := func(context.Context, StreamPartsDialInput) (StreamPartsSession, error) {
				called <- "local"
				return nil, xerrors.New("local")
			}
			var remote StreamPartsDialer
			if tc.remote {
				remote = func(context.Context, StreamPartsDialInput) (StreamPartsSession, error) {
					called <- "remote"
					return nil, xerrors.New("remote")
				}
			}
			dialer := streamPartsDialerForServer(serverWorkerID, local, remote)
			_, _ = dialer(ctx, StreamPartsDialInput{WorkerID: tc.workerID})
			require.Equal(t, tc.want, <-called)
		})
	}
}

func serveStreamPartsEndpoint(ctx context.Context, t *testing.T, endpoint streamPartsEndpoint, transport streamPartsServerTransport) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := endpoint.serve(ctx, transport)
		if err != nil && !streamPartsExpectedTransportClose(err) {
			require.NoError(t, err)
		}
	}()
	return done
}

func readStreamPartsTransportBatch(ctx context.Context, t *testing.T, transport streamPartsClientTransport) []codersdk.ChatStreamEvent {
	t.Helper()
	got, err := transport.ReadEvents(ctx)
	require.NoError(t, err)
	assertStreamPartsBatch(t, got)
	return got
}

func readStreamPartsWebSocketBatch(ctx context.Context, t *testing.T, conn *websocket.Conn) []codersdk.ChatStreamEvent {
	t.Helper()
	var got []codersdk.ChatStreamEvent
	require.NoError(t, wsjson.Read(ctx, conn, &got))
	assertStreamPartsBatch(t, got)
	return got
}

func assertStreamPartsBatch(t *testing.T, got []codersdk.ChatStreamEvent) {
	t.Helper()
	for _, event := range got {
		require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, event.Type)
		require.NotNil(t, event.MessagePart)
	}
}

func readStreamPart(ctx context.Context, t *testing.T, parts <-chan StreamPart) StreamPart {
	t.Helper()
	select {
	case part, ok := <-parts:
		require.True(t, ok)
		return part
	case <-ctx.Done():
		t.Fatal("timed out waiting for stream part")
		return StreamPart{}
	}
}
