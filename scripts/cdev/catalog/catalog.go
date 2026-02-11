package catalog

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/unit"
)

const (
	CDevLabelEphemeral = "cdev/ephemeral"
	CDevLabelCache     = "cdev/cache"
)

type ServiceBase interface {
	// Name returns a unique identifier for this service.
	Name() string

	// DependsOn returns the names of services this service depends on before "Start" can be called.
	// This is used to determine the order in which services should be started and stopped.
	DependsOn() []string

	// Start launches the service. This should not block
	Start(ctx context.Context, c *Catalog) error

	// Stop gracefully shuts down the service.
	Stop(ctx context.Context) error
}

type Service[Result any] interface {
	ServiceBase
	// Result is usable by other services.
	Result() Result
}

type Catalog struct {
	mu       sync.RWMutex
	services map[string]ServiceBase
	logger   slog.Logger

	manager *unit.Manager
}

func New(logger slog.Logger) *Catalog {
	return &Catalog{
		services: make(map[string]ServiceBase),
		manager:  unit.NewManager(),
		logger:   logger,
	}
}

// Logger returns the catalog's logger.
func (c *Catalog) Logger() slog.Logger {
	return c.logger
}

func Get[T Service[R], R any](c *Catalog) R {
	var zero T

	s, ok := c.Get(zero.Name())
	if !ok {
		panic(fmt.Sprintf("catalog.Get[%q] not found", zero.Name()))
	}
	typed, ok := s.(T)
	if !ok {
		panic(fmt.Sprintf("catalog.Get[%q] has wrong type: %T", zero.Name(), s))
	}
	return typed.Result()
}

func (c *Catalog) Register(s ...ServiceBase) error {
	for _, srv := range s {
		if err := c.registerOne(srv); err != nil {
			return err
		}
	}
	return nil
}

func (c *Catalog) registerOne(s ServiceBase) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	name := s.Name()
	if _, exists := c.services[name]; exists {
		return xerrors.Errorf("service %q already registered", name)
	}

	// Register with unit manager.
	if err := c.manager.Register(unit.ID(name)); err != nil {
		return xerrors.Errorf("register %s with manager: %w", name, err)
	}

	// Add dependencies.
	for _, dep := range s.DependsOn() {
		// Register dependency if not already registered (it may not exist yet).
		_ = c.manager.Register(unit.ID(dep))
		if err := c.manager.AddDependency(unit.ID(name), unit.ID(dep), unit.StatusComplete); err != nil {
			return xerrors.Errorf("add dependency %s -> %s: %w", name, dep, err)
		}
	}

	c.services[name] = s
	return nil
}

func (c *Catalog) MustGet(name string) ServiceBase {
	s, ok := c.Get(name)
	if !ok {
		panic(fmt.Sprintf("catalog.MustGet: service %q not found", name))
	}
	return s
}

// Get returns a service by name.
func (c *Catalog) Get(name string) (ServiceBase, bool) {
	s, ok := c.services[name]
	return s, ok
}

// Start launches all registered services concurrently.
// Services block until their dependencies (tracked by unit.Manager) are ready.
func (c *Catalog) Start(ctx context.Context, logger slog.Logger) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	wg, ctx := errgroup.WithContext(ctx)
	wg.SetLimit(-1) // No limit on concurrency, since unit.Manager tracks dependencies.
	for _, srv := range c.services {
		wg.Go(func() (failure error) {
			defer func() {
				if err := recover(); err != nil {
					failure = fmt.Errorf("panic: %v", err)
				}
			}()
			name := srv.Name()

			if err := c.waitForReady(ctx, name); err != nil {
				return xerrors.Errorf("wait for %s to be ready: %w", name, err)
			}

			if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusStarted); err != nil {
				return xerrors.Errorf("update status for %s: %w", name, err)
			}

			logger.Info(ctx, "Starting service",
				slog.F("name", name),
			)
			// Start the service.
			if err := srv.Start(ctx, c); err != nil {
				return xerrors.Errorf("start %s: %w", name, err)
			}

			// Mark as complete after starting, which allows dependent services to start.
			if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusComplete); err != nil {
				return xerrors.Errorf("update status for %s: %w", name, err)
			}

			return nil
		})
	}

	err := wg.Wait()
	if err != nil {
		return xerrors.Errorf("start services: %w", err)
	}

	return nil
}

// waitForReady polls until the service's dependencies are satisfied.
func (c *Catalog) waitForReady(ctx context.Context, name string) error {
	for {
		ready, err := c.manager.IsReady(unit.ID(name))
		if err != nil {
			return err
		}
		if ready {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Small sleep to avoid busy loop - could use a channel-based approach for better perf.
			time.Sleep(time.Millisecond * 15)
			continue
		}
	}
}
