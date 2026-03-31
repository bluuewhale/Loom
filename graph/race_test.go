//go:build race

package graph

// raceEnabled is true when the race detector is active.
// Performance tests use this to skip timing assertions that are
// invalidated by the ~3x overhead the race detector adds to
// goroutine synchronization.
const raceEnabled = true
