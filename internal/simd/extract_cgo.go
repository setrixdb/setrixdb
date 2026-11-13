//go:build cgo && amd64

// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package simd

/*
#cgo CFLAGS: -O3
#include <stdint.h>
size_t addb_extract_set(const uint64_t* words, size_t n, uint64_t* out);
*/
import "C"

import "unsafe"

// ExtractSet devolve, em ordem crescente, as posições (IDs) dos bits setados.
// Portável: usa AVX-512 quando disponível (dispatch em runtime), senão escalar.
//
// A saída é dimensionada pelo popcount real: alocar 64 posições por palavra
// (o máximo teórico) custava mais que a própria extração e fazia o caminho
// denso perder para o merge — medido na v1.1.
func ExtractSet(words []uint64) []uint64 {
	if len(words) == 0 {
		return nil
	}
	n := Popcount(words)
	if n == 0 {
		return nil
	}
	out := make([]uint64, n)
	got := int(C.addb_extract_set(
		(*C.uint64_t)(unsafe.Pointer(&words[0])),
		C.size_t(len(words)),
		(*C.uint64_t)(unsafe.Pointer(&out[0]))))
	return out[:got]
}
