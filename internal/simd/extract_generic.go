//go:build !(cgo && amd64)

// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

package simd

import "math/bits"

// ExtractSet (fallback puro em Go).
func ExtractSet(words []uint64) []uint64 {
	var out []uint64
	for i, w := range words {
		for w != 0 {
			p := bits.TrailingZeros64(w)
			out = append(out, uint64(i*64+p))
			w &= w - 1
		}
	}
	return out
}
