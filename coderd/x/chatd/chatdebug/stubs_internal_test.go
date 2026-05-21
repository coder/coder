package chatdebug

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBeginStep_SkipsNilRunID(t *testing.T) {
	t.Parallel()

	ctx := ContextWithRun(context.Background(), &RunContext{ChatID: uuid.New()})
	handle, enriched := beginStep(ctx, &Service{}, RecorderOptions{ChatID: uuid.New()}, OperationGenerate, nil)
	require.Nil(t, handle)
	require.Equal(t, ctx, enriched)
}
