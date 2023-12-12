package replicasync

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
)

var PubsubEvent = "replica"

type Options struct {
	ID              uuid.UUID
	CleanupInterval time.Duration
	UpdateInterval  time.Duration
	PeerTimeout     time.Duration
	RelayAddress    string
	RegionID        int32
	TLSConfig       *tls.Config
}

// New registers the replica with the database and periodically updates to ensure
// it's healthy. It contacts all other alive replicas to ensure they are reachable.
func New(ctx context.Context, logger slog.Logger, db database.Store, ps pubsub.Pubsub, options *Options) (*Manager, error) {
	if options == nil {
		options = &Options{}
	}
	if options.ID == uuid.Nil {
		options.ID = uuid.New()
	}
	if options.PeerTimeout == 0 {
		options.PeerTimeout = 3 * time.Second
	}
	if options.UpdateInterval == 0 {
		options.UpdateInterval = 5 * time.Second
	}
	if options.CleanupInterval == 0 {
		// The cleanup interval can be quite long, because it's
		// primary purpose is to clean up dead replicas.
		options.CleanupInterval = 30 * time.Minute
	}
	hostname := cliutil.Hostname()
	databaseLatency, err := db.Ping(ctx)
	if err != nil {
		return nil, xerrors.Errorf("ping database: %w", err)
	}
	// nolint:gocritic // Inserting a replica is a system function.
	replica, err := db.InsertReplica(dbauthz.AsSystemRestricted(ctx), database.InsertReplicaParams{
		ID:              options.ID,
		CreatedAt:       dbtime.Now(),
		StartedAt:       dbtime.Now(),
		UpdatedAt:       dbtime.Now(),
		Hostname:        hostname,
		RegionID:        options.RegionID,
		RelayAddress:    options.RelayAddress,
		Version:         buildinfo.Version(),
		DatabaseLatency: int32(databaseLatency.Microseconds()),
		Primary:         true,
	})
	if err != nil {
		return nil, xerrors.Errorf("insert replica: %w", err)
	}
	err = ps.Publish(PubsubEvent, []byte(options.ID.String()))
	if err != nil {
		return nil, xerrors.Errorf("publish new replica: %w", err)
	}
	ctx, cancelFunc := context.WithCancel(ctx)
	manager := &Manager{
		id:          options.ID,
		options:     options,
		db:          db,
		pubsub:      ps,
		self:        replica,
		logger:      logger,
		closed:      make(chan struct{}),
		closeCancel: cancelFunc,
	}
	err = manager.syncReplicas(ctx)
	if err != nil {
		return nil, xerrors.Errorf("run replica: %w", err)
	}
	err = manager.subscribe(ctx)
	if err != nil {
		return nil, xerrors.Errorf("subscribe: %w", err)
	}
	manager.closeWait.Add(1)
	go manager.loop(ctx)
	return manager, nil
}

// Manager keeps the replica up to date and in sync with other replicas.
type Manager struct {
	id      uuid.UUID
	options *Options
	db      database.Store
	pubsub  pubsub.Pubsub
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

func (m *Manager) ID() uuid.UUID {
	return m.id
}

// UpdateNow synchronously updates replicas.
func (m *Manager) UpdateNow(ctx context.Context) error {
	return m.syncReplicas(ctx)
}

// PublishUpdate notifies all other replicas to update.
func (m *Manager) PublishUpdate() error {
	return m.pubsub.Publish(PubsubEvent, []byte(m.id.String()))
}

// updateInterval is used to determine a replicas state.
// If the replica was updated > the time, it's considered healthy.
// If the replica was updated < the time, it's considered stale.
func (m *Manager) updateInterval() time.Time {
	return dbtime.Now().Add(-3 * m.options.UpdateInterval)
}

// loop runs the replica update sequence on an update interval.
func (m *Manager) loop(ctx context.Context) {
	defer m.closeWait.Done()
	updateTicker := time.NewTicker(m.options.UpdateInterval)
	defer updateTicker.Stop()
	deleteTicker := time.NewTicker(m.options.CleanupInterval)
	defer deleteTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deleteTicker.C:
			// nolint:gocritic // Deleting a replica is a system function
			err := m.db.DeleteReplicasUpdatedBefore(dbauthz.AsSystemRestricted(ctx), m.updateInterval())
			if err != nil {
				m.logger.Warn(ctx, "delete old replicas", slog.Error(err))
			}
			continue
		case <-updateTicker.C:
		}
		err := m.syncReplicas(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			m.logger.Warn(ctx, "run replica update loop", slog.Error(err))
		}
	}
}

