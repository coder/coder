package chatdebug //nolint:testpackage // Uses unexported recorder helpers.

import (
	"context"
	"sort"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

type stubModel struct {
	provider string
	model    string
}

func (*stubModel) Generate(
	ctx context.Context,
	call fantasy.Call,
) (*fantasy.Response, error) {
	return &fantasy.Response{}, nil
}

func (*stubModel) Stream(
	ctx context.Context,
	call fantasy.Call,
) (fantasy.StreamResponse, error) {
	return fantasy.StreamResponse(func(func(fantasy.StreamPart) bool) {}), nil
}

func (*stubModel) GenerateObject(
	ctx context.Context,
	call fantasy.ObjectCall,
) (*fantasy.ObjectResponse, error) {
	return &fantasy.ObjectResponse{}, nil
}

func (*stubModel) StreamObject(
	ctx context.Context,
	call fantasy.ObjectCall,
) (fantasy.ObjectStreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (s *stubModel) Provider() string {
	return s.provider
}

func (s *stubModel) Model() string {
	return s.model
}

func TestAttemptSink_ThreadSafe(t *testing.T) {
	t.Parallel()

	const n = 256

	sink := &attemptSink{}
	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		i := i
		go func() {
			defer wg.Done()
			sink.record(Attempt{Number: i + 1, ResponseStatus: 200 + i})
		}()
	}

	wg.Wait()

	attempts := sink.snapshot()
	require.Len(t, attempts, n)

	numbers := make([]int, 0, n)
	statuses := make([]int, 0, n)
	for _, attempt := range attempts {
		numbers = append(numbers, attempt.Number)
		statuses = append(statuses, attempt.ResponseStatus)
	}
	sort.Ints(numbers)
	sort.Ints(statuses)

	for i := range n {
		require.Equal(t, i+1, numbers[i])
		require.Equal(t, 200+i, statuses[i])
	}
}

func TestAttemptSinkContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	require.Nil(t, attemptSinkFromContext(ctx))

	sink := &attemptSink{}
	ctx = withAttemptSink(ctx, sink)
	require.Same(t, sink, attemptSinkFromContext(ctx))
}

func TestWrapModel_NilModel(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		WrapModel(nil, &Service{}, RecorderOptions{})
	})
}

func TestWrapModel_NilService(t *testing.T) {
	t.Parallel()

	model := &stubModel{provider: "provider", model: "model"}
	wrapped := WrapModel(model, nil, RecorderOptions{})
	require.Same(t, model, wrapped)
}

func TestNextStepNumber_Concurrent(t *testing.T) {
	t.Parallel()

	const n = 256

	runID := uuid.New()
	results := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		i := i
		go func() {
			defer wg.Done()
			results[i] = int(nextStepNumber(runID))
		}()
	}

	wg.Wait()

	sort.Ints(results)
	for i := range n {
		require.Equal(t, i+1, results[i])
	}
}

func TestStepHandleFinish_NilHandle(t *testing.T) {
	t.Parallel()

	var handle *stepHandle
	handle.finish(context.Background(), StatusCompleted, nil, nil, nil, nil)
}

func TestBeginStep_NilService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handle, enriched := beginStep(ctx, nil, RecorderOptions{}, OperationGenerate, nil)
	require.Nil(t, handle)
	require.Nil(t, attemptSinkFromContext(enriched))
	_, ok := StepFromContext(enriched)
	require.False(t, ok)
}

func TestWrapModel_ReturnsDebugModel(t *testing.T) {
	t.Parallel()

	model := &stubModel{provider: "provider", model: "model"}
	wrapped := WrapModel(model, &Service{}, RecorderOptions{})

	require.NotSame(t, model, wrapped)
	require.IsType(t, &debugModel{}, wrapped)
	require.Implements(t, (*fantasy.LanguageModel)(nil), wrapped)
	require.Equal(t, model.Provider(), wrapped.Provider())
	require.Equal(t, model.Model(), wrapped.Model())
}
