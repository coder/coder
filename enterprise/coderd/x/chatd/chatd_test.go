package chatd_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	osschatd "github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	entchatd "github.com/coder/coder/v2/enterprise/coderd/x/chatd"
	"github.com/coder/websocket"
)

func TestRelayDialErrorIsUnrecoverable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"unauthorized", http.StatusUnauthorized, true},
		{"forbidden", http.StatusForbidden, true},
		{"internal_server", http.StatusInternalServerError, false},
		{"bad_gateway", http.StatusBadGateway, false},
		{"pre_response", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := &entchatd.RelayDialError{HTTPStatus: tc.status, Err: context.Canceled}
			require.Equal(t, tc.want, err.IsUnrecoverable())
		})
	}
}

func TestStreamPartsDialerDialsPartsEndpoint(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	workerID := uuid.New()
	replicaID := uuid.New()
	received := make(chan http.Header, 1)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v2/chats/"+chatID.String()+"/stream/parts", r.URL.Path)
		require.Empty(t, r.URL.RawQuery)
		received <- r.Header.Clone()
		conn, err := websocket.Accept(rw, r, nil)
		require.NoError(t, err)
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	t.Cleanup(server.Close)

	dialer := entchatd.NewStreamPartsDialer(entchatd.StreamPartsDialerConfig{
		ResolveReplicaAddress: func(_ context.Context, gotWorker uuid.UUID) (string, bool) {
			require.Equal(t, workerID, gotWorker)
			return server.URL, true
		},
		ReplicaHTTPClient: server.Client(),
		ReplicaIDFn:       func() uuid.UUID { return replicaID },
	})

	session, err := dialer(context.Background(), osschatd.StreamPartsDialInput{
		ChatID:   chatID,
		WorkerID: workerID,
		RequestHeader: http.Header{
			codersdk.SessionTokenHeader: {"session-token"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, session)
	require.NoError(t, session.Close())

	headers := <-received
	require.Equal(t, "session-token", headers.Get(codersdk.SessionTokenHeader))
	require.Equal(t, replicaID.String(), headers.Get(entchatd.RelaySourceHeader))
}

func TestStreamPartsDialerClassifiesHTTPFailures(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	workerID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		http.Error(rw, "nope", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	dialer := entchatd.NewStreamPartsDialer(entchatd.StreamPartsDialerConfig{
		ResolveReplicaAddress: func(context.Context, uuid.UUID) (string, bool) { return server.URL, true },
		ReplicaHTTPClient:     server.Client(),
		ReplicaIDFn:           uuid.New,
	})

	session, err := dialer(context.Background(), osschatd.StreamPartsDialInput{
		ChatID:   chatID,
		WorkerID: workerID,
	})
	require.Nil(t, session)
	var dialErr *entchatd.RelayDialError
	require.ErrorAs(t, err, &dialErr)
	require.Equal(t, http.StatusUnauthorized, dialErr.HTTPStatus)
	require.True(t, dialErr.IsUnrecoverable())
}
