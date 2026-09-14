//go:build cgo && amd64

// Package simd expõe kernels vetorizados para o ADDB. Nesta build, o kernel de
// interseção de bitsets usa AVX-512 (vpandq + vpopcntq).
package simd

/*
#cgo CFLAGS: -O3 -mavx512f -mavx512vpopcntdq
#include <stdint.h>
long long addb_and_popcount(const uint64_t* a, const uint64_t* b, long long n);
*/
import "C"

import "unsafe"

// AndPopcount devolve popcount(a AND b) sobre n palavras de 64 bits (AVX-512).
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

// Name identifica o kernel em uso.
func Name() string { return "avx512" }
