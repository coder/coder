package intercept_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/recorder"
)

func TestNilActor(t *testing.T) {
	t.Parallel()

	require.Nil(t, intercept.ActorHeadersAsOpenAIOpts(nil))
	require.Nil(t, intercept.ActorHeadersAsAnthropicOpts(nil))
}

func TestBasic(t *testing.T) {
	t.Parallel()

	actorID := uuid.NewString()
	actor := &context.Actor{
		ID: actorID,
	}

	// We can't peek inside since these opts require an internal type to apply onto.
	// All we can do is check the length.
	// See TestActorHeaders for an integration test.
	oaiOpts := intercept.ActorHeadersAsOpenAIOpts(actor)
	require.Len(t, oaiOpts, 1)
	antOpts := intercept.ActorHeadersAsAnthropicOpts(actor)
	require.Len(t, antOpts, 1)
}

func TestBasicAndMetadata(t *testing.T) {
	t.Parallel()

	actorID := uuid.NewString()
	actor := &context.Actor{
		ID: actorID,
		Metadata: recorder.Metadata{
			"This": "That",
			"And":  "The other",
		},
	}

	// We can't peek inside since these opts require an internal type to apply onto.
	// All we can do is check the length.
	// See TestActorHeaders for an integration test.
	oaiOpts := intercept.ActorHeadersAsOpenAIOpts(actor)
	require.Len(t, oaiOpts, 1+len(actor.Metadata))
	antOpts := intercept.ActorHeadersAsAnthropicOpts(actor)
	require.Len(t, antOpts, 1+len(actor.Metadata))
}

func TestSetActorHeaders(t *testing.T) {
	t.Parallel()

	t.Run("nil actor is a no-op", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{"User-Agent": {"test"}}
		intercept.SetActorHeaders(headers, nil)
		require.Equal(t, http.Header{"User-Agent": {"test"}}, headers)
	})

	t.Run("sets actor ID header", func(t *testing.T) {
		t.Parallel()

		actorID := uuid.NewString()
		actor := &context.Actor{ID: actorID}

		headers := http.Header{}
		intercept.SetActorHeaders(headers, actor)

		require.Equal(t, actorID, headers.Get(intercept.ActorIDHeader()))
	})

	t.Run("sets actor ID and metadata headers", func(t *testing.T) {
		t.Parallel()

		actorID := uuid.NewString()
		actor := &context.Actor{
			ID: actorID,
			Metadata: recorder.Metadata{
				"Name":  "alice",
				"Email": "alice@example.com",
			},
		}

		headers := http.Header{}
		intercept.SetActorHeaders(headers, actor)

		require.Equal(t, actorID, headers.Get(intercept.ActorIDHeader()))
		require.Equal(t, "alice", headers.Get(intercept.ActorMetadataHeader("Name")))
		require.Equal(t, "alice@example.com", headers.Get(intercept.ActorMetadataHeader("Email")))
	})
}
