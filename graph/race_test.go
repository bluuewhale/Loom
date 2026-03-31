//go:build race

package graph

// raceEnabled is true when the test binary is compiled with -race.
// Used to skip timing-sensitive performance tests that are invalidated
// by the race detector's ~3x overhead on goroutine synchronization.
const raceEnabled = true
