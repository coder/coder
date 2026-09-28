package chattool_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestIsCapableAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		apiVersion string
		want       bool
	}{
		{apiVersion: "", want: false},
		{apiVersion: "garbage", want: false},
		{apiVersion: "2.12", want: false},
		{apiVersion: "1.99", want: false},
		{apiVersion: "2.13", want: true},
		{apiVersion: "2.14", want: true},
		{apiVersion: "3.0", want: true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, chattool.IsCapableAgent(database.WorkspaceAgent{APIVersion: tt.apiVersion}), "api_version %q", tt.apiVersion)
	}
}

func TestSendsToolCallIdentity(t *testing.T) {
	t.Parallel()

	assert.True(t, chattool.SendsToolCallIdentity(chattool.ExecuteToolName))
	// File tools send no identity until the agent records them.
	for _, name := range []string{"edit_files", "write_file", "read_file", "process_output"} {
		assert.False(t, chattool.SendsToolCallIdentity(name), name)
	}
}

func TestRequestUntilAnswered(t *testing.T) {
	t.Parallel()

	transportErr := &url.Error{Op: "Post", URL: "http://agent", Err: xerrors.New("connection reset by peer")}

	t.Run("AnswersAreReturned", func(t *testing.T) {
		t.Parallel()

		for _, answer := range []error{
			nil,
			&workspacesdk.ToolCallError{Code: workspacesdk.ToolCallErrorCanceled},
			codersdk.NewError(http.StatusInternalServerError, codersdk.Response{Message: "boom"}),
		} {
			sends := 0
			err := chattool.RequestUntilAnswered(testutil.Context(t, testutil.WaitShort), quartz.NewMock(t), func(context.Context) error {
				sends++
				return answer
			})
			assert.Equal(t, answer, err)
			assert.Equal(t, 1, sends)
		}
	})

	t.Run("ContextEndStopsRetries", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		err := chattool.RequestUntilAnswered(ctx, quartz.NewMock(t), func(context.Context) error {
			cancel()
			return transportErr
		})
		require.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, chattool.ErrAgentNoAnswer)
	})

	t.Run("NoAnswerWrapsLastError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)

		sent := make(chan struct{}, 1)
		done := make(chan error, 1)
		go func() {
			done <- chattool.RequestUntilAnswered(ctx, clock, func(ctx context.Context) error {
				sent <- struct{}{}
				<-ctx.Done()
				return transportErr
			})
		}()
		testutil.RequireReceive(ctx, t, sent)
		clock.Advance(chattool.AgentAnswerTimeout).MustWait(ctx)

		err := testutil.RequireReceive(ctx, t, done)
		require.ErrorIs(t, err, chattool.ErrAgentNoAnswer)
		require.ErrorIs(t, err, transportErr)
		assert.Contains(t, err.Error(), "the workspace agent did not answer within 1m0s")
	})
}
