//go:build !(cgo && amd64)

// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

// Package simd — fallback puro em Go (sem AVX-512).
package simd

import "math/bits"

// AndPopcount devolve popcount(a AND b) (versão escalar).
func AndPopcount(a, b []uint64) int64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var c int64
	for i := 0; i < n; i++ {
		c += int64(bits.OnesCount64(a[i] & b[i]))
	}
	return c
}

// Name identifica o kernel em uso.
func Name() string { return "generic" }
