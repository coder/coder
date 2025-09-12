package aibridged_test

import (
	"bytes"
	_ "embed"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/aibridge"
	"github.com/coder/coder/v2/enterprise/x/aibridged"
	mock "github.com/coder/coder/v2/enterprise/x/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/enterprise/x/aibridged/proto"
	"github.com/coder/coder/v2/testutil"
)

// TestPool validates the published behavior of [aibridged.CachedBridgePool].
// It is not meant to be an exhaustive test of the internal cache's functionality,
// since that is already covered by its library.
func TestPool(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)

	srv := httptest.NewServer(&mockAIUpstreamServer{})

	client.EXPECT().GetMCPServerConfigs(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.GetMCPServerConfigsResponse{}, nil)

	pool, err := aibridged.NewCachedBridgePool(1, aibridge.Config{
		OpenAI: aibridge.ProviderConfig{
			BaseURL: srv.URL,
		},
	}, logger)
	require.NoError(t, err)

	id := uuid.New()
	_, err = pool.Acquire(t.Context(), aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id,
	}, func() (aibridged.DRPCClient, error) {
		return client, nil
	})
	require.NoError(t, err, "acquire pool instance")

	req, err := http.NewRequestWithContext(testutil.Context(t, testutil.WaitShort), http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{
  "messages": [
    {
      "role": "user",
      "content": "how many angels can dance on the head of a pin\n"
    }
  ],
  "model": "gpt-4.1"
}`))
	require.NoError(t, err)
	req.Header.Add("Authorization", "Bearer key")

	// rec := httptest.NewRecorder()
	// h.ServeHTTP(rec, req)

	// TODO: incomplete.
}
