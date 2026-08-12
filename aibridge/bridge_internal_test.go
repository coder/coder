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

	const validSessionID = "e5f6a7b8-1234-5678-9abc-def012345678"

	ptr := func(s string) *string { return &s }

	cases := []struct {
		name string
		// sessionID and seqNumber set the corresponding headers when
		// non-nil. A nil value leaves the header unset.
		sessionID *string
		seqNumber *string

		wantErr     bool
		errContains string
		wantSession *string
		wantSeq     *int32
	}{
		{
			name:        "both headers present",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("42"),
			wantSession: ptr(validSessionID),
			wantSeq:     int32Ptr(42),
		},
		{
			name: "no headers present",
		},
		{
			name:        "only session ID returns error",
			sessionID:   ptr(validSessionID),
			wantErr:     true,
			errContains: "without sequence number",
		},
		{
			name:        "only sequence number returns error",
			seqNumber:   ptr("7"),
			wantErr:     true,
			errContains: "without session ID",
		},
		{
			name:        "sequence number zero",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("0"),
			wantSession: ptr(validSessionID),
			wantSeq:     int32Ptr(0),
		},
		{
			name:        "invalid session ID returns error",
			sessionID:   ptr("not-a-uuid"),
			seqNumber:   ptr("42"),
			wantErr:     true,
			errContains: "invalid agent firewall session ID",
		},
		{
			name:        "invalid sequence number returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("not-a-number"),
			wantErr:     true,
			errContains: "invalid agent firewall sequence number",
		},
		{
			name:        "negative sequence number returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("-1"),
			wantErr:     true,
			errContains: "must be non-negative",
		},
		{
			name:        "sequence number exceeding int32 range returns error",
			sessionID:   ptr(validSessionID),
			seqNumber:   ptr("2147483648"), // max int32 + 1
			wantErr:     true,
			errContains: "invalid agent firewall sequence number",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
			require.NoError(t, err)
			if tc.sessionID != nil {
				req.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, *tc.sessionID)
			}
			if tc.seqNumber != nil {
				req.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, *tc.seqNumber)
			}

			sessionID, seqNumber, extractErr := extractAgentFirewallHeaders(req)

			if tc.wantErr {
				require.Error(t, extractErr)
				assert.Contains(t, extractErr.Error(), tc.errContains)
				assert.Nil(t, sessionID)
				assert.Nil(t, seqNumber)
				return
			}

			require.NoError(t, extractErr)
			if tc.wantSession == nil {
				assert.Nil(t, sessionID)
			} else {
				require.NotNil(t, sessionID)
				assert.Equal(t, *tc.wantSession, *sessionID)
			}
			if tc.wantSeq == nil {
				assert.Nil(t, seqNumber)
			} else {
				require.NotNil(t, seqNumber)
				assert.Equal(t, *tc.wantSeq, *seqNumber)
			}
		})
	}
}

func int32Ptr(n int32) *int32 { return &n }
