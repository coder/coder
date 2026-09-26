package agenttoolcall

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// The HTTP path rejects negative ages, so only this package can pass one.
func TestNegativeAgeCountsAsZero(t *testing.T) {
	t.Parallel()

	clock := quartz.NewMock(t)
	s := NewStore(clock)
	clock.Advance(time.Second).MustWait(testutil.Context(t, testutil.WaitShort))
	key := Key{ChatID: uuid.New(), MessageID: 1, ToolCallID: "call"}

	_, _, err := s.begin(key, -time.Hour, [sha256.Size]byte{})
	require.ErrorIs(t, err, errAgentStartedAfterToolCall)
	_, _, err = s.cancel(key, -time.Hour)
	require.ErrorIs(t, err, errAgentStartedAfterToolCall)
}
