package graph

import (
	"strings"
	"testing"
)

// Compile-time interface satisfaction checks.
var _ CommunityDetector = (*louvainDetector)(nil)
var _ CommunityDetector = (*leidenDetector)(nil)

// Test 1: NewLouvain returns a non-nil value satisfying CommunityDetector.
func TestNewLouvain_ReturnsCommunityDetector(t *testing.T) {
	d := NewLouvain(LouvainOptions{})
	if d == nil {
		t.Fatal("NewLouvain returned nil")
	}
}

// Test 2: NewLeiden returns a non-nil value satisfying CommunityDetector.
func TestNewLeiden_ReturnsCommunityDetector(t *testing.T) {
	d := NewLeiden(LeidenOptions{})
	if d == nil {
		t.Fatal("NewLeiden returned nil")
	}
}

// Test 3: NewLeiden.Detect returns nil error on a valid undirected graph.
func TestNewLeiden_DetectReturnsError(t *testing.T) {
	d := NewLeiden(LeidenOptions{Seed: 1, NumRuns: 1})
	g := NewGraph(false)
	// Empty graph — no error expected (returns empty CommunityResult).
	_, err := d.Detect(g)
	if err != nil {
		t.Fatalf("expected nil error from leidenDetector.Detect on empty graph, got: %v", err)
	}
}

// Test 4: ErrDirectedNotSupported is non-nil and contains "directed".
func TestErrDirectedNotSupported(t *testing.T) {
	if ErrDirectedNotSupported == nil {
		t.Fatal("ErrDirectedNotSupported is nil")
	}
	if !strings.Contains(ErrDirectedNotSupported.Error(), "directed") {
		t.Fatalf("ErrDirectedNotSupported message %q does not contain 'directed'", ErrDirectedNotSupported.Error())
	}
}

// Test 5: CommunityResult{} zero-value has expected field defaults.
func TestCommunityResult_ZeroValue(t *testing.T) {
	r := CommunityResult{}
	if r.Partition != nil {
		t.Errorf("expected Partition nil, got %v", r.Partition)
	}
	if r.Modularity != 0.0 {
		t.Errorf("expected Modularity 0.0, got %v", r.Modularity)
	}
	if r.Passes != 0 {
		t.Errorf("expected Passes 0, got %v", r.Passes)
	}
	if r.Moves != 0 {
		t.Errorf("expected Moves 0, got %v", r.Moves)
	}
}
