package intercept_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/aibridge/context"
	"github.com/coder/aibridge/intercept"
	"github.com/coder/aibridge/recorder"
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
