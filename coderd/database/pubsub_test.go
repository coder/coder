//go:build linux

package database_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/postgres"
)

func TestPubsub(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip()
		return
	}

	t.Run("Postgres", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		connectionURL, close, err := postgres.Open()
		require.NoError(t, err)
		t.Cleanup(close)

		db, err := pgxpool.Connect(ctx, connectionURL)
		require.NoError(t, err)
		t.Cleanup(db.Close)

		pubsub, err := database.NewPubsub(ctx, db, connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() { _ = pubsub.Close() })

		var (
			event   = "test"
			data    = "testing"
			msgChan = make(chan []byte)
		)

		cancelFunc, err = pubsub.Subscribe(event, func(ctx context.Context, message []byte) {
			msgChan <- message
		})
		require.NoError(t, err)
		defer cancelFunc()

		go func() {
			err = pubsub.Publish(event, []byte(data))
			require.NoError(t, err)
		}()

		message := <-msgChan
		assert.Equal(t, string(message), data)
	})

	t.Run("PostgresCloseCancel", func(t *testing.T) {
		t.Parallel()
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		connectionURL, close, err := postgres.Open()
		require.NoError(t, err)
		t.Cleanup(close)

		db, err := pgxpool.Connect(ctx, connectionURL)
		require.NoError(t, err)
		t.Cleanup(db.Close)

		pubsub, err := database.NewPubsub(ctx, db, connectionURL)
		require.NoError(t, err)
		t.Cleanup(func() { _ = pubsub.Close() })

		cancelFunc()
	})
}
