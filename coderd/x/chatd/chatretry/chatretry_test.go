package chatretry_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
)

func TestIsRetryableDelegatesToClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{name: "Nil", err: nil, retryable: false},
		{name: "RetryableExplicitStatus429", err: xerrors.New("received status 429 from upstream"), retryable: true},
		{name: "RetryableTimeout", err: xerrors.New("service unavailable"), retryable: true},
		{name: "NonRetryableAuth", err: xerrors.New("invalid api key"), retryable: false},
		{name: "NonRetryableGeneric", err: xerrors.New("boom"), retryable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.retryable, chatretry.IsRetryable(tt.err))
			require.Equal(t, chaterror.Classify(tt.err).Retryable, chatretry.IsRetryable(tt.err))
		})
	}
}

func TestRetryabilityFromClassifyStatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code      int
		retryable bool
	}{
		{408, true},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{529, true},
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Status%d", tt.code), func(t *testing.T) {
			t.Parallel()

			err := xerrors.Errorf("status %d from upstream", tt.code)
			classified := chaterror.Classify(err)
			require.Equal(t, tt.retryable, classified.Retryable)
			require.Equal(t, classified.Retryable, chatretry.IsRetryable(err))
		})
	}
}

func TestDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 60 * time.Second},
		{10, 60 * time.Second},
		{100, 60 * time.Second},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Attempt%d", tt.attempt), func(t *testing.T) {
			t.Parallel()
			got := chatretry.Delay(tt.attempt)
			if got != tt.want {
				t.Errorf("Delay(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestRetry_SuccessOnFirstTry(t *testing.T) {
	t.Parallel()

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		return nil
	}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestRetry_TransientThenSuccess(t *testing.T) {
	t.Parallel()

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		if calls == 1 {
			return xerrors.New("service unavailable")
		}
		return nil
	}, nil)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestRetry_MultipleTransientThenSuccess(t *testing.T) {
	t.Parallel()

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		if calls <= 3 {
			return xerrors.New("overloaded")
		}
		return nil
	}, nil)
	require.NoError(t, err)
	require.Equal(t, 4, calls)
}

func TestRetry_NonRetryableError(t *testing.T) {
	t.Parallel()

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		return xerrors.New("invalid api key")
	}, nil)

	require.Error(t, err)
	require.EqualError(t, err, "invalid api key")
	require.Equal(t, 1, calls)
	require.Equal(
		t,
		chaterror.Classify(xerrors.New("invalid api key")),
		chaterror.Classify(err),
	)
}

func TestRetry_ContextCanceledDuringWait(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := chatretry.Retry(ctx, func(_ context.Context) error {
		calls++
		if calls == 1 {
			cancel()
		}
		return xerrors.New("overloaded")
	}, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRetry_ContextCanceledDuringFn(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	err := chatretry.Retry(ctx, func(_ context.Context) error {
		cancel()
		return xerrors.New("overloaded")
	}, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRetry_OnRetryCalledWithCorrectArgs(t *testing.T) {
	t.Parallel()

	type retryRecord struct {
		attempt    int
		errMsg     string
		classified chatretry.ClassifiedError
		delay      time.Duration
	}
	var records []retryRecord

	calls := 0
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		calls++
		if calls <= 2 {
			return xerrors.New("received status 429 from upstream")
		}
		return nil
	}, func(
		attempt int,
		err error,
		classified chatretry.ClassifiedError,
		delay time.Duration,
	) {
		records = append(records, retryRecord{
			attempt:    attempt,
			errMsg:     err.Error(),
			classified: classified,
			delay:      delay,
		})
	})
	require.NoError(t, err)
	require.Len(t, records, 2)

	expected := chaterror.Classify(xerrors.New("received status 429 from upstream"))
	require.Equal(t, 1, records[0].attempt)
	require.Equal(t, 2, records[1].attempt)
	require.Equal(t, "received status 429 from upstream", records[0].errMsg)
	require.Equal(t, expected, records[0].classified)
	require.Equal(t, expected, records[1].classified)
	require.Equal(t, chatretry.Delay(0), records[0].delay)
	require.Equal(t, chatretry.Delay(1), records[1].delay)
}

func TestRetry_OnRetryNilDoesNotPanic(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	err := chatretry.Retry(context.Background(), func(_ context.Context) error {
		if calls.Add(1) == 1 {
			return xerrors.New("overloaded")
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
