package utils

import (
	"sync"

	"github.com/hashicorp/go-multierror"
)

// ConcurrentGroup is like errgroup.Group but differs in that an error in one
// goroutine will not interrupt the functioning of another.
// See https://pkg.go.dev/golang.org/x/sync/errgroup#Group.Go.
type ConcurrentGroup struct {
	wg sync.WaitGroup

	errsMu sync.Mutex
	errs   error
}

func NewConcurrentGroup() *ConcurrentGroup {
	return &ConcurrentGroup{}
}

func (c *ConcurrentGroup) Go(fn func() error) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := fn(); err != nil {
			c.errsMu.Lock()
			c.errs = multierror.Append(c.errs, err)
			c.errsMu.Unlock()
		}
	}()
}

func (c *ConcurrentGroup) Wait() error {
	c.wg.Wait()
	return c.errs
}
