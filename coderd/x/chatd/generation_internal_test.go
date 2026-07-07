package chatd //nolint:testpackage // Exercises unexported generation helpers.

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

func TestRecordGenerationFinishFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		wantRecorded bool
	}{
		{
			name:         "TerminalFailureRecordsError",
			err:          normalizeTaskTransitionError(chatstate.ErrTransitionNotAllowed, "finish generation error"),
			wantRecorded: true,
		},
		{
			name:         "ExpectedExitSkips",
			err:          normalizeTaskTransitionError(errTaskExpectedExit, "finish generation error"),
			wantRecorded: false,
		},
		{
			name:         "RetryableSkips",
			err:          normalizeTaskTransitionError(xerrors.New("transient infrastructure failure"), "finish generation error"),
			wantRecorded: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			turn := newRunnerDebugTurn(testutil.Context(t, testutil.WaitShort), testutil.Logger(t))
			recordGenerationFinishFailure(turn, tt.err)
			require.Equal(t, tt.wantRecorded, turn.statusSet)
			if tt.wantRecorded {
				require.Equal(t, chatdebug.StatusError, turn.status)
			}
		})
	}
}
