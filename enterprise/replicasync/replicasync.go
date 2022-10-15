package replicasync

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/database"
)

var (
	PubsubEvent = "replica"
)

type Options struct {
	ID             uuid.UUID
	UpdateInterval time.Duration
	PeerTimeout    time.Duration
	RelayAddress   string
	RegionID       int32
	TLSConfig      *tls.Config
}

// New registers the replica with the database and periodically updates to ensure
// it's healthy. It contacts all other alive replicas to ensure they are reachable.
func New(ctx context.Context, logger slog.Logger, db database.Store, pubsub database.Pubsub, options Options) (*Manager, error) {
	if options.ID == uuid.Nil {
		panic("An ID must be provided!")
	}
	if options.PeerTimeout == 0 {
		options.PeerTimeout = 3 * time.Second
	}
	if options.UpdateInterval == 0 {
		options.UpdateInterval = 5 * time.Second
	}
	hostname, err := os.Hostname()
	if err != nil {
		return nil, xerrors.Errorf("get hostname: %w", err)
	}
	databaseLatency, err := db.Ping(ctx)
	if err != nil {
		return nil, xerrors.Errorf("ping database: %w", err)
	}
	var replica database.Replica
	_, err = db.GetReplicaByID(ctx, options.ID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get replica: %w", err)
		}
		replica, err = db.InsertReplica(ctx, database.InsertReplicaParams{
			ID:              options.ID,
			CreatedAt:       database.Now(),
			StartedAt:       database.Now(),
			UpdatedAt:       database.Now(),
			Hostname:        hostname,
			RegionID:        options.RegionID,
			RelayAddress:    options.RelayAddress,
			Version:         buildinfo.Version(),
			DatabaseLatency: int32(databaseLatency.Microseconds()),
		})
		if err != nil {
			return nil, xerrors.Errorf("insert replica: %w", err)
		}
	} else {
		replica, err = db.UpdateReplica(ctx, database.UpdateReplicaParams{
			ID:              options.ID,
			UpdatedAt:       database.Now(),
			StartedAt:       database.Now(),
			StoppedAt:       sql.NullTime{},
			RelayAddress:    options.RelayAddress,
			RegionID:        options.RegionID,
			Hostname:        hostname,
			Version:         buildinfo.Version(),
			Error:           sql.NullString{},
			DatabaseLatency: int32(databaseLatency.Microseconds()),
		})
		if err != nil {
			return nil, xerrors.Errorf("update replica: %w", err)
		}
	}
	err = pubsub.Publish(PubsubEvent, []byte(options.ID.String()))
	if err != nil {
		return nil, xerrors.Errorf("publish new replica: %w", err)
	}
	ctx, cancelFunc := context.WithCancel(ctx)
	server := &Manager{
		options:     &options,
		db:          db,
		pubsub:      pubsub,
		self:        replica,
		logger:      logger,
		closed:      make(chan struct{}),
		closeCancel: cancelFunc,
	}
	err = server.run(ctx)
	if err != nil {
		return nil, xerrors.Errorf("run replica: %w", err)
	}
	peers := server.Regional()
	if len(peers) > 0 {
		self := server.Self()
		if self.RelayAddress == "" {
			return nil, xerrors.Errorf("a relay address must be specified when running multiple replicas in the same region")
		}
	}

	err = server.subscribe(ctx)
	if err != nil {
		return nil, xerrors.Errorf("subscribe: %w", err)
	}
	server.closeWait.Add(1)
	go server.loop(ctx)
	return server, nil
}

// Manager keeps the replica up to date and in sync with other replicas.
type Manager struct {
	options *Options
	db      database.Store
	pubsub  database.Pubsub
	logger  slog.Logger

	closeWait   sync.WaitGroup
	closeMutex  sync.Mutex
	closed      chan (struct{})
	closeCancel context.CancelFunc

	self     database.Replica
	mutex    sync.Mutex
	peers    []database.Replica
	callback func()
}

// loop runs the replica update sequence on an update interval.
func (m *Manager) loop(ctx context.Context) {
	defer m.closeWait.Done()
	ticker := time.NewTicker(m.options.UpdateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		err := m.run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			m.logger.Warn(ctx, "run replica update loop", slog.Error(err))
		}
	}
}

// subscribe listens for new replica information!
func (m *Manager) subscribe(ctx context.Context) error {
	needsUpdate := false
	updating := false
	updateMutex := sync.Mutex{}

	// This loop will continually update nodes as updates are processed.
	// The intent is to always be up to date without spamming the run
	// function, so if a new update comes in while one is being processed,
	// it will reprocess afterwards.
	var update func()
	update = func() {
		err := m.run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			m.logger.Error(ctx, "run replica from subscribe", slog.Error(err))
		}
		updateMutex.Lock()
		if needsUpdate {
			needsUpdate = false
			updateMutex.Unlock()
			update()
			return
		}
		updating = false
		updateMutex.Unlock()
	}
	cancelFunc, err := m.pubsub.Subscribe(PubsubEvent, func(ctx context.Context, message []byte) {
		updateMutex.Lock()
		defer updateMutex.Unlock()
		id, err := uuid.Parse(string(message))
		if err != nil {
			return
		}
		// Don't process updates for ourself!
		if id == m.options.ID {
			return
		}
		if updating {
			needsUpdate = true
			return
		}
		updating = true
		go update()
	})
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		cancelFunc()
	}()
	return nil
}

