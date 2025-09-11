package broker

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"golang.org/x/xerrors"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/retry"
)

// Service for the database broker. The broker's job is to make temporary copies of template databases for use in
// testing. Tests should Clone a database and then Discard it when they are done (typically in a t.Cleanup).
//
// The broker is designed to run in a separate process from the main test suite, connected over stdio. If the main test
// process ends (panics, times out, or is killed) without explicitly discarding the databases it clones, the broker
// removes them so they don't leak beyond the test session. c.f. https://github.com/coder/internal/issues/927
type Service struct {
	dRPCServer *drpcserver.Server
	session    *yamux.Session
	db         *sql.DB

	mu        sync.Mutex
	cleaned   bool
	databases map[string]struct{}
}

// Query is a RPC used by the main test process to discover whether a database with a given name exists. E.g. to
// determine whether a template has been created before cloning based on it.
func (s *Service) Query(ctx context.Context, request *QueryRequest) (*QueryResponse, error) {
	resp := &QueryResponse{Status: &Status{}}
	rows, err := s.db.QueryContext(ctx, "SELECT 1 FROM pg_database WHERE datname = $1", request.DbName)
	if err != nil {
		resp.Status.Code = Status_ERR_POSTGRES_CONNECTION
		resp.Status.Message = err.Error()
		return resp, nil
	}
	defer rows.Close()
	if rows.Next() {
		resp.Status.Code = Status_OK
		return resp, nil
	}
	resp.Status.Code = Status_ERR_DB_NOT_FOUND
	resp.Status.Message = fmt.Sprintf("database '%s' not found", request.DbName)
	return resp, nil
}

// Clone makes a copy of a database given by the TemplateDbName.
func (s *Service) Clone(ctx context.Context, request *CloneRequest) (*CloneResponse, error) {
	resp := &CloneResponse{Status: &Status{}}

	if err := validateDBName(request.TemplateDbName); err != nil {
		resp.Status.Code = Status_ERR_BAD_DB_NAME
		resp.Status.Message = err.Error()
		return resp, nil
	}

	now := time.Now().Format("test_2006_01_02_15_04_05")
	dbSuffix, err := cryptorand.StringCharset(cryptorand.Lower, 10)
	if err != nil {
		resp.Status.Code = Status_ERR_INTERNAL
		resp.Status.Message = err.Error()
		return resp, nil
	}
	dbName := now + "_" + dbSuffix
	s.mu.Lock()
	if s.cleaned {
		s.mu.Unlock()
		resp.Status.Code = Status_ERR_INTERNAL
		resp.Status.Message = "clone request after cleanup"
		return resp, nil
	}
	s.databases[dbName] = struct{}{}
	s.mu.Unlock()

	_, err = s.db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s WITH TEMPLATE %s", dbName, request.TemplateDbName))
	if err != nil {
		resp.Status.Code = Status_ERR_POSTGRES_CONNECTION
		resp.Status.Message = err.Error()
		return resp, nil
	}
	resp.Status.Code = Status_OK
	resp.DbName = dbName
	return resp, nil
}

var validDBName = regexp.MustCompile("^[a-zA-Z0-9_]+$")

func validateDBName(name string) error {
	if validDBName.MatchString(name) {
		return nil
	}
	return xerrors.Errorf("invalid db name: %s", name)
}

// Discard a database by name.
func (s *Service) Discard(ctx context.Context, request *DiscardRequest) (*DiscardResponse, error) {
	resp := &DiscardResponse{Status: &Status{}}
	if err := validateDBName(request.DbName); err != nil {
		resp.Status.Code = Status_ERR_BAD_DB_NAME
		resp.Status.Message = err.Error()
		return resp, nil
	}
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE %s", request.DbName))
	if err != nil {
		resp.Status.Code = Status_ERR_POSTGRES_CONNECTION
		resp.Status.Message = err.Error()
		return resp, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cleaned {
		resp.Status.Code = Status_ERR_INTERNAL
		resp.Status.Message = "discard request after cleanup"
		return resp, nil
	}
	delete(s.databases, request.DbName)
	resp.Status.Code = Status_OK
	return resp, nil
}

func NewService(ctx context.Context, r io.Reader, w io.WriteCloser) (*Service, error) {
	conn := &readWriteCloser{r, w}
	config := yamux.DefaultConfig()
	config.LogOutput = io.Discard
	session, err := yamux.Server(conn, config)
	if err != nil {
		return nil, xerrors.Errorf("yamux init failed: %w", err)
	}
	service := &Service{
		session:   session,
		databases: make(map[string]struct{}),
	}
	err = service.initDB(ctx)
	if err != nil {
		return nil, xerrors.Errorf("db init failed: %w", err)
	}

	mux := drpcmux.New()
	err = DRPCRegisterBroker(mux, service)
	if err != nil {
		return nil, xerrors.Errorf("register broker failed: %w", err)
	}
	service.dRPCServer = drpcserver.New(mux)
	return service, nil
}

func (s *Service) Serve(ctx context.Context) error {
	defer s.cleanUp()
	return s.dRPCServer.Serve(ctx, s.session)
}

func (s *Service) initDB(ctx context.Context) error {
	username := os.Getenv("DB_BROKER_USERNAME")
	password := os.Getenv("DB_BROKER_PASSWORD")
	portStr := os.Getenv("DB_BROKER_PORT")
	var err error
	port := 5432
	if portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return xerrors.Errorf("couldn't parse port: %w", err)
		}
	}
	if username == "" {
		username = "postgres"
	}
	if password == "" {
		password = "postgres"
	}

	dsn := fmt.Sprintf("postgres://%s:%s@localhost:%d/?sslmode=disable", username, password, port)
	s.db, err = sql.Open("postgres", dsn)
	if err != nil {
		return xerrors.Errorf("couldn't open DB: %w", err)
	}
	// we don't want this service to compete too much with tests for postgres connections
	s.db.SetMaxOpenConns(8)
	for r := retry.New(10*time.Millisecond, 500*time.Millisecond); r.Wait(ctx); {
		err = s.db.PingContext(ctx)
		if err == nil {
			return nil
		}
	}
	return ctx.Err()
}

func (s *Service) cleanUp() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s.mu.Lock()
	databases := s.databases
	s.cleaned = true
	s.mu.Unlock()

	for dbName := range databases {
		if ctx.Err() != nil {
			return // timed out
		}
		_, _ = s.db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE %s", dbName))
	}
}

type readWriteCloser struct {
	r io.Reader
	w io.WriteCloser
}

func (rw *readWriteCloser) Read(p []byte) (int, error) {
	return rw.r.Read(p)
}

func (rw *readWriteCloser) Write(p []byte) (int, error) {
	return rw.w.Write(p)
}

func (rw *readWriteCloser) Close() error {
	return rw.w.Close()
}
