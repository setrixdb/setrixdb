//go:build cgo && amd64

// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

// Package simd expõe kernels vetorizados para o SetrixDB.
//
// O kernel de interseção de bitsets é HÍBRIDO: o arquivo C é compilado portável
// (sem exigir AVX-512) e escolhe EM RUNTIME entre a versão AVX-512
// (vpandq + vpopcntq) e o fallback escalar — via __builtin_cpu_supports.
// CPUs AVX10.2 (que incluem 512-bit + VPOPCNTDQ) usam o mesmo caminho AVX-512.
package simd

/*
#cgo CFLAGS: -O3
#include <stdint.h>
long long addb_and_popcount(const uint64_t* a, const uint64_t* b, long long n);
int addb_and_popcount_tier(void);
void addb_and(const uint64_t* a, const uint64_t* b, uint64_t* dst, long long n);
long long addb_popcount(const uint64_t* a, long long n);
*/
import "C"

import "unsafe"

// AndPopcount devolve popcount(a AND b) sobre n palavras de 64 bits.
func AndPopcount(a, b []uint64) int64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	return int64(C.addb_and_popcount(
		(*C.uint64_t)(unsafe.Pointer(&a[0])),
		(*C.uint64_t)(unsafe.Pointer(&b[0])),
		C.longlong(n)))
}

// Name identifica o kernel efetivamente usado em runtime.
func Name() string {
	if int(C.addb_and_popcount_tier()) == 1 {
		return "avx512 (hibrido)"
	}
	return "scalar (hibrido)"
}

// And grava em dst o AND palavra a palavra de a e b; devolve quantas palavras
// foram escritas (o menor dos três comprimentos — palavra ausente é zero).
func And(dst, a, b []uint64) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if len(dst) < n {
		n = len(dst)
	}
	if n == 0 {
		return 0
	}
	C.addb_and(
		(*C.uint64_t)(unsafe.Pointer(&a[0])),
		(*C.uint64_t)(unsafe.Pointer(&b[0])),
		(*C.uint64_t)(unsafe.Pointer(&dst[0])),
		C.longlong(n))
	return n
}

// Popcount devolve quantos bits estão setados — pelo kernel vetorial.
func Popcount(words []uint64) int64 {
	if len(words) == 0 {
		return 0
	}
	return int64(C.addb_popcount((*C.uint64_t)(unsafe.Pointer(&words[0])), C.longlong(len(words))))
}
