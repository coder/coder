// Package catalog provides service definitions for cdev.
package catalog

import (
	"context"
	"fmt"
	"sync"

	"github.com/coder/coder/v2/agent/unit"
)

// Service is something that can be started and stopped as part of the dev environment.
type Service interface {
	// Name returns a unique identifier for this service.
	Name() string
	// DependsOn returns the names of services this service depends on.
	DependsOn() []string
	// Start launches the service. It should return once the service is ready.
	Start(ctx context.Context) error
	// Stop gracefully shuts down the service.
	Stop(ctx context.Context) error
}

// Configurer is implemented by services that need to configure other services
// before the run phase (e.g., setting environment variables on coderd).
type Configurer interface {
	Configure(c *Catalog) error
}

// FlagProvider is implemented by services that can be enabled via CLI flags.
type FlagProvider interface {
	// EnablementFlag returns the CLI flag that enables this service (e.g., "--oidc").
	EnablementFlag() string
}

// HealthChecker is implemented by services that support health checks.
type HealthChecker interface {
	// Healthy returns nil if the service is healthy, or an error describing the problem.
	Healthy(ctx context.Context) error
}

// EnvSetter is implemented by services that accept environment variables.
type EnvSetter interface {
	SetEnv(key, value string)
}

// Catalog manages the lifecycle of all services using the unit.Manager for dependencies.
type Catalog struct {
	mu       sync.RWMutex
	services map[string]Service
	running  map[string]bool
	order    []string // registration order for stop
	manager  *unit.Manager
}

// New creates a new empty catalog.
func New() *Catalog {
	return &Catalog{
		services: make(map[string]Service),
		running:  make(map[string]bool),
		manager:  unit.NewManager(),
	}
}

// Register adds a service to the catalog.
func (c *Catalog) Register(s Service) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	name := s.Name()
	if _, exists := c.services[name]; exists {
		return fmt.Errorf("service %q already registered", name)
	}

	// Register with unit manager.
	if err := c.manager.Register(unit.ID(name)); err != nil {
		return fmt.Errorf("register %s with manager: %w", name, err)
	}

	// Add dependencies.
	for _, dep := range s.DependsOn() {
		// Register dependency if not already registered (it may not exist yet).
		_ = c.manager.Register(unit.ID(dep))
		if err := c.manager.AddDependency(unit.ID(name), unit.ID(dep), unit.StatusComplete); err != nil {
			return fmt.Errorf("add dependency %s -> %s: %w", name, dep, err)
		}
	}

	c.services[name] = s
	c.order = append(c.order, name)
	return nil
}

// Get returns a service by name.
func (c *Catalog) Get(name string) (Service, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.services[name]
	return s, ok
}

// Start launches all registered services concurrently.
// Services block until their dependencies (tracked by unit.Manager) are ready.
func (c *Catalog) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Configure phase: let services set up cross-service configuration.
	for _, name := range c.order {
		svc := c.services[name]
		if cfg, ok := svc.(Configurer); ok {
			if err := cfg.Configure(c); err != nil {
				return fmt.Errorf("configure %s: %w", name, err)
			}
		}
	}

	// Start phase: launch all services concurrently.
	// Each service waits for its dependencies via the manager.
	var wg sync.WaitGroup
	errCh := make(chan error, len(c.order))

	for _, name := range c.order {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			// Wait for dependencies to be ready.
			if err := c.waitForReady(ctx, name); err != nil {
				errCh <- fmt.Errorf("waiting for %s dependencies: %w", name, err)
				return
			}

			// Mark as started.
			if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusStarted); err != nil {
				errCh <- fmt.Errorf("update status for %s: %w", name, err)
				return
			}

			// Start the service.
			svc := c.services[name]
			if err := svc.Start(ctx); err != nil {
				errCh <- fmt.Errorf("start %s: %w", name, err)
				return
			}

			// Mark as complete.
			if err := c.manager.UpdateStatus(unit.ID(name), unit.StatusComplete); err != nil {
				errCh <- fmt.Errorf("update status for %s: %w", name, err)
				return
			}

			c.mu.Lock()
			c.running[name] = true
			c.mu.Unlock()
		}(name)
	}

	// Wait for all goroutines to finish.
	go func() {
		wg.Wait()
		close(errCh)
	}()

	// Collect errors.
	for err := range errCh {
		if err != nil {
			return err
		}
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
			continue
		}
	}
}

// Stop shuts down all running services in reverse registration order.
func (c *Catalog) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var firstErr error
	// Stop in reverse order.
	for i := len(c.order) - 1; i >= 0; i-- {
		name := c.order[i]
		if !c.running[name] {
			continue
		}
		svc := c.services[name]
		if err := svc.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("stop %s: %w", name, err)
		}
		c.running[name] = false
	}
	return firstErr
}

// ExportDOT exports the dependency graph in DOT format for visualization.
func (c *Catalog) ExportDOT(name string) (string, error) {
	return c.manager.ExportDOT(name)
}


