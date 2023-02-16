package checks

import (
	"context"
	"sync"
	"time"

	"cdr.dev/slog"
)

// CheckFunc is a function that returns an error.
// It should return nil if the service is healthy.
// If the service is unhealthy, it should return an error.
// The error message will be visible to all users.
// This is not intended to be used for Kubernetes-style
// health checks. It is intended for user-facing health
// checks.
type CheckFunc func() error

// CheckResult is the result of a single check.
type CheckResult struct {
	Error      string    `json:"error"`
	CheckedAt  time.Time `json:"checked_at"`
	DurationMs int64     `json:"duration_ms"`
}

type Checker interface {
	Results() map[string]*CheckResult
	Add(name string, check CheckFunc)
	Stop()
}

type checker struct {
	sync.RWMutex
	tick    <-chan time.Time
	checks  map[string]CheckFunc
	results map[string]*CheckResult
	stop    chan struct{}
	log     slog.Logger
}

func New(tick <-chan time.Time, log slog.Logger) Checker {
	stop := make(chan struct{})
	c := &checker{
		tick:    tick,
		checks:  make(map[string]CheckFunc),
		results: make(map[string]*CheckResult),
		stop:    stop,
		log:     log,
	}
	go c.run()
	return c
}

func (c *checker) Results() map[string]*CheckResult {
	c.RLock()
	defer c.RUnlock()
	out := make(map[string]*CheckResult, len(c.results))
	for k := range c.results {
		if c.results[k] == nil {
			continue
		}
		res := *(c.results[k])
		out[k] = &res
	}
	return out
}

func (c *checker) Add(name string, check CheckFunc) {
	c.Lock()
	defer c.Unlock()
	c.checks[name] = check
	c.results[name] = nil
}

func (c *checker) Stop() {
	close(c.stop)
}

func (c *checker) run() {
	for {
		select {
		case <-c.stop:
			return
		case <-c.tick:
			for name := range c.checks {
				go c.runOneCheck(name)
			}
		}
	}
}

func (c *checker) runOneCheck(name string) {
	start := time.Now()
	var prevErr string
	c.RLock()
	checkFunc, ok := c.checks[name]
	if c.results[name] != nil {
		prevErr = c.results[name].Error
	}
	c.RUnlock()
	if !ok { // check was removed?
		return
	}
	err := checkFunc()
	changed := (prevErr == "" && err != nil) || (prevErr != "" && err == nil)
	if changed {
		fromStr := "ok"
		toStr := "ok"
		if err != nil {
			toStr = err.Error()
		}
		c.log.Warn(context.Background(), "check status changed",
			slog.F("from", fromStr),
			slog.F("to", toStr),
			slog.F("check", name),
		)
	}
	elapsed := time.Since(start)
	result := &CheckResult{
		CheckedAt:  start,
		DurationMs: elapsed.Milliseconds(),
	}
	if err != nil {
		result.Error = err.Error()
	}
	c.Lock()
	c.results[name] = result
	c.Unlock()
}