func (m *Manager) run(ctx context.Context) error {
	m.closeMutex.Lock()
	m.closeWait.Add(1)
	m.closeMutex.Unlock()
	go func() {
		m.closeWait.Done()
	}()
	// Expect replicas to update once every three times the interval...
	// If they don't, assume death!
	replicas, err := m.db.GetReplicasUpdatedAfter(ctx, database.Now().Add(-3*m.options.UpdateInterval))
	if err != nil {
		return xerrors.Errorf("get replicas: %w", err)
	}

	m.mutex.Lock()
	m.peers = make([]database.Replica, 0, len(replicas))
	for _, replica := range replicas {
		if replica.ID == m.options.ID {
			continue
		}
		m.peers = append(m.peers, replica)
	}
	m.mutex.Unlock()

	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := make([]string, 0)
	for _, peer := range m.Regional() {
		wg.Add(1)
		peer := peer
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, peer.RelayAddress, nil)
			if err != nil {
				m.logger.Error(ctx, "create http request for relay probe",
					slog.F("relay_address", peer.RelayAddress), slog.Error(err))
				return
			}
			client := http.Client{
				Timeout: m.options.PeerTimeout,
				Transport: &http.Transport{
					TLSClientConfig: m.options.TLSConfig,
				},
			}
			res, err := client.Do(req)
			if err != nil {
				mu.Lock()
				failed = append(failed, fmt.Sprintf("relay %s (%s): %s", peer.Hostname, peer.RelayAddress, err))
				mu.Unlock()
				return
			}
			_ = res.Body.Close()
		}()
	}
	wg.Wait()
	replicaError := sql.NullString{}
	if len(failed) > 0 {
		replicaError = sql.NullString{
			Valid:  true,
			String: fmt.Sprintf("Failed to dial peers: %s", strings.Join(failed, ", ")),
		}
	}

	databaseLatency, err := m.db.Ping(ctx)
	if err != nil {
		return xerrors.Errorf("ping database: %w", err)
	}

	replica, err := m.db.UpdateReplica(ctx, database.UpdateReplicaParams{
		ID:              m.self.ID,
		UpdatedAt:       database.Now(),
		StartedAt:       m.self.StartedAt,
		StoppedAt:       m.self.StoppedAt,
		RelayAddress:    m.self.RelayAddress,
		RegionID:        m.self.RegionID,
		Hostname:        m.self.Hostname,
		Version:         m.self.Version,
		Error:           replicaError,
		DatabaseLatency: int32(databaseLatency.Microseconds()),
	})
	if err != nil {
		return xerrors.Errorf("update replica: %w", err)
	}
	m.mutex.Lock()
	if m.self.Error.String != replica.Error.String {
		// Publish an update occurred!
		err = m.pubsub.Publish(PubsubEvent, []byte(m.self.ID.String()))
		if err != nil {
			m.mutex.Unlock()
			return xerrors.Errorf("publish replica update: %w", err)
		}
	}
	m.self = replica
	if m.callback != nil {
		go m.callback()
	}
	m.mutex.Unlock()
	return nil
}

// Self represents the current replica.
func (m *Manager) Self() database.Replica {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.self
}

// All returns every replica, including itself.
func (m *Manager) All() []database.Replica {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return append(m.peers, m.self)
}

// Regional returns all replicas in the same region excluding itself.
func (m *Manager) Regional() []database.Replica {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	replicas := make([]database.Replica, 0)
	for _, replica := range m.peers {
		if replica.RegionID != m.self.RegionID {
			continue
		}
		replicas = append(replicas, replica)
	}
	return replicas
}

// SetCallback sets a function to execute whenever new peers
// are refreshed or updated.
func (m *Manager) SetCallback(callback func()) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.callback = callback
	// Instantly call the callback to inform replicas!
	go callback()
}

func (m *Manager) Close() error {
	m.closeMutex.Lock()
	select {
	case <-m.closed:
		m.closeMutex.Unlock()
		return nil
	default:
	}
	close(m.closed)
	m.closeCancel()
	m.closeWait.Wait()
	m.closeMutex.Unlock()

	ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()
	_, err := m.db.UpdateReplica(ctx, database.UpdateReplicaParams{
		ID:        m.self.ID,
		UpdatedAt: database.Now(),
		StartedAt: m.self.StartedAt,
		StoppedAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
		RelayAddress: m.self.RelayAddress,
		RegionID:     m.self.RegionID,
		Hostname:     m.self.Hostname,
		Version:      m.self.Version,
		Error:        m.self.Error,
	})
	if err != nil {
		return xerrors.Errorf("update replica: %w", err)
	}
	err = m.pubsub.Publish(PubsubEvent, []byte(m.self.ID.String()))
	if err != nil {
		return xerrors.Errorf("publish replica update: %w", err)
	}
	return nil
}
