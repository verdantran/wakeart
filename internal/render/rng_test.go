package render

import "math/rand"

func testRNG() *rand.Rand { return rand.New(rand.NewSource(1)) }
