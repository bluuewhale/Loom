package graph

import (
	"errors"
	"testing"
)

// Compile-time interface satisfaction check.
var _ OverlappingCommunityDetector = (*egoSplittingDetector)(nil)

// Test 1: NewEgoSplitting returns a non-nil value satisfying OverlappingCommunityDetector.
func TestNewEgoSplitting_ReturnsOverlappingCommunityDetector(t *testing.T) {
	d := NewEgoSplitting(EgoSplittingOptions{})
	if d == nil {
		t.Fatal("NewEgoSplitting returned nil")
	}
}

// Test 2: NewEgoSplitting defaults nil LocalDetector and GlobalDetector to Louvain.
func TestNewEgoSplitting_DefaultsNilDetectors(t *testing.T) {
	d := NewEgoSplitting(EgoSplittingOptions{})
	impl, ok := d.(*egoSplittingDetector)
	if !ok {
		t.Fatal("NewEgoSplitting did not return *egoSplittingDetector")
	}
	if impl.opts.LocalDetector == nil {
		t.Error("LocalDetector is nil after NewEgoSplitting with zero options")
	}
	if impl.opts.GlobalDetector == nil {
		t.Error("GlobalDetector is nil after NewEgoSplitting with zero options")
	}
	if impl.opts.Resolution != 1.0 {
		t.Errorf("Resolution = %v, want 1.0", impl.opts.Resolution)
	}
}

// Test 3: NewEgoSplitting preserves caller-supplied detectors.
func TestNewEgoSplitting_PreservesSuppliedDetectors(t *testing.T) {
	local := NewLeiden(LeidenOptions{})
	global := NewLeiden(LeidenOptions{})
	d := NewEgoSplitting(EgoSplittingOptions{
		LocalDetector:  local,
		GlobalDetector: global,
		Resolution:     2.0,
	})
	impl := d.(*egoSplittingDetector)
	if impl.opts.LocalDetector != local {
		t.Error("LocalDetector was overwritten")
	}
	if impl.opts.GlobalDetector != global {
		t.Error("GlobalDetector was overwritten")
	}
	if impl.opts.Resolution != 2.0 {
		t.Errorf("Resolution = %v, want 2.0", impl.opts.Resolution)
	}
}

// Test 4: Detect stub returns ErrNotImplemented.
func TestEgoSplittingDetector_Detect_ReturnsErrNotImplemented(t *testing.T) {
	d := NewEgoSplitting(EgoSplittingOptions{})
	g := NewGraph(false)
	_, err := d.Detect(g)
	if err == nil {
		t.Fatal("expected ErrNotImplemented, got nil")
	}
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got: %v", err)
	}
}

// Test 5: OverlappingCommunityResult zero-value has expected field defaults.
func TestOverlappingCommunityResult_ZeroValue(t *testing.T) {
	r := OverlappingCommunityResult{}
	if r.Communities != nil {
		t.Errorf("expected Communities nil, got %v", r.Communities)
	}
	if r.NodeCommunities != nil {
		t.Errorf("expected NodeCommunities nil, got %v", r.NodeCommunities)
	}
}
