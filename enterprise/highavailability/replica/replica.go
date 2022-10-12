package replica

import (
	"context"
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
}

// New registers the replica with the database and periodically updates to ensure
// it's healthy. It contacts all other alive replicas to ensure they are reachable.
func New(ctx context.Context, logger slog.Logger, db database.Store, pubsub database.Pubsub, options Options) (*Server, error) {
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
	var replica database.Replica
	_, err = db.GetReplicaByID(ctx, options.ID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, xerrors.Errorf("get replica: %w", err)
		}
		replica, err = db.InsertReplica(ctx, database.InsertReplicaParams{
			ID:           options.ID,
			CreatedAt:    database.Now(),
			StartedAt:    database.Now(),
			UpdatedAt:    database.Now(),
			Hostname:     hostname,
			RegionID:     options.RegionID,
			RelayAddress: options.RelayAddress,
			Version:      buildinfo.Version(),
		})
		if err != nil {
			return nil, xerrors.Errorf("insert replica: %w", err)
		}
	} else {
		replica, err = db.UpdateReplica(ctx, database.UpdateReplicaParams{
			ID:           options.ID,
			UpdatedAt:    database.Now(),
			StartedAt:    database.Now(),
			StoppedAt:    sql.NullTime{},
			RelayAddress: options.RelayAddress,
			RegionID:     options.RegionID,
			Hostname:     hostname,
			Version:      buildinfo.Version(),
			Error:        sql.NullString{},
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
	server := &Server{
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
	err = server.subscribe(ctx)
	if err != nil {
		return nil, xerrors.Errorf("subscribe: %w", err)
	}
	server.closeWait.Add(1)
	go server.loop(ctx)
	return server, nil
}

type Server struct {
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
func (s *Server) loop(ctx context.Context) {
	defer s.closeWait.Done()
	ticker := time.NewTicker(s.options.UpdateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		err := s.run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Warn(ctx, "run replica update loop", slog.Error(err))
		}
	}
}

// subscribe listens for new replica information!
func (s *Server) subscribe(ctx context.Context) error {
	needsUpdate := false
	updating := false
	updateMutex := sync.Mutex{}

	// This loop will continually update nodes as updates are processed.
	// The intent is to always be up to date without spamming the run
	// function, so if a new update comes in while one is being processed,
	// it will reprocess afterwards.
	var update func()
	update = func() {
		err := s.run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Error(ctx, "run replica from subscribe", slog.Error(err))
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
	cancelFunc, err := s.pubsub.Subscribe(PubsubEvent, func(ctx context.Context, message []byte) {
		updateMutex.Lock()
		defer updateMutex.Unlock()
		id, err := uuid.Parse(string(message))
		if err != nil {
			return
		}
		// Don't process updates for ourself!
		if id == s.options.ID {
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

func (s *Server) run(ctx context.Context) error {
	s.closeMutex.Lock()
	s.closeWait.Add(1)
	s.closeMutex.Unlock()
	go func() {
		s.closeWait.Done()
	}()
	// Expect replicas to update once every three times the interval...
	// If they don't, assume death!
	replicas, err := s.db.GetReplicasUpdatedAfter(ctx, database.Now().Add(-3*s.options.UpdateInterval))
	if err != nil {
		return xerrors.Errorf("get replicas: %w", err)
	}

	s.mutex.Lock()
	s.peers = make([]database.Replica, 0, len(replicas))
	for _, replica := range replicas {
		if replica.ID == s.options.ID {
			continue
		}
		s.peers = append(s.peers, replica)
	}
	s.mutex.Unlock()

	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := make([]string, 0)
	for _, peer := range s.Regional() {
		wg.Add(1)
		peer := peer
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, peer.RelayAddress, nil)
			if err != nil {
				s.logger.Error(ctx, "create http request for relay probe",
					slog.F("relay_address", peer.RelayAddress), slog.Error(err))
				return
			}
			client := http.Client{
				Timeout: s.options.PeerTimeout,
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

	replica, err := s.db.UpdateReplica(ctx, database.UpdateReplicaParams{
		ID:           s.self.ID,
		UpdatedAt:    database.Now(),
		StartedAt:    s.self.StartedAt,
		StoppedAt:    s.self.StoppedAt,
		RelayAddress: s.self.RelayAddress,
		RegionID:     s.self.RegionID,
		Hostname:     s.self.Hostname,
		Version:      s.self.Version,
		Error:        replicaError,
	})
	if err != nil {
		return xerrors.Errorf("update replica: %w", err)
	}
	s.mutex.Lock()
	if s.self.Error.String != replica.Error.String {
		// Publish an update occurred!
		err = s.pubsub.Publish(PubsubEvent, []byte(s.self.ID.String()))
		if err != nil {
			s.mutex.Unlock()
			return xerrors.Errorf("publish replica update: %w", err)
		}
	}
	s.self = replica
	if s.callback != nil {
		go s.callback()
	}
	s.mutex.Unlock()
	return nil
}

// Self represents the current replica.
func (s *Server) Self() database.Replica {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.self
}

// All returns every replica, including itself.
func (s *Server) All() []database.Replica {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append(s.peers, s.self)
}

// Regional returns all replicas in the same region excluding itself.
func (s *Server) Regional() []database.Replica {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	replicas := make([]database.Replica, 0)
	for _, replica := range s.peers {
		if replica.RegionID != s.self.RegionID {
			continue
		}
		replicas = append(replicas, replica)
	}
	return replicas
}

// SetCallback sets a function to execute whenever new peers
// are refreshed or updated.
func (s *Server) SetCallback(callback func()) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.callback = callback
	// Instantly call the callback to inform replicas!
	go callback()
}

func (s *Server) Close() error {
	s.closeMutex.Lock()
	select {
	case <-s.closed:
		s.closeMutex.Unlock()
		return nil
	default:
	}
	close(s.closed)
	s.closeCancel()
	s.closeWait.Wait()
	s.closeMutex.Unlock()

	ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()
	_, err := s.db.UpdateReplica(ctx, database.UpdateReplicaParams{
		ID:        s.self.ID,
		UpdatedAt: database.Now(),
		StartedAt: s.self.StartedAt,
		StoppedAt: sql.NullTime{
			Time:  database.Now(),
			Valid: true,
		},
		RelayAddress: s.self.RelayAddress,
		RegionID:     s.self.RegionID,
		Hostname:     s.self.Hostname,
		Version:      s.self.Version,
		Error:        s.self.Error,
	})
	if err != nil {
		return xerrors.Errorf("update replica: %w", err)
	}
	err = s.pubsub.Publish(PubsubEvent, []byte(s.self.ID.String()))
	if err != nil {
		return xerrors.Errorf("publish replica update: %w", err)
	}
	return nil
}
