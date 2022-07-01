package database

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

// Listener represents a pubsub handler.
type Listener func(ctx context.Context, message []byte)

// Pubsub is a generic interface for broadcasting and receiving messages.
// Implementors should assume high-availability with the backing implementation.
type Pubsub interface {
	Subscribe(event string, listener Listener) (cancel func(), err error)
	Publish(event string, message []byte) error
	Close() error
}

// Pubsub implementation using PostgreSQL.
type pgPubsub struct {
	pgListener *pq.Listener
	db         *sql.DB
	mut        sync.Mutex
	listeners  map[string]map[uuid.UUID]Listener
}

// Subscribe calls the listener when an event matching the name is received.
func (p *pgPubsub) Subscribe(event string, listener Listener) (cancel func(), err error) {
	p.mut.Lock()
	defer p.mut.Unlock()

	err = p.pgListener.Listen(event)
	if errors.Is(err, pq.ErrChannelAlreadyOpen) {
		// It's ok if it's already open!
		err = nil
	}
	if err != nil {
		return nil, xerrors.Errorf("listen: %w", err)
	}

	var eventListeners map[uuid.UUID]Listener
	var ok bool
	if eventListeners, ok = p.listeners[event]; !ok {
		eventListeners = map[uuid.UUID]Listener{}
		p.listeners[event] = eventListeners
	}

	var id uuid.UUID
	for {
		id = uuid.New()
		if _, ok = eventListeners[id]; !ok {
			break
		}
	}

	eventListeners[id] = listener
	return func() {
		p.mut.Lock()
		defer p.mut.Unlock()
		listeners := p.listeners[event]
		delete(listeners, id)

		if len(listeners) == 0 {
			_ = p.pgListener.Unlisten(event)
		}
	}, nil
}

func (p *pgPubsub) Publish(event string, message []byte) error {
	// This is safe because we are calling pq.QuoteLiteral. pg_notify doesn't
	// support the first parameter being a prepared statement.
	//nolint:gosec
	_, err := p.db.ExecContext(context.Background(), `select pg_notify(`+pq.QuoteLiteral(event)+`, $1)`, message)
	if err != nil {
		return xerrors.Errorf("exec pg_notify: %w", err)
	}
	return nil
}

// Close closes the pubsub instance.
func (p *pgPubsub) Close() error {
	return p.pgListener.Close()
}

// listen begins receiving messages on the pq listener.
func (p *pgPubsub) listen(ctx context.Context) {
	var (
		notif *pq.Notification
		ok    bool
	)
	defer p.pgListener.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case notif, ok = <-p.pgListener.Notify:
			if !ok {
				return
			}
		}
		// A nil notification can be dispatched on reconnect.
		if notif == nil {
			continue
		}
		p.listenReceive(ctx, notif)
	}
}

func (p *pgPubsub) listenReceive(ctx context.Context, notif *pq.Notification) {
	p.mut.Lock()
	defer p.mut.Unlock()
	listeners, ok := p.listeners[notif.Channel]
	if !ok {
		return
	}
	extra := []byte(notif.Extra)
	for _, listener := range listeners {
		go listener(ctx, extra)
	}
}

// NewPubsub creates a new Pubsub implementation using a PostgreSQL connection.
func NewPubsub(ctx context.Context, database *sql.DB, connectURL string) (Pubsub, error) {
	// Creates a new listener using pq.
	errCh := make(chan error)
	listener := pq.NewListener(connectURL, time.Second, time.Minute, func(event pq.ListenerEventType, err error) {
		// This callback gets events whenever the connection state changes.
		// Don't send if the errChannel has already been closed.
		select {
		case <-errCh:
			return
		default:
			errCh <- err
			close(errCh)
		}
	})
	select {
	case err := <-errCh:
		if err != nil {
			return nil, xerrors.Errorf("create pq listener: %w", err)
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	pgPubsub := &pgPubsub{
		db:         database,
		pgListener: listener,
		listeners:  make(map[string]map[uuid.UUID]Listener),
	}
	go pgPubsub.listen(ctx)

	return pgPubsub, nil
}
