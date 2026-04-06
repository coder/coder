package agentgit_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func TestExtractChatContext(t *testing.T) {
	t.Parallel()

	validID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	ancestor1 := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	ancestor2 := uuid.MustParse("66666666-7777-8888-9999-aaaaaaaaaaaa")

	tests := []struct {
		name            string
		chatID          string // empty means header not set
		setChatID       bool   // whether to set the chat ID header at all
		ancestors       string // empty means header not set
		setAncestors    bool   // whether to set the ancestor header at all
		wantChatID      uuid.UUID
		wantAncestorIDs []uuid.UUID
		wantOK          bool
	}{
		{
			name:            "NoHeadersPresent",
			setChatID:       false,
			setAncestors:    false,
			wantChatID:      uuid.Nil,
			wantAncestorIDs: nil,
			wantOK:          false,
		},
		{
			name:            "ValidChatID_NoAncestors",
			chatID:          validID.String(),
			setChatID:       true,
			setAncestors:    false,
			wantChatID:      validID,
			wantAncestorIDs: nil,
			wantOK:          true,
		},
		{
			name:      "ValidChatID_ValidAncestors",
			chatID:    validID.String(),
			setChatID: true,
			ancestors: mustMarshalJSON(t, []string{
				ancestor1.String(),
				ancestor2.String(),
			}),
			setAncestors:    true,
			wantChatID:      validID,
			wantAncestorIDs: []uuid.UUID{ancestor1, ancestor2},
			wantOK:          true,
		},
		{
			name:            "MalformedChatID",
			chatID:          "not-a-uuid",
			setChatID:       true,
			setAncestors:    false,
			wantChatID:      uuid.Nil,
			wantAncestorIDs: nil,
			wantOK:          false,
		},
		{
			name:            "ValidChatID_MalformedAncestorJSON",
			chatID:          validID.String(),
			setChatID:       true,
			ancestors:       `{this is not json}`,
			setAncestors:    true,
			wantChatID:      validID,
			wantAncestorIDs: nil,
			wantOK:          true,
		},
		{
			// Only valid UUIDs in the array are returned; invalid
			// entries are silently skipped.
			name:      "ValidChatID_PartialValidAncestorUUIDs",
			chatID:    validID.String(),
			setChatID: true,
			ancestors: mustMarshalJSON(t, []string{
				ancestor1.String(),
				"bad-uuid",
				ancestor2.String(),
			}),
			setAncestors:    true,
			wantChatID:      validID,
			wantAncestorIDs: []uuid.UUID{ancestor1, ancestor2},
			wantOK:          true,
		},
		{
			// Header is explicitly set to an empty string, which
			// Header.Get returns as "".
			name:            "EmptyChatIDHeader",
			chatID:          "",
			setChatID:       true,
			setAncestors:    false,
			wantChatID:      uuid.Nil,
			wantAncestorIDs: nil,
			wantOK:          false,
		},
		{
			name:            "ValidChatID_EmptyAncestorHeader",
			chatID:          validID.String(),
			setChatID:       true,
			ancestors:       "",
			setAncestors:    true,
			wantChatID:      validID,
			wantAncestorIDs: nil,
			wantOK:          true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest("GET", "/", nil)
			if tt.setChatID {
				r.Header.Set(workspacesdk.CoderChatIDHeader, tt.chatID)
			}
			if tt.setAncestors {
				r.Header.Set(workspacesdk.CoderAncestorChatIDsHeader, tt.ancestors)
			}

			chatID, ancestorIDs, ok := agentgit.ExtractChatContext(r)

			require.Equal(t, tt.wantOK, ok, "ok mismatch")
			require.Equal(t, tt.wantChatID, chatID, "chatID mismatch")
			require.Equal(t, tt.wantAncestorIDs, ancestorIDs, "ancestorIDs mismatch")
		})
	}
}

// mustMarshalJSON marshals v to a JSON string, failing the test on error.
func mustMarshalJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
