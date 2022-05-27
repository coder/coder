package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
)

func TestPubsubMemory(t *testing.T) {
	t.Parallel()

	t.Run("Memory", func(t *testing.T) {
		t.Parallel()

		pubsub := database.NewPubsubInMemory()
		event := "test"
		data := "testing"
		messageChannel := make(chan []byte)
		cancelFunc, err := pubsub.Subscribe(event, func(ctx context.Context, message []byte) {
			messageChannel <- message
		})
		require.NoError(t, err)
		defer cancelFunc()
		go func() {
			err = pubsub.Publish(event, []byte(data))
			assert.NoError(t, err)
		}()
		message := <-messageChannel
		assert.Equal(t, string(message), data)
	})
}
