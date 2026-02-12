package catalog

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/serpent"
)

const (
	CDevLabelEphemeral = "cdev/ephemeral"
	CDevLabelCache     = "cdev/cache"
)

type ServiceBase interface {
	// Name returns a unique identifier for this service.
	Name() string

	// Emoji returns a single emoji used to identify this service
	// in log output.
	Emoji() string

	// DependsOn returns the names of services this service depends on before "Start" can be called.
	// This is used to determine the order in which services should be started and stopped.
	DependsOn() []string

	// Start launches the service. This should not block.
	Start(ctx context.Context, logger slog.Logger, c *Catalog) error

	// Stop gracefully shuts down the service.
	Stop(ctx context.Context) error
}

type ConfigurableService interface {
	ServiceBase
	Options() serpent.OptionSet
}

type Service[Result any] interface {
	ServiceBase
	// Result is usable by other services.
	Result() Result
}

type configurator struct {
	target string
	apply  func(ServiceBase)
}

type Catalog struct {
	mu       sync.RWMutex
	services map[string]ServiceBase
	loggers  map[string]slog.Logger
	logger   slog.Logger
	w        io.Writer

	manager *unit.Manager

	configurators []configurator
	configured    bool
}

func New() *Catalog {
	return &Catalog{
		services: make(map[string]ServiceBase),
		loggers:  make(map[string]slog.Logger),
		manager:  unit.NewManager(),
	}
}

// Init sets the writer and builds the base logger and all
// per-service loggers. Call this after registration and before
// Start.
func (c *Catalog) Init(w io.Writer) {
	c.w = w
	c.logger = slog.Make(NewLoggerSink(w, nil))
	for name, svc := range c.services {
		c.loggers[name] = slog.Make(NewLoggerSink(w, svc))
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

func (c *Catalog) ForEach(f func(s ServiceBase) error) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, srv := range c.services {
		if err := f(srv); err != nil {
			return err
		}
	}
	return nil
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
	if err := c.manager.Register(unit.ID(name)); err != nil && !xerrors.Is(err, unit.ErrUnitAlreadyRegistered) {
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

// Configure registers a typed callback to mutate a target service
// before startup. Panics if called after ApplyConfigurations.
func Configure[T ServiceBase](c *Catalog, target string, fn func(T)) {
	if c.configured {
		panic(fmt.Sprintf("catalog: Configure(%q) called after ApplyConfigurations", target))
	}
	c.configurators = append(c.configurators, configurator{
		target: target,
		apply: func(s ServiceBase) {
			typed, ok := s.(T)
			if !ok {
				panic(fmt.Sprintf("catalog: Configure(%q) type mismatch: got %T", target, s))
			}
			fn(typed)
		},
	})
}

// ApplyConfigurations runs all registered Configure callbacks,
// then prevents further Configure calls. Must be called after
// option parsing but before Start.
func (c *Catalog) ApplyConfigurations() error {
	for _, cfg := range c.configurators {
		svc, ok := c.services[cfg.target]
		if !ok {
			return xerrors.Errorf("configure target %q not found", cfg.target)
		}
		cfg.apply(svc)
	}
	c.configured = true
	return nil
}

// Start launches all registered services concurrently.
// Services block until their dependencies (tracked by unit.Manager) are ready.
func (c *Catalog) Start(ctx context.Context) error {
	c.mu.Lock()

	// Log the service dependency graph on startup.
	c.logger.Info(ctx, "service dependency graph")
	for _, srv := range c.services {
		deps := srv.DependsOn()
		if len(deps) == 0 {
			c.logger.Info(ctx, fmt.Sprintf("  %s %s (no dependencies)", srv.Emoji(), srv.Name()))
		} else {
			c.logger.Info(ctx, fmt.Sprintf("  %s %s -> [%s]", srv.Emoji(), srv.Name(), strings.Join(deps, ", ")))
		}
	}

	type svcEntry struct {
		srv    ServiceBase
		logger slog.Logger
	}
	entries := make([]svcEntry, 0, len(c.services))
	for _, srv := range c.services {
		entries = append(entries, svcEntry{srv: srv, logger: c.loggers[srv.Name()]})
	}
	c.mu.Unlock()

	wg, ctx := errgroup.WithContext(ctx)
	wg.SetLimit(-1) // No limit on concurrency, since unit.Manager tracks dependencies.
	for _, e := range entries {
		wg.Go(func() (failure error) {
			defer func() {
				if err := recover(); err != nil {
					failure = xerrors.Errorf("panic: %v", err)
				}
			}()
			name := e.srv.Name()
			svcLogger := e.logger

			if err := c.waitForReady(ctx, name); err != nil {
				return xerrors.Errorf("wait for %s to be ready: %w", name, err)
			}

			if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusStarted); err != nil {
				return xerrors.Errorf("update status for %s: %w", name, err)
			}

			svcLogger.Info(ctx, "starting service")
			if err := e.srv.Start(ctx, svcLogger, c); err != nil {
				return xerrors.Errorf("start %s: %w", name, err)
			}

			// Mark as complete after starting, which allows dependent services to start.
			if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusComplete); err != nil {
				return xerrors.Errorf("update status for %s: %w", name, err)
			}

			svcLogger.Info(ctx, "service started", slog.F("name", name))
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
	logTicker := time.NewTicker(5 * time.Second)
	defer logTicker.Stop()

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
		case <-logTicker.C:
			unmet, _ := c.manager.GetUnmetDependencies(unit.ID(name))
			if len(unmet) > 0 {
				depNames := make([]string, 0, len(unmet))
				for _, d := range unmet {
					depNames = append(depNames, string(d.DependsOn))
				}
				c.loggers[name].Info(ctx, "waiting for dependencies",
					slog.F("unmet", strings.Join(depNames, ", ")))
			}
		default:
			time.Sleep(time.Millisecond * 15)
			continue
		}
	}
}
