//go:build linux

package database_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/coder/coder/database"
	"github.com/coder/coder/database/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPubsub(t *testing.T) {
	t.Parallel()

	t.Run("Postgres", func(t *testing.T) {
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		connectionURL, close, err := postgres.Open()
		require.NoError(t, err)
		defer close()
		db, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer db.Close()
		pubsub, err := database.NewPubsub(ctx, db, connectionURL)
		require.NoError(t, err)
		defer pubsub.Close()
		event := "test"
		data := "testing"
		ch := make(chan []byte)
		cancelFunc, err = pubsub.Subscribe(event, func(ctx context.Context, message []byte) {
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
