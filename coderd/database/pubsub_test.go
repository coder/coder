//go:build linux

package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/coder/coder/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/postgres"
)

// nolint:tparallel,paralleltest
func TestPubsub(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.SkipNow()
		return
	}

	t.Run("Postgres", func(t *testing.T) {
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()

		connectionURL, closePg, err := postgres.Open()
		require.NoError(t, err)
		defer closePg()
		db, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer db.Close()
		pubsub, err := database.NewPubsub(ctx, db, connectionURL)
		require.NoError(t, err)
		defer pubsub.Close()
		event := "test"
		data := "testing"
		messageChannel := make(chan []byte)
		cancelFunc, err = pubsub.Subscribe(event, func(ctx context.Context, message []byte) {
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

	t.Run("PostgresCloseCancel", func(t *testing.T) {
		ctx, cancelFunc := context.WithCancel(context.Background())
		defer cancelFunc()
		connectionURL, closePg, err := postgres.Open()
		require.NoError(t, err)
		defer closePg()
		db, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer db.Close()
		pubsub, err := database.NewPubsub(ctx, db, connectionURL)
		require.NoError(t, err)
		defer pubsub.Close()
		cancelFunc()
	})
}

func TestPubsub_ordering(t *testing.T) {
	t.Parallel()

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	connectionURL, closePg, err := postgres.Open()
	require.NoError(t, err)
	defer closePg()
	db, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	defer db.Close()
	pubsub, err := database.NewPubsub(ctx, db, connectionURL)
	require.NoError(t, err)
	defer pubsub.Close()
	event := "test"
	messageChannel := make(chan []byte, 100)
	cancelFunc, err = pubsub.Subscribe(event, func(ctx context.Context, message []byte) {
		// sleep a random amount of time to simulate handlers taking different amount of time
		// to process, depending on the message
		// nolint: gosec
		n := rand.Intn(100)
		time.Sleep(time.Duration(n) * time.Millisecond)
		messageChannel <- message
	})
	require.NoError(t, err)
	defer cancelFunc()
	for i := 0; i < 100; i++ {
		err = pubsub.Publish(event, []byte(fmt.Sprintf("%d", i)))
		assert.NoError(t, err)
	}
	for i := 0; i < 100; i++ {
		select {
		case <-time.After(testutil.WaitShort):
			t.Fatalf("timed out waiting for message %d", i)
		case message := <-messageChannel:
			assert.Equal(t, fmt.Sprintf("%d", i), string(message))
		}
	}
}
