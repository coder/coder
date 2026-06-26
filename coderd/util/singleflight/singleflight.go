package singleflight

import (
	"sync"
)

type call struct {
	wg  sync.WaitGroup
	val any
	err error
}

// Group protects concurrent calls.
type Group struct {
	mu     sync.Mutex       // protects m
	m      map[string]*call // lazily initialized
	notify chan string
}

func NewGroup(notifier chan string) *Group {
	group := new(Group)
	group.notify = notifier
	return group
}

// Do ensures there is only one call to fn in flight at a time.  Any calls that
// come in while it is in flight wait for the original call and get the same
// results.
func (g *Group) Do(key string, fn func() (any, error)) (v any, err error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		if g.notify != nil {
			g.notify <- key
		}
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := new(call)
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	g.mu.Lock()
	defer g.mu.Unlock()
	c.wg.Done()
	delete(g.m, key)
	return c.val, c.err
}
