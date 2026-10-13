// Package lru implements a fixed-size least-recently-used cache.
package lru

import "container/list"

type entry struct {
	key   string
	value int
}

// Cache holds at most Capacity entries and evicts the least recently used
// one when a new key would exceed it. Both Get and Put count as a use.
type Cache struct {
	capacity int
	order    *list.List
	items    map[string]*list.Element
}

// New returns an empty cache. Capacity must be positive.
func New(capacity int) *Cache {
	return &Cache{capacity: capacity, order: list.New(), items: map[string]*list.Element{}}
}

// Get returns the value for key and whether it was present.
func (c *Cache) Get(key string) (int, bool) {
	el, ok := c.items[key]
	if !ok {
		return 0, false
	}
	return el.Value.(*entry).value, true
}

// Put stores value under key, evicting the least recently used entry when
// the cache is full.
func (c *Cache) Put(key string, value int) {
	if el, ok := c.items[key]; ok {
		el.Value.(*entry).value = value
		c.order.MoveToFront(el)
		return
	}
	if c.order.Len() >= c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*entry).key)
	}
	c.items[key] = c.order.PushFront(&entry{key: key, value: value})
}

// Len reports how many entries the cache holds.
func (c *Cache) Len() int {
	return c.order.Len()
}
