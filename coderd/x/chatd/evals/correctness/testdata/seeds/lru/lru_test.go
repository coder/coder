package lru

import "testing"

func TestPutGet(t *testing.T) {
	c := New(2)
	c.Put("a", 1)
	c.Put("b", 2)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %d, %v", v, ok)
	}
	c.Put("c", 3)
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
}
