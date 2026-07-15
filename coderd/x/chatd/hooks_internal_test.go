package chatd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/codersdk/agenthooks"
	"github.com/coder/coder/v2/testutil"
)

func TestSessionStartDispatchSources(t *testing.T) {
	t.Parallel()

	const secret = "test-hook-secret-32-bytes-minimum!!"
	type received struct {
		request agenthooks.Request
		claims  agenthooks.Claims
		data    *agenthooks.SessionStartData
	}
	receivedCh := make(chan received, 2)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		claims, err := agenthooks.Verify(r.Header.Get("Authorization"), []byte(secret))
		require.NoError(t, err)
		decoded, err := request.Decode()
		require.NoError(t, err)
		data, ok := decoded.(*agenthooks.SessionStartData)
		require.True(t, ok)
		receivedCh <- received{request: request, claims: claims, data: data}
		_, err = w.Write([]byte(`{}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)

	db, _ := dbtestutil.NewDB(t)
	dispatcher := chathooks.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		db,
		consumer.Client(),
		consumer.URL,
		secret,
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
	server := &Server{hookDispatcher: dispatcher}
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{OwnerID: user.ID, OrganizationID: org.ID, LastModelConfigID: model.ID})
	turnID := uuid.New()
	ctx := testutil.Context(t, testutil.WaitLong)

	_, err := server.dispatchSessionStart(ctx, chat, &turnID, sessionStartSource(nil))
	require.NoError(t, err)
	_, err = server.dispatchSessionStart(ctx, chat, &turnID, sessionStartSource([]database.ChatMessage{{Role: database.ChatMessageRoleAssistant}}))
	require.NoError(t, err)

	startup := <-receivedCh
	resume := <-receivedCh
	require.Equal(t, agenthooks.EventSessionStart, startup.request.Type)
	require.Equal(t, sessionStartSourceStartup, startup.data.Source)
	require.Equal(t, startup.request.Meta.DispatchID, startup.claims.JTI)
	require.Equal(t, agenthooks.EventSessionStart, resume.request.Type)
	require.Equal(t, sessionStartSourceResume, resume.data.Source)
	require.Equal(t, resume.request.Meta.DispatchID, resume.claims.JTI)
	require.NotEqual(t, startup.claims.JTI, resume.claims.JTI)
}
