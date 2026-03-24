package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"

	agplcoderd "github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

func TestPGCoordPubsubPrefersPostgresPubsub(t *testing.T) {
	t.Parallel()

	featurePubsub := pubsub.NewInMemory()
	pgPubsub := pubsub.NewInMemory()
	api := &API{
		Options: &Options{
			Options: &agplcoderd.Options{
				Pubsub:   featurePubsub,
				PGPubsub: pgPubsub,
			},
		},
	}

	require.Same(t, pgPubsub, api.pgCoordPubsub())
}

func TestPGCoordPubsubFallsBackToPrimaryPubsub(t *testing.T) {
	t.Parallel()

	featurePubsub := pubsub.NewInMemory()
	api := &API{
		Options: &Options{
			Options: &agplcoderd.Options{
				Pubsub: featurePubsub,
			},
		},
	}

	require.Same(t, featurePubsub, api.pgCoordPubsub())
}
