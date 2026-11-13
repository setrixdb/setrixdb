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

// And grava em dst o AND palavra a palavra de a e b (versão escalar); devolve
// quantas palavras foram escritas (o menor dos três comprimentos).
func And(dst, a, b []uint64) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if len(dst) < n {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		dst[i] = a[i] & b[i]
	}
	return n
}

// Popcount devolve quantos bits estão setados (versão escalar).
func Popcount(words []uint64) int64 {
	var c int64
	for _, w := range words {
		c += int64(bits.OnesCount64(w))
	}
	return c
}
