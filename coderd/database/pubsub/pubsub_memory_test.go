package pubsub_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database/pubsub"
)

func TestPubsubMemory(t *testing.T) {
	t.Parallel()

	t.Run("Legacy", func(t *testing.T) {
		t.Parallel()

		pubsub := pubsub.NewInMemory()
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

	t.Run("WithErr", func(t *testing.T) {
		t.Parallel()

		pubsub := pubsub.NewInMemory()
		event := "test"
		data := "testing"
		messageChannel := make(chan []byte)
		cancelFunc, err := pubsub.SubscribeWithErr(event, func(ctx context.Context, message []byte, err error) {
			assert.NoError(t, err) // memory pubsub never sends errors.
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
