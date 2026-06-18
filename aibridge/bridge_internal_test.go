package aibridge

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

func TestExtractAgentFirewallHeaders(t *testing.T) {
	t.Parallel()

	t.Run("both headers present", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
		require.NoError(t, err)
		req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "e5f6a7b8-1234-5678-9abc-def012345678")
		req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, "42")

		sessionID, seqNumber, extractErr := extractAgentFirewallHeaders(req)

		require.NoError(t, extractErr)
		require.NotNil(t, sessionID)
		assert.Equal(t, "e5f6a7b8-1234-5678-9abc-def012345678", *sessionID)
		require.NotNil(t, seqNumber)
		assert.Equal(t, int32(42), *seqNumber)
	})

	t.Run("no headers present", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
		require.NoError(t, err)

		sessionID, seqNumber, extractErr := extractAgentFirewallHeaders(req)

		require.NoError(t, extractErr)
		assert.Nil(t, sessionID)
		assert.Nil(t, seqNumber)
	})

	t.Run("only session ID returns error", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
		require.NoError(t, err)
		req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "e5f6a7b8-1234-5678-9abc-def012345678")

		sessionID, seqNumber, extractErr := extractAgentFirewallHeaders(req)

		require.Error(t, extractErr)
		assert.Contains(t, extractErr.Error(), "without sequence number")
		assert.Nil(t, sessionID)
		assert.Nil(t, seqNumber)
	})

	t.Run("only sequence number returns error", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
		require.NoError(t, err)
		req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, "7")

		sessionID, seqNumber, extractErr := extractAgentFirewallHeaders(req)

		require.Error(t, extractErr)
		assert.Contains(t, extractErr.Error(), "without session ID")
		assert.Nil(t, sessionID)
		assert.Nil(t, seqNumber)
	})

	t.Run("sequence number zero", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
		require.NoError(t, err)
		req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "e5f6a7b8-1234-5678-9abc-def012345678")
		req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, "0")

		sessionID, seqNumber, extractErr := extractAgentFirewallHeaders(req)

		require.NoError(t, extractErr)
		require.NotNil(t, sessionID)
		assert.Equal(t, "e5f6a7b8-1234-5678-9abc-def012345678", *sessionID)
		require.NotNil(t, seqNumber)
		assert.Equal(t, int32(0), *seqNumber)
	})

	t.Run("invalid sequence number returns error", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
		require.NoError(t, err)
		req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "e5f6a7b8-1234-5678-9abc-def012345678")
		req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, "not-a-number")

		sessionID, seqNumber, extractErr := extractAgentFirewallHeaders(req)

		require.Error(t, extractErr)
		assert.Contains(t, extractErr.Error(), "invalid agent firewall sequence number")
		assert.Nil(t, sessionID)
		assert.Nil(t, seqNumber)
	})

	t.Run("sequence number exceeding int32 range returns error", func(t *testing.T) {
		t.Parallel()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
		require.NoError(t, err)
		req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "e5f6a7b8-1234-5678-9abc-def012345678")
		req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, "2147483648") // max int32 + 1

		sessionID, seqNumber, extractErr := extractAgentFirewallHeaders(req)

		require.Error(t, extractErr)
		assert.Contains(t, extractErr.Error(), "invalid agent firewall sequence number")
		assert.Nil(t, sessionID)
		assert.Nil(t, seqNumber)
	})
}
