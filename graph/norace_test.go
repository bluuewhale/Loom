//go:build !race

package graph

// raceEnabled is false when the race detector is not active.
// Performance tests use this to skip timing assertions that are
// invalidated by the ~3x overhead the race detector adds to
// goroutine synchronization.
const raceEnabled = false
