package context_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aibcontext "github.com/coder/coder/v2/aibridge/context"
)

func TestAsActor(t *testing.T) {
	t.Parallel()

	want := aibcontext.Actor{
		ID:       uuid.New(),
		APIKeyID: "test-key-id",
		Username: "actor",
		Email:    "actor@example.com",
	}
	ctx := aibcontext.AsActor(t.Context(), want)
	actor := aibcontext.ActorFromContext(ctx)
	require.NotNil(t, actor)
	assert.Equal(t, want, *actor)

	want.Username = "changed"
	assert.Equal(t, "actor", actor.Username, "the context owns a copy of the actor")
	assert.Same(t, actor, aibcontext.ActorFromContext(context.WithoutCancel(ctx)))
}

func TestActorFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns actor when present", func(t *testing.T) {
		t.Parallel()

		// Given: a context with an actor
		id := uuid.New()
		ctx := aibcontext.AsActor(t.Context(), aibcontext.Actor{ID: id})

		// When: extracting the actor from context
		actor := aibcontext.ActorFromContext(ctx)

		// Then: the actor should be returned with correct ID
		require.NotNil(t, actor)
		assert.Equal(t, id, actor.ID)
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
		id := uuid.New()
		ctx := aibcontext.AsActor(t.Context(), aibcontext.Actor{ID: id})

		// When: extracting the actor ID from context
		got := aibcontext.ActorIDFromContext(ctx)

		// Then: the actor ID should be returned
		assert.Equal(t, id.String(), got)
	})

	t.Run("returns empty string for nil ID", func(t *testing.T) {
		t.Parallel()
		ctx := aibcontext.AsActor(t.Context(), aibcontext.Actor{})
		assert.Empty(t, aibcontext.ActorIDFromContext(ctx))
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
