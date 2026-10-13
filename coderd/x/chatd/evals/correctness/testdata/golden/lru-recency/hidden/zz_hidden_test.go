package lru

import "testing"

func TestHiddenGetCountsAsUse(t *testing.T) {
	c := New(2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a")
	c.Put("c", 3)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a was evicted although it was used more recently than b")
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted")
	}
}

func TestHiddenUpdateKeepsSize(t *testing.T) {
	c := New(1)
	c.Put("a", 1)
	c.Put("a", 2)
	if v, _ := c.Get("a"); v != 2 || c.Len() != 1 {
		t.Fatalf("Get(a) = %d, Len = %d", v, c.Len())
	}
}
