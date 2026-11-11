package lru

import "testing"

func TestHitAndMiss(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)

	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %d, %v; want 1, true", v, ok)
	}
	if v, ok := c.Get("b"); ok || v != 0 {
		t.Fatalf("Get(b) = %d, %v; want 0, false", v, ok)
	}
}

func TestEvictsLeastRecent(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // evicts a

	if _, ok := c.Get("a"); ok {
		t.Fatal("a should have been evicted")
	}
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Fatalf("Get(b) = %d, %v; want 2, true", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Fatalf("Get(c) = %d, %v; want 3, true", v, ok)
	}
	if c.Len() != 2 {
		t.Fatalf("Len() = %d; want 2", c.Len())
	}
}

func TestGetRefreshesRecency(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a")    // a is now most recent, b is least recent
	c.Put("c", 3) // evicts b, not a

	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted after a was read")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %d, %v; want 1, true", v, ok)
	}
}

func TestMaxOne(t *testing.T) {
	c := New[string, int](1)
	c.Put("a", 1)
	c.Put("b", 2) // evicts a

	if c.Len() != 1 {
		t.Fatalf("Len() = %d; want 1", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("a should have been evicted")
	}
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Fatalf("Get(b) = %d, %v; want 2, true", v, ok)
	}
}

func TestOverwriteUpdatesValueAndRecency(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("a", 10) // overwrite; a is now most recent, no growth
	c.Put("c", 3)  // evicts b

	if c.Len() != 2 {
		t.Fatalf("Len() = %d; want 2", c.Len())
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	if v, ok := c.Get("a"); !ok || v != 10 {
		t.Fatalf("Get(a) = %d, %v; want 10, true", v, ok)
	}
}
