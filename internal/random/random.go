// Package random is the only place in this repository that creates a source of
// randomness. Everything else takes one and is handed one.
//
// The reason is the same as the clock's, on issue #27: a test that cannot say
// what the next number will be is a test that fails once a week and is rerun
// until it passes. A seeded source makes a failing run reproducible from the
// seed printed with it.
//
// This is not the package for anything a secret depends on. A key, a token or a
// room credential comes from crypto/rand at the place that needs it, and issue
// #33 is where that is decided. The guard that names the exceptions is in the
// guard package.
package random

import "math/rand/v2"

// Seeded returns a source that produces the same numbers for the same seed. A
// test names its seed, and a failure is reproduced by naming it again.
func Seeded(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed)) }

// System returns a source seeded by the runtime. A service uses this; a test
// does not, because a test that uses it cannot be reproduced.
func System() *rand.Rand { return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) }
