//go:build race

package curly_test

// The race detector allocates on its own account, so counts taken under it
// say nothing about the parser.
const raceEnabled = true
