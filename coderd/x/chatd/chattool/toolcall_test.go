package chattool_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

func TestToolCallAge(t *testing.T) {
	t.Parallel()

	dbNow := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		committedAt time.Time
		elapsed     time.Duration
		want        time.Duration
	}{
		{name: "AtRead", committedAt: dbNow.Add(-3 * time.Second), want: 3 * time.Second},
		{name: "AddsElapsed", committedAt: dbNow.Add(-3 * time.Second), elapsed: 2 * time.Second, want: 5 * time.Second},
		{name: "CommittedAfterReadIsZero", committedAt: dbNow.Add(2 * time.Second), want: 0},
		{name: "CommittedAfterReadCatchesUp", committedAt: dbNow.Add(2 * time.Second), elapsed: 5 * time.Second, want: 3 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The mock clock is years away from dbNow, so a result that
			// mixed coderd's clock with the database's would be far off.
			clock := quartz.NewMock(t)
			age := chattool.NewToolCallAge(clock, dbNow, tt.committedAt)
			clock.Advance(tt.elapsed)
			assert.Equal(t, tt.want, age.Now())
		})
	}

	t.Run("ZeroValue", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, time.Duration(0), chattool.ToolCallAge{}.Now())
	})
}

func TestToolCallIdentityContext(t *testing.T) {
	t.Parallel()

	_, ok := chattool.ToolCallIdentityFromContext(context.Background())
	require.False(t, ok)

	dbNow := time.Now()
	want := chattool.ToolCallIdentity{
		ChatID:     uuid.New(),
		MessageID:  42,
		ToolCallID: "call_" + uuid.NewString(),
		Age:        chattool.NewToolCallAge(quartz.NewMock(t), dbNow, dbNow.Add(-time.Minute)),
	}
	got, ok := chattool.ToolCallIdentityFromContext(chattool.WithToolCallIdentity(context.Background(), want))
	require.True(t, ok)
	assert.Equal(t, want.ChatID, got.ChatID)
	assert.Equal(t, want.MessageID, got.MessageID)
	assert.Equal(t, want.ToolCallID, got.ToolCallID)
	assert.Equal(t, time.Minute, got.Age.Now())
}

func TestToolCallIdentityAgentRequests(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	dbNow := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	id := chattool.ToolCallIdentity{
		ChatID:     uuid.New(),
		MessageID:  42,
		ToolCallID: "call_" + uuid.NewString(),
		Age:        chattool.NewToolCallAge(clock, dbNow, dbNow.Add(-time.Minute)),
	}

	assert.Equal(t, workspacesdk.ToolCallUUID(id.ChatID, 42, id.ToolCallID).String(), id.UUID())
	assert.Equal(t, workspacesdk.ToolCall{MessageID: 42, ID: id.ToolCallID, Age: time.Minute}, id.AgentToolCall())
	// Each request measures the age when it is built.
	clock.Advance(3 * time.Second)
	assert.Equal(t, workspacesdk.ToolCall{MessageID: 42, ID: id.ToolCallID, Age: time.Minute + 3*time.Second}, id.AgentToolCall())
}

func TestClassifyAgentError(t *testing.T) {
	t.Parallel()

	refused := &workspacesdk.ToolCallError{Code: workspacesdk.ToolCallErrorInputMismatch}
	tests := []struct {
		name     string
		err      error
		wantKind chattool.AgentErrorKind
		wantCode workspacesdk.ToolCallErrorCode
	}{
		{name: "Refused", err: xerrors.Errorf("start: %w", refused), wantKind: chattool.AgentErrorRefused, wantCode: workspacesdk.ToolCallErrorInputMismatch},
		{name: "Response", err: xerrors.Errorf("start: %w", codersdk.NewError(http.StatusNotFound, codersdk.Response{Message: "not found"})), wantKind: chattool.AgentErrorResponse},
		{name: "Unreachable", err: xerrors.Errorf("do request: %w", &url.Error{Op: "Post", URL: "http://agent/api/v0/processes/start", Err: io.ErrUnexpectedEOF}), wantKind: chattool.AgentErrorUnreachable},
		{name: "Unreadable", err: xerrors.New("decode response: unexpected EOF"), wantKind: chattool.AgentErrorUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			kind, code := chattool.ClassifyAgentError(tt.err)
			assert.Equal(t, tt.wantKind, kind)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}
