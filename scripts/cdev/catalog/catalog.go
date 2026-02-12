package catalog

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/serpent"
)

const (
	CDevLabelEphemeral = "cdev/ephemeral"
	CDevLabelCache     = "cdev/cache"
)

type ServiceBase interface {
	// Name returns a unique identifier for this service.
	Name() ServiceName

	// Emoji returns a single emoji used to identify this service
	// in log output.
	Emoji() string

	// DependsOn returns the names of services this service depends on before "Start" can be called.
	// This is used to determine the order in which services should be started and stopped.
	DependsOn() []ServiceName

	// CurrentStep returns a human-readable description of what the service
	// is currently doing. Returns empty string if idle/complete.
	CurrentStep() string

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
	target ServiceName
	apply  func(ServiceBase)
}

type Catalog struct {
	mu       sync.RWMutex
	services map[ServiceName]ServiceBase
	loggers  map[ServiceName]slog.Logger
	logger   slog.Logger
	w        io.Writer

	manager *unit.Manager

	subscribers   map[chan struct{}]struct{}
	subscribersMu sync.Mutex

	configurators []configurator
	configured    bool
}

func New() *Catalog {
	return &Catalog{
		services:    make(map[ServiceName]ServiceBase),
		loggers:     make(map[ServiceName]slog.Logger),
		manager:     unit.NewManager(),
		subscribers: make(map[chan struct{}]struct{}),
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

func (c *Catalog) MustGet(name ServiceName) ServiceBase {
	s, ok := c.Get(name)
	if !ok {
		panic(fmt.Sprintf("catalog.MustGet: service %q not found", name))
	}
	return s
}

// Get returns a service by name.
func (c *Catalog) Get(name ServiceName) (ServiceBase, bool) {
	s, ok := c.services[name]
	return s, ok
}

func (c *Catalog) Status(name ServiceName) (unit.Status, error) {
	u, err := c.manager.Unit(unit.ID(name))
	if err != nil {
		return unit.StatusPending, xerrors.Errorf("get unit for %q: %w", name, err)
	}
	return u.Status(), nil
}

// UnmetDependencies returns the list of dependencies that are not yet satisfied
// for the given service.
func (c *Catalog) UnmetDependencies(name ServiceName) ([]string, error) {
	deps, err := c.manager.GetUnmetDependencies(unit.ID(name))
	if err != nil {
		return nil, xerrors.Errorf("get unmet dependencies for %q: %w", name, err)
	}

	result := make([]string, 0, len(deps))
	for _, dep := range deps {
		result = append(result, string(dep.DependsOn))
	}
	return result, nil
}

// Configure registers a typed callback to mutate a target service
// before startup. Panics if called after ApplyConfigurations.
func Configure[T ServiceBase](c *Catalog, target ServiceName, fn func(T)) {
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
			c.logger.Info(ctx, fmt.Sprintf("  %s %s -> [%s]", srv.Emoji(), srv.Name(), strings.Join(slice.ToStrings(deps), ", ")))
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
			c.notifySubscribers()

			svcLogger.Info(ctx, "starting service")
			if err := e.srv.Start(ctx, svcLogger, c); err != nil {
				return xerrors.Errorf("start %s: %w", name, err)
			}

			// Mark as complete after starting, which allows dependent services to start.
			if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusComplete); err != nil {
				return xerrors.Errorf("update status for %s: %w", name, err)
			}
			c.notifySubscribers()

			svcLogger.Info(ctx, "service started", slog.F("name", name))
			return nil
		})
	}

	// Start a goroutine that prints startup progress every 3 seconds.
	startTime := time.Now()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if c.allUnitsComplete() {
					return
				}
				c.unitsWaiting(ctx, startTime)
			}
		}
	}()

	err := wg.Wait()
	close(done)
	if err != nil {
		return xerrors.Errorf("start services: %w", err)
	}

	return nil
}

// allUnitsComplete returns true if all registered units have completed.
func (c *Catalog) allUnitsComplete() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for name := range c.services {
		u, err := c.manager.Unit(unit.ID(name))
		if err != nil {
			return false
		}
		if u.Status() != unit.StatusComplete {
			return false
		}
	}
	return true
}