// subscribe listens for new replica information!
func (m *Manager) subscribe(ctx context.Context) error {
	var (
		needsUpdate = false
		updating    = false
		updateMutex = sync.Mutex{}
	)

	// This loop will continually update nodes as updates are processed.
	// The intent is to always be up to date without spamming the run
	// function, so if a new update comes in while one is being processed,
	// it will reprocess afterwards.
	var update func()
	update = func() {
		err := m.syncReplicas(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			m.logger.Warn(ctx, "run replica from subscribe", slog.Error(err))
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
		if id == m.id {
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

func (m *Manager) syncReplicas(ctx context.Context) error {
	m.closeMutex.Lock()
	select {
	case <-m.closed:
		m.closeMutex.Unlock()
		return xerrors.New("manager is closed")
	default:
	}
	m.closeWait.Add(1)
	m.closeMutex.Unlock()
	defer m.closeWait.Done()
	// Expect replicas to update once every three times the interval...
	// If they don't, assume death!
	// nolint:gocritic // Reading replicas is a system function
	replicas, err := m.db.GetReplicasUpdatedAfter(dbauthz.AsSystemRestricted(ctx), m.updateInterval())
	if err != nil {
		return xerrors.Errorf("get replicas: %w", err)
	}

	m.mutex.Lock()
	m.peers = make([]database.Replica, 0, len(replicas))
	for _, replica := range replicas {
		if replica.ID == m.id {
			continue
		}
		m.peers = append(m.peers, replica)
	}
	m.mutex.Unlock()

	client := http.Client{
		Timeout: m.options.PeerTimeout,
		Transport: &http.Transport{
			TLSClientConfig: m.options.TLSConfig,
		},
	}
	defer client.CloseIdleConnections()
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := make([]string, 0)
	for _, peer := range m.Regional() {
		wg.Add(1)
		go func(peer database.Replica) {
			defer wg.Done()
			ra, err := url.Parse(peer.RelayAddress)
			if err != nil {
				m.logger.Warn(ctx, "could not parse relay address",
					slog.F("relay_address", peer.RelayAddress), slog.Error(err))
				return
			}
			target, err := ra.Parse("/derp/latency-check")
			if err != nil {
				m.logger.Warn(ctx, "could not resolve /derp/latency-check endpoint",
					slog.F("relay_address", peer.RelayAddress), slog.Error(err))
				return
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
			if err != nil {
				m.logger.Warn(ctx, "create http request for relay probe",
					slog.F("relay_address", peer.RelayAddress), slog.Error(err))
				return
			}
			res, err := client.Do(req)
			if err != nil {
				mu.Lock()
				failed = append(failed, fmt.Sprintf("relay %s (%s): %s", peer.Hostname, peer.RelayAddress, err))
				mu.Unlock()
				return
			}
			_ = res.Body.Close()
		}(peer)
	}
	wg.Wait()
	replicaError := ""
	if len(failed) > 0 {
		replicaError = fmt.Sprintf("Failed to dial peers: %s", strings.Join(failed, ", "))
	}

	databaseLatency, err := m.db.Ping(ctx)
	if err != nil {
		return xerrors.Errorf("ping database: %w", err)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	// nolint:gocritic // Updating a replica is a system function.
	replica, err := m.db.UpdateReplica(dbauthz.AsSystemRestricted(ctx), database.UpdateReplicaParams{
		ID:              m.self.ID,
		UpdatedAt:       dbtime.Now(),
		StartedAt:       m.self.StartedAt,
		StoppedAt:       m.self.StoppedAt,
		RelayAddress:    m.self.RelayAddress,
		RegionID:        m.self.RegionID,
		Hostname:        m.self.Hostname,
		Version:         m.self.Version,
		Error:           replicaError,
		DatabaseLatency: int32(databaseLatency.Microseconds()),
		Primary:         m.self.Primary,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("update replica: %w", err)
		}
		// self replica has been cleaned up, we must reinsert
		// nolint:gocritic // Updating a replica is a system function.
		replica, err = m.db.InsertReplica(dbauthz.AsSystemRestricted(ctx), database.InsertReplicaParams{
			ID:              m.self.ID,
			CreatedAt:       dbtime.Now(),
			UpdatedAt:       dbtime.Now(),
			StartedAt:       m.self.StartedAt,
			RelayAddress:    m.self.RelayAddress,
			RegionID:        m.self.RegionID,
			Hostname:        m.self.Hostname,
			Version:         m.self.Version,
			DatabaseLatency: int32(databaseLatency.Microseconds()),
			Primary:         m.self.Primary,
		})
		if err != nil {
			return xerrors.Errorf("update replica: %w", err)
		}
	}
	if m.self.Error != replica.Error {
		// Publish an update occurred!
		err = m.PublishUpdate()
		if err != nil {
			return xerrors.Errorf("publish replica update: %w", err)
		}
	}
	m.self = replica
	if m.callback != nil {
		go m.callback()
	}
	return nil
}

// Self represents the current replica.
func (m *Manager) Self() database.Replica {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.self
}

// AllPrimary returns every primary replica (not workspace proxy replicas),
// including itself.
func (m *Manager) AllPrimary() []database.Replica {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	replicas := make([]database.Replica, 0, len(m.peers))
	for _, replica := range append(m.peers, m.self) {
		if !replica.Primary {
			continue
		}

		// When we assign the non-pointer to a
		// variable it loses the reference.
		replica := replica
		replicas = append(replicas, replica)
	}
	return replicas
}

// InRegion returns every replica in the given DERP region excluding itself.
func (m *Manager) InRegion(regionID int32) []database.Replica {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	replicas := make([]database.Replica, 0)
	for _, replica := range m.peers {
		if replica.RegionID != regionID {
			continue
		}
		replicas = append(replicas, replica)
	}
	return replicas
}

// Regional returns all replicas in the same region excluding itself.
func (m *Manager) Regional() []database.Replica {
	return m.InRegion(m.regionID())
}

func (m *Manager) regionID() int32 {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.self.RegionID
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
	m.closeMutex.Unlock()
	m.closeWait.Wait()
	m.mutex.Lock()
	defer m.mutex.Unlock()
	ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()
	// nolint:gocritic // Updating a replica is a system function.
	_, err := m.db.UpdateReplica(dbauthz.AsSystemRestricted(ctx), database.UpdateReplicaParams{
		ID:        m.self.ID,
		UpdatedAt: dbtime.Now(),
		StartedAt: m.self.StartedAt,
		StoppedAt: sql.NullTime{
			Time:  dbtime.Now(),
			Valid: true,
		},
		RelayAddress:    m.self.RelayAddress,
		RegionID:        m.self.RegionID,
		Hostname:        m.self.Hostname,
		Version:         m.self.Version,
		Error:           m.self.Error,
		DatabaseLatency: 0,     // A stopped replica has no latency.
		Primary:         false, // A stopped replica cannot be primary.
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
