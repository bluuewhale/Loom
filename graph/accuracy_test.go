package graph

import (
	"testing"

	"community-detection/graph/testdata"
)

// TestLouvainKarateClubNMI verifies Louvain on Karate Club: Q > 0.35 and NMI >= 0.7. (TEST-01)
// Seed=1 gives 3 communities with Q=0.40 and NMI=0.83 against the 2-community ground truth.
func TestLouvainKarateClubNMI(t *testing.T) {
	g := buildGraph(testdata.KarateClubEdges)
	det := NewLouvain(LouvainOptions{Seed: 1})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0.35 {
		t.Errorf("Q = %.4f, want > 0.35", res.Modularity)
	}
	score := nmi(res.Partition, groundTruthPartition(testdata.KarateClubPartition))
	if score < 0.7 {
		t.Errorf("NMI = %.4f, want >= 0.7", score)
	}
	t.Logf("KarateClub Louvain: Q=%.4f communities=%d NMI=%.4f",
		res.Modularity, uniqueCommunities(res.Partition), score)
}

// TestLouvainFootballNMI verifies Louvain on the 115-node Football network:
// Q > 0 and NMI >= 0.5 vs 12-conference ground truth. (TEST-02)
func TestLouvainFootballNMI(t *testing.T) {
	g := buildGraph(testdata.FootballEdges)
	det := NewLouvain(LouvainOptions{Seed: 42})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0.0 {
		t.Errorf("Q = %.4f, want > 0.0", res.Modularity)
	}
	score := nmi(res.Partition, groundTruthPartition(testdata.FootballPartition))
	if score < 0.5 {
		t.Errorf("NMI = %.4f, want >= 0.5", score)
	}
	t.Logf("Football Louvain: Q=%.4f communities=%d NMI=%.4f",
		res.Modularity, uniqueCommunities(res.Partition), score)
}

// TestLeidenFootballNMI verifies Leiden on the 115-node Football network:
// Q > 0 and NMI >= 0.5 vs 12-conference ground truth. (TEST-02)
func TestLeidenFootballNMI(t *testing.T) {
	g := buildGraph(testdata.FootballEdges)
	det := NewLeiden(LeidenOptions{Seed: 2})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0.0 {
		t.Errorf("Q = %.4f, want > 0.0", res.Modularity)
	}
	score := nmi(res.Partition, groundTruthPartition(testdata.FootballPartition))
	if score < 0.5 {
		t.Errorf("NMI = %.4f, want >= 0.5", score)
	}
	t.Logf("Football Leiden: Q=%.4f communities=%d NMI=%.4f",
		res.Modularity, uniqueCommunities(res.Partition), score)
}

// TestLouvainPolbooksNMI verifies Louvain on the 105-node Polbooks network:
// Q > 0 and NMI >= 0.5 vs 3-community ground truth. (TEST-03)
func TestLouvainPolbooksNMI(t *testing.T) {
	g := buildGraph(testdata.PolbooksEdges)
	det := NewLouvain(LouvainOptions{Seed: 42})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0.0 {
		t.Errorf("Q = %.4f, want > 0.0", res.Modularity)
	}
	score := nmi(res.Partition, groundTruthPartition(testdata.PolbooksPartition))
	if score < 0.5 {
		t.Errorf("NMI = %.4f, want >= 0.5", score)
	}
	t.Logf("Polbooks Louvain: Q=%.4f communities=%d NMI=%.4f",
		res.Modularity, uniqueCommunities(res.Partition), score)
}

// TestLeidenPolbooksNMI verifies Leiden on the 105-node Polbooks network:
// Q > 0 and NMI >= 0.5 vs 3-community ground truth. (TEST-03)
func TestLeidenPolbooksNMI(t *testing.T) {
	g := buildGraph(testdata.PolbooksEdges)
	det := NewLeiden(LeidenOptions{Seed: 2})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0.0 {
		t.Errorf("Q = %.4f, want > 0.0", res.Modularity)
	}
	score := nmi(res.Partition, groundTruthPartition(testdata.PolbooksPartition))
	if score < 0.5 {
		t.Errorf("NMI = %.4f, want >= 0.5", score)
	}
	t.Logf("Polbooks Leiden: Q=%.4f communities=%d NMI=%.4f",
		res.Modularity, uniqueCommunities(res.Partition), score)
}
