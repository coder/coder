package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/database"
)

func TestPubsubMemory(t *testing.T) {
	t.Parallel()

	t.Run("Memory", func(t *testing.T) {
		pubsub := database.NewPubsubInMemory()
		event := "test"
		data := "testing"
		ch := make(chan []byte)
		cancelFunc, err := pubsub.Subscribe(event, func(ctx context.Context, message []byte) {
			ch <- message
		})
		require.NoError(t, err)
		defer cancelFunc()
		go func() {
			err = pubsub.Publish(event, []byte(data))
			require.NoError(t, err)
		}()
		message := <-ch
		assert.Equal(t, string(message), data)
	})
}