// unitsWaiting logs the current state of all units, showing which dependencies
// are blocking each waiting unit. This helps debug startup DAG issues.
func (c *Catalog) unitsWaiting(ctx context.Context, startTime time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elapsed := time.Since(startTime).Truncate(time.Millisecond)

	var waiting, started, completed []string

	for name := range c.services {
		u, err := c.manager.Unit(unit.ID(name))
		if err != nil {
			c.logger.Warn(ctx, "failed to get unit", slog.F("name", name), slog.Error(err))
			continue
		}

		switch u.Status() {
		case unit.StatusPending:
			waiting = append(waiting, string(name))
		case unit.StatusStarted:
			started = append(started, string(name))
		case unit.StatusComplete:
			completed = append(completed, string(name))
		}
	}

	// Sort for deterministic output.
	slices.Sort(waiting)
	slices.Sort(started)
	slices.Sort(completed)

	c.logger.Info(ctx, "startup progress",
		slog.F("elapsed", elapsed.String()),
		slog.F("completed", len(completed)),
		slog.F("started", len(started)),
		slog.F("waiting", len(waiting)),
	)

	// Log details for each waiting unit.
	for _, name := range waiting {
		unmet, err := c.manager.GetUnmetDependencies(unit.ID(name))
		if err != nil {
			c.logger.Warn(ctx, "failed to get unmet dependencies",
				slog.F("name", name), slog.Error(err))
			continue
		}

		if len(unmet) == 0 {
			c.logger.Info(ctx, "unit waiting (ready to start)",
				slog.F("name", name))
			continue
		}

		// Build a summary of unmet dependencies.
		blockers := make([]string, 0, len(unmet))
		for _, dep := range unmet {
			blockers = append(blockers, fmt.Sprintf("%s(%s!=%s)",
				dep.DependsOn, dep.CurrentStatus, dep.RequiredStatus))
		}
		slices.Sort(blockers)
		c.logger.Info(ctx, "unit waiting on dependencies",
			slog.F("name", name),
			slog.F("blocked_by", strings.Join(blockers, ", ")),
		)
	}

	// Log started units (in progress).
	for _, name := range started {
		c.logger.Info(ctx, "unit in progress", slog.F("name", name))
	}
}

// Subscribe returns a channel that receives a notification whenever
// service state changes. The channel is buffered with size 1 so
// sends never block. Pass the returned channel to Unsubscribe when
// done.
func (c *Catalog) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	c.subscribersMu.Lock()
	c.subscribers[ch] = struct{}{}
	c.subscribersMu.Unlock()
	return ch
}

// Unsubscribe removes and closes a subscriber channel.
func (c *Catalog) Unsubscribe(ch chan struct{}) {
	c.subscribersMu.Lock()
	delete(c.subscribers, ch)
	c.subscribersMu.Unlock()
	close(ch)
}

// NotifySubscribers does a non-blocking send to every subscriber.
// It is exported so that API handlers can trigger notifications
// after operations like restart or stop.
func (c *Catalog) NotifySubscribers() {
	c.notifySubscribers()
}

// notifySubscribers does a non-blocking send to every subscriber.
func (c *Catalog) notifySubscribers() {
	c.subscribersMu.Lock()
	defer c.subscribersMu.Unlock()
	for ch := range c.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// waitForReady polls until the service's dependencies are satisfied.
// RestartService stops a service, resets its status, and starts it again,
// updating the unit.Manager status throughout the lifecycle.
func (c *Catalog) RestartService(ctx context.Context, name ServiceName, logger slog.Logger) error {
	svc, ok := c.services[name]
	if !ok {
		return xerrors.Errorf("service %q not found", name)
	}
	if err := svc.Stop(ctx); err != nil {
		return xerrors.Errorf("stop %s: %w", name, err)
	}
	// Reset status to pending, then follow the same lifecycle as Catalog.Start().
	if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusPending); err != nil {
		return xerrors.Errorf("reset status for %s: %w", name, err)
	}
	c.notifySubscribers()
	if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusStarted); err != nil {
		return xerrors.Errorf("update status for %s: %w", name, err)
	}
	c.notifySubscribers()
	if err := svc.Start(ctx, logger, c); err != nil {
		return xerrors.Errorf("start %s: %w", name, err)
	}
	if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusComplete); err != nil {
		return xerrors.Errorf("update status for %s: %w", name, err)
	}
	c.notifySubscribers()
	return nil
}

// StartService starts a previously stopped service, transitioning its
// unit.Manager status through pending → started → completed.
func (c *Catalog) StartService(ctx context.Context, name ServiceName, logger slog.Logger) error {
	svc, ok := c.services[name]
	if !ok {
		return xerrors.Errorf("service %q not found", name)
	}
	if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusStarted); err != nil {
		return xerrors.Errorf("update status for %s: %w", name, err)
	}
	c.notifySubscribers()
	if err := svc.Start(ctx, logger, c); err != nil {
		return xerrors.Errorf("start %s: %w", name, err)
	}
	if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusComplete); err != nil {
		return xerrors.Errorf("update status for %s: %w", name, err)
	}
	c.notifySubscribers()
	return nil
}

// StopService stops a service and resets its unit.Manager status to pending.
func (c *Catalog) StopService(ctx context.Context, name ServiceName) error {
	svc, ok := c.services[name]
	if !ok {
		return xerrors.Errorf("service %q not found", name)
	}
	if err := svc.Stop(ctx); err != nil {
		return xerrors.Errorf("stop %s: %w", name, err)
	}
	// Reset to pending since the service is no longer running.
	if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusPending); err != nil {
		return xerrors.Errorf("reset status for %s: %w", name, err)
	}
	c.notifySubscribers()
	return nil
}

func (c *Catalog) waitForReady(ctx context.Context, name ServiceName) error {
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
			return xerrors.Errorf("wait for service %s: %w", name, ctx.Err())
		default:
			time.Sleep(time.Millisecond * 15)
			continue
		}
	}
}
