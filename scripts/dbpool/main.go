package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogjson"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
)

type mockTB struct{}

func (*mockTB) Cleanup(_ func()) {
	// noop, we won't be running cleanup
}

func (*mockTB) Helper() {
	// noop
}

func (*mockTB) Logf(format string, args ...any) {
	_, _ = fmt.Printf(format, args...)
}

type DBPool struct {
	numCleanupWorkers int

	availableDBs chan string
	garbageDBs   chan string
	dbRequests   chan struct{}
	ctx          context.Context
	Cancel       context.CancelFunc
	logger       *slog.Logger
}

type DBPoolArgs struct {
	PoolSize          int
	NumCleanupWorkers int
	Logger            *slog.Logger
}

func NewDBPool(args DBPoolArgs) *DBPool {
	ctx, cancel := context.WithCancel(context.Background())
	dbRequests := make(chan struct{}, args.PoolSize)
	for i := 0; i < args.PoolSize; i++ {
		dbRequests <- struct{}{}
	}
	args.Logger.Info(ctx, "starting db pool", slog.F("size", args.PoolSize), slog.F("action", "start"))
	return &DBPool{
		availableDBs:      make(chan string, args.PoolSize),
		garbageDBs:        make(chan string, args.PoolSize),
		dbRequests:        dbRequests,
		ctx:               ctx,
		Cancel:            cancel,
		logger:            args.Logger,
		numCleanupWorkers: args.NumCleanupWorkers,
	}
}

func (m *DBPool) GetDB(_ *int, reply *string) error {
	select {
	case dbURL := <-m.availableDBs:
		*reply = dbURL
		m.logger.Info(m.ctx, "db lease started", slog.F("action", "GetDB"), slog.F("db_url", dbURL))
		return nil
	case <-m.ctx.Done():
		return xerrors.Errorf("server context canceled while waiting for DB: %w", m.ctx.Err())
	}
}

func (m *DBPool) DisposeDB(dbURL *string, _ *int) error {
	select {
	case m.garbageDBs <- *dbURL:
		m.logger.Info(m.ctx, "db returned to pool for disposal", slog.F("action", "DisposeDB"), slog.F("db_url", *dbURL))
		return nil
	case <-m.ctx.Done():
		return xerrors.Errorf("could not dispose DB %s, server context canceled: %w", *dbURL, m.ctx.Err())
	}
}

func (m *DBPool) createDB() error {
	t := &mockTB{}
	dbURL, err := dbtestutil.Open(t)
	if err != nil {
		return xerrors.Errorf("open db: %w", err)
	}
	m.availableDBs <- dbURL
	m.logger.Info(m.ctx, "created db and added to pool", slog.F("action", "createDB"), slog.F("db_url", dbURL))
	return nil
}

func (m *DBPool) destroyDB(dbURL string) error {
	t := &mockTB{}
	connParams, err := dbtestutil.ParseDSN(dbURL)
	if err != nil {
		return xerrors.Errorf("parse dsn: %w", err)
	}
	if err := dbtestutil.RemoveDB(t, connParams.DBName); err != nil {
		return xerrors.Errorf("remove db: %w", err)
	}
	m.dbRequests <- struct{}{}
	m.logger.Info(m.ctx, "removed db from pool", slog.F("action", "destroyDB"), slog.F("db_url", dbURL))
	return nil
}

func (m *DBPool) cleanup() {
	wg := sync.WaitGroup{}
	for range m.numCleanupWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case dbURL := <-m.availableDBs:
					err := m.destroyDB(dbURL)
					if err != nil {
						m.logger.Error(m.ctx, "error destroying db in cleanup", slog.Error(err))
					}
				default:
					return
				}
			}
		}()
	}
	wg.Wait()
}

func (m *DBPool) Start(numCreateWorkers int, numDestroyWorkers int) {
	wg := sync.WaitGroup{}
	errChan := make(chan error, 1)
	for range numCreateWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-m.ctx.Done():
					return
				case <-m.dbRequests:
					if err := m.createDB(); err != nil {
						// we only care about the first error
						select {
						case errChan <- xerrors.Errorf("create db: %w", err):
						default:
						}
					}
				}
			}
		}()
	}
	for range numDestroyWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-m.ctx.Done():
					return
				case dbURL := <-m.garbageDBs:
					if err := m.destroyDB(dbURL); err != nil {
						// we only care about the first error
						select {
						case errChan <- xerrors.Errorf("destroy db: %w", err):
						default:
						}
					}
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-m.ctx.Done():
				return
			case err := <-errChan:
				m.logger.Error(m.ctx, "received error over channel", slog.Error(err))
				m.Cancel()
				return
			}
		}
	}()
	wg.Wait()

	m.cleanup()
}

var errAlreadyPrinted = xerrors.New("error already printed")

func inner(logger *slog.Logger) error {
	dbPool := NewDBPool(DBPoolArgs{
		PoolSize:          250,
		NumCleanupWorkers: 16,
		Logger:            logger,
	})

	osSignalChan := make(chan os.Signal, 1)
	signal.Notify(osSignalChan, syscall.SIGINT)

	// for both errChan and shutdownSignalChan, we buffer 16 to avoid deadlocks
	errChan := make(chan error, 16)
	shutdownSignalChan := make(chan struct{}, 16)
	dbPoolStoppedChan := make(chan struct{})
	shutdownTimeoutChan := make(chan struct{})

	go func() {
		<-osSignalChan
		shutdownSignalChan <- struct{}{}
	}()
	go func() {
		defer func() {
			shutdownSignalChan <- struct{}{}
		}()
		l, err := net.Listen("tcp", "localhost:8080")
		if err != nil {
			select {
			case errChan <- xerrors.Errorf("listen: %w", err):
			default:
			}
			return
		}
		if err := rpc.Register(dbPool); err != nil {
			select {
			case errChan <- xerrors.Errorf("register db manager: %w", err):
			default:
			}
			return
		}
		rpc.HandleHTTP()
		server := &http.Server{
			Addr:         l.Addr().String(),
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
		}
		dbPool.logger.Info(dbPool.ctx, "serving on port 8080")
		if err := server.Serve(l); err != nil && err != http.ErrServerClosed {
			select {
			case errChan <- xerrors.Errorf("serve: %w", err):
			default:
			}
		}
	}()
	go func() {
		<-shutdownSignalChan
		dbPool.Cancel()
		time.Sleep(15 * time.Second)
		close(shutdownTimeoutChan)
	}()
	go func() {
		dbPool.Start(10, 10)
		close(dbPoolStoppedChan)
	}()

	select {
	case <-dbPoolStoppedChan:
		dbPool.logger.Info(dbPool.ctx, "cleaned up, exiting gracefully")
	case <-shutdownTimeoutChan:
		select {
		case errChan <- xerrors.Errorf("timed out waiting for server to clean up"):
		default:
		}
	}

	errorPrinted := false
	for {
		select {
		case err := <-errChan:
			dbPool.logger.Error(dbPool.ctx, "an error occurred", slog.Error(err))
			errorPrinted = true
		default:
			goto finishLine
		}
	}

finishLine:
	if errorPrinted {
		return errAlreadyPrinted
	}
	return nil
}

func main() {
	logger := slog.Make(slogjson.Sink(os.Stdout))
	if err := inner(&logger); err != nil {
		if !errors.Is(err, errAlreadyPrinted) {
			logger.Error(context.Background(), "an error occurred, exiting", slog.Error(err))
		}
		os.Exit(1)
	}
}
