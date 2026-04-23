package context_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/recorder"
)

func TestAsActor(t *testing.T) {
	t.Parallel()

	// Given: a metadata map
	metadata := recorder.Metadata{"key": "value"}

	// When: storing an actor in the context
	ctx := aibcontext.AsActor(context.Background(), "actor-123", metadata)

	// Then: the actor should be retrievable with correct ID and metadata
	actor := aibcontext.ActorFromContext(ctx)
	require.NotNil(t, actor)
	assert.Equal(t, "actor-123", actor.ID)
	assert.Equal(t, "value", actor.Metadata["key"])
}

func TestActorFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns actor when present", func(t *testing.T) {
		t.Parallel()

		// Given: a context with an actor
		ctx := aibcontext.AsActor(context.Background(), "test-id", recorder.Metadata{})

		// When: extracting the actor from context
		actor := aibcontext.ActorFromContext(ctx)

		// Then: the actor should be returned with correct ID
		require.NotNil(t, actor)
		assert.Equal(t, "test-id", actor.ID)
	})

	t.Run("returns nil when no actor", func(t *testing.T) {
		t.Parallel()

		// Given: a context without an actor
		ctx := context.Background()

		// When: extracting the actor from context
		actor := aibcontext.ActorFromContext(ctx)

		// Then: nil should be returned
		assert.Nil(t, actor)
	})
}

func TestActorIDFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns actor ID when present", func(t *testing.T) {
		t.Parallel()

		// Given: a context with an actor
		ctx := aibcontext.AsActor(context.Background(), "test-actor-id", recorder.Metadata{})

		// When: extracting the actor ID from context
		got := aibcontext.ActorIDFromContext(ctx)

		// Then: the actor ID should be returned
		assert.Equal(t, "test-actor-id", got)
	})

	t.Run("returns empty string when no actor", func(t *testing.T) {
		t.Parallel()

		// Given: a context without an actor
		ctx := context.Background()

		// When: extracting the actor ID from context
		got := aibcontext.ActorIDFromContext(ctx)

		// Then: an empty string should be returned
		assert.Empty(t, got)
	})
}
