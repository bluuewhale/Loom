//go:build !race

package graph

// raceEnabled is false when the test binary is compiled without -race.
const raceEnabled = false
