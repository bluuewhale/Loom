package graph

import (
	"testing"
)

// TestRegistryEmpty verifies that a new registry is empty and returns false for unknown lookups.
func TestRegistryEmpty(t *testing.T) {
	reg := NewRegistry()
	if reg.Len() != 0 {
		t.Fatalf("expected Len()==0, got %d", reg.Len())
	}
	if id, ok := reg.ID("x"); ok || id != 0 {
		t.Fatalf("expected ID('x')==(0,false), got (%d,%v)", id, ok)
	}
	if name, ok := reg.Name(0); ok || name != "" {
		t.Fatalf("expected Name(0)==('',false), got (%q,%v)", name, ok)
	}
}

// TestRegistryRegisterNew verifies that two distinct names get sequential IDs starting from 0.
func TestRegistryRegisterNew(t *testing.T) {
	reg := NewRegistry()
	id0 := reg.Register("alice")
	id1 := reg.Register("bob")
	if id0 != 0 {
		t.Fatalf("expected Register('alice')==0, got %d", id0)
	}
	if id1 != 1 {
		t.Fatalf("expected Register('bob')==1, got %d", id1)
	}
	if reg.Len() != 2 {
		t.Fatalf("expected Len()==2, got %d", reg.Len())
	}
}

// TestRegistryRegisterDuplicate verifies that registering the same name twice returns the same NodeID.
func TestRegistryRegisterDuplicate(t *testing.T) {
	reg := NewRegistry()
	id1 := reg.Register("alice")
	id2 := reg.Register("alice")
	if id1 != id2 {
		t.Fatalf("expected same NodeID for duplicate Register, got %d and %d", id1, id2)
	}
	if reg.Len() != 1 {
		t.Fatalf("expected Len()==1 after duplicate register, got %d", reg.Len())
	}
}

// TestRegistryIDLookup verifies that ID returns the correct value and existence flag.
func TestRegistryIDLookup(t *testing.T) {
	reg := NewRegistry()
	reg.Register("alice")
	if id, ok := reg.ID("alice"); !ok || id != 0 {
		t.Fatalf("expected ID('alice')==(0,true), got (%d,%v)", id, ok)
	}
	if id, ok := reg.ID("unknown"); ok || id != 0 {
		t.Fatalf("expected ID('unknown')==(0,false), got (%d,%v)", id, ok)
	}
}

// TestRegistryNameLookup verifies that Name returns the correct name and bounds-checks IDs.
func TestRegistryNameLookup(t *testing.T) {
	reg := NewRegistry()
	reg.Register("alice")
	if name, ok := reg.Name(0); !ok || name != "alice" {
		t.Fatalf("expected Name(0)==('alice',true), got (%q,%v)", name, ok)
	}
	if name, ok := reg.Name(1); ok || name != "" {
		t.Fatalf("expected Name(1)==('',false) for out-of-range, got (%q,%v)", name, ok)
	}
	if name, ok := reg.Name(-1); ok || name != "" {
		t.Fatalf("expected Name(-1)==('',false) for negative, got (%q,%v)", name, ok)
	}
}

// TestRegistryLen verifies that Len returns the number of distinct registered names.
func TestRegistryLen(t *testing.T) {
	reg := NewRegistry()
	reg.Register("a")
	reg.Register("b")
	reg.Register("c")
	if reg.Len() != 3 {
		t.Fatalf("expected Len()==3, got %d", reg.Len())
	}
}

// TestRegistryRoundTrip verifies that registered names integrate correctly with a Graph.
func TestRegistryRoundTrip(t *testing.T) {
	reg := NewRegistry()
	aliceID := reg.Register("alice")
	bobID := reg.Register("bob")
	_ = reg.Register("carol")

	g := NewGraph(false)
	g.AddEdge(aliceID, bobID, 1.0)

	lookedUpAlice, ok := reg.ID("alice")
	if !ok {
		t.Fatal("expected ID('alice') to be found")
	}
	lookedUpBob, ok := reg.ID("bob")
	if !ok {
		t.Fatal("expected ID('bob') to be found")
	}

	neighbors := g.Neighbors(lookedUpAlice)
	if len(neighbors) == 0 {
		t.Fatal("expected alice to have neighbors")
	}
	found := false
	for _, e := range neighbors {
		if e.To == lookedUpBob {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected edge from alice(%d) to bob(%d) in Neighbors", lookedUpAlice, lookedUpBob)
	}
}

// TestRegistryIdempotentMultiple verifies that registering 5 names each twice results in Len()==5.
func TestRegistryIdempotentMultiple(t *testing.T) {
	reg := NewRegistry()
	names := []string{"a", "b", "c", "d", "e"}
	ids := make(map[string]NodeID)

	// First registration pass
	for _, name := range names {
		ids[name] = reg.Register(name)
	}

	// Second registration pass — must return same IDs
	for _, name := range names {
		id := reg.Register(name)
		if id != ids[name] {
			t.Fatalf("Register(%q) second call returned %d, expected %d", name, id, ids[name])
		}
	}

	if reg.Len() != 5 {
		t.Fatalf("expected Len()==5 after idempotent registrations, got %d", reg.Len())
	}

	// Verify all ID lookups are correct
	for _, name := range names {
		id, ok := reg.ID(name)
		if !ok {
			t.Fatalf("expected ID(%q) to be found", name)
		}
		if id != ids[name] {
			t.Fatalf("ID(%q) returned %d, expected %d", name, id, ids[name])
		}
	}
}
