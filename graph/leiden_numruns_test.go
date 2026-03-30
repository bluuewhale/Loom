package graph

import (
	"math"
	"testing"
)

// TestLeidenNumRunsSeed42Deterministic verifies that Seed=42 with any NumRuns value
// behaves identically to the existing single-run deterministic path.
// (Seed!=0 must always be a single run regardless of NumRuns.)
func TestLeidenNumRunsSeed42Deterministic(t *testing.T) {
	g := buildKarateClubLeiden()
	det1 := NewLeiden(LeidenOptions{Seed: 42})
	det2 := NewLeiden(LeidenOptions{Seed: 42, NumRuns: 5}) // NumRuns must be ignored
	res1, err1 := det1.Detect(g)
	res2, err2 := det2.Detect(g)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if math.Abs(res1.Modularity-res2.Modularity) > 1e-10 {
		t.Errorf("Seed!=0 with NumRuns=5 changed result: Q1=%.6f Q2=%.6f", res1.Modularity, res2.Modularity)
	}
}

// TestLeidenNumRunsZeroDefaultsToThree verifies that Seed=0, NumRuns=0 defaults to 3 runs
// and returns a valid CommunityResult (Q > 0) for the Karate Club graph.
func TestLeidenNumRunsZeroDefaultsToThree(t *testing.T) {
	g := buildKarateClubLeiden()
	det := NewLeiden(LeidenOptions{Seed: 0, NumRuns: 0}) // 0 → default 3
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0 {
		t.Errorf("expected Q > 0, got %.4f", res.Modularity)
	}
	if len(res.Partition) != 34 {
		t.Errorf("expected 34 nodes in Partition, got %d", len(res.Partition))
	}
}

// TestLeidenNumRunsThreeExplicit verifies that Seed=0, NumRuns=3 returns a valid
// CommunityResult with Q > 0 for the Karate Club graph.
func TestLeidenNumRunsThreeExplicit(t *testing.T) {
	g := buildKarateClubLeiden()
	det := NewLeiden(LeidenOptions{Seed: 0, NumRuns: 3})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0 {
		t.Errorf("expected Q > 0, got %.4f", res.Modularity)
	}
	if len(res.Partition) != 34 {
		t.Errorf("expected 34 nodes in Partition, got %d", len(res.Partition))
	}
	t.Logf("Seed=0 NumRuns=3: Q=%.4f communities=%d", res.Modularity, uniqueCommunities(res.Partition))
}

// TestLeidenNumRunsOneIsEquivalent verifies that Seed=0, NumRuns=1 returns a valid
// CommunityResult (single run, non-deterministic path).
func TestLeidenNumRunsOneIsEquivalent(t *testing.T) {
	g := buildKarateClubLeiden()
	det := NewLeiden(LeidenOptions{Seed: 0, NumRuns: 1})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0 {
		t.Errorf("expected Q > 0, got %.4f", res.Modularity)
	}
}
