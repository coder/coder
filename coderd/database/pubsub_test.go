//go:build linux

package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/postgres"
	"github.com/coder/coder/testutil"
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
	cancelSub, err := pubsub.Subscribe(event, func(ctx context.Context, message []byte) {
		// sleep a random amount of time to simulate handlers taking different amount of time
		// to process, depending on the message
		// nolint: gosec
		n := rand.Intn(100)
		time.Sleep(time.Duration(n) * time.Millisecond)
		messageChannel <- message
	})
	require.NoError(t, err)
	defer cancelSub()
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

func TestPubsub_Disconnect(t *testing.T) {
	t.Parallel()
	// we always use a Docker container for this test, even in CI, since we need to be able to kill
	// postgres and bring it back on the same port.
	connectionURL, closePg, err := postgres.OpenContainerized(0)
	require.NoError(t, err)
	defer closePg()
	db, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	defer db.Close()

	ctx, cancelFunc := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	defer cancelFunc()
	pubsub, err := database.NewPubsub(ctx, db, connectionURL)
	require.NoError(t, err)
	defer pubsub.Close()
	event := "test"

	// buffer responses so that when the test completes, goroutines don't get blocked & leak
	errors := make(chan error, database.PubsubBufferSize)
	messages := make(chan string, database.PubsubBufferSize)
	readOne := func() (m string, e error) {
		t.Helper()
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		case m = <-messages:
			// OK
		}
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		case e = <-errors:
			// OK
		}
		return m, e
	}

	cancelSub, err := pubsub.SubscribeWithErr(event, func(ctx context.Context, msg []byte, err error) {
		messages <- string(msg)
		errors <- err
	})
	require.NoError(t, err)
	defer cancelSub()

	for i := 0; i < 100; i++ {
		err = pubsub.Publish(event, []byte(fmt.Sprintf("%d", i)))
		require.NoError(t, err)
	}
	// make sure we're getting at least one message.
	m, err := readOne()
	require.NoError(t, err)
	require.Equal(t, "0", m)

	closePg()
	// write some more messages until we hit an error
	j := 100
	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		default:
			// ok
		}
		err = pubsub.Publish(event, []byte(fmt.Sprintf("%d", j)))
		j++
		if err != nil {
			break
		}
		time.Sleep(testutil.IntervalFast)
	}

	// restart postgres on the same port --- since we only use LISTEN/NOTIFY it doesn't
	// matter that the new postgres doesn't have any persisted state from before.
	u, err := url.Parse(connectionURL)
	require.NoError(t, err)
	addr, err := net.ResolveTCPAddr("tcp", u.Host)
	require.NoError(t, err)
	newURL, closeNewPg, err := postgres.OpenContainerized(addr.Port)
	require.NoError(t, err)
	require.Equal(t, connectionURL, newURL)
	defer closeNewPg()

	// now write messages until we DON'T hit an error -- pubsub is back up.
	for {
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		default:
			// ok
		}
		err = pubsub.Publish(event, []byte(fmt.Sprintf("%d", j)))
		if err == nil {
			break
		}
		j++
		time.Sleep(testutil.IntervalFast)
	}
	// any message k or higher comes from after the restart.
	k := j
	// exceeding the buffer invalidates the test because this causes us to drop messages for reasons other than DB
	// reconnect
	require.Less(t, k, database.PubsubBufferSize, "exceeded buffer")

	// We don't know how quickly the pubsub will reconnect, so continue to send messages with increasing numbers.  As
	// soon as we see k or higher we know we're getting messages after the restart.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// ok
			}
			_ = pubsub.Publish(event, []byte(fmt.Sprintf("%d", j)))
			j++
			time.Sleep(testutil.IntervalFast)
		}
	}()

	gotDroppedErr := false
	for {
		m, err := readOne()
		if xerrors.Is(err, database.ErrDroppedMessages) {
			gotDroppedErr = true
			continue
		}
		require.NoError(t, err, "should only get ErrDroppedMessages")
		l, err := strconv.Atoi(m)
		require.NoError(t, err)
		if l >= k {
			// exceeding the buffer invalidates the test because this causes us to drop messages for reasons other than
			// DB reconnect
			require.Less(t, l, database.PubsubBufferSize, "exceeded buffer")
			break
		}
	}
	require.True(t, gotDroppedErr)
}
