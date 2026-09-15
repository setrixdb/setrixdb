//go:build cgo && amd64

// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

package simd

/*
#cgo CFLAGS: -O3 -mavx512f
#include <stdint.h>
size_t addb_extract_set(const uint64_t* words, size_t n, uint64_t* out);
*/
import "C"

import "unsafe"

// ExtractSet devolve, em ordem crescente, as posições (IDs) dos bits setados,
// usando AVX-512 (pula palavras zeradas rápido).
func ExtractSet(words []uint64) []uint64 {
	if len(words) == 0 {
		return nil
	}
	out := make([]uint64, len(words)*64)
	n := int(C.addb_extract_set(
		(*C.uint64_t)(unsafe.Pointer(&words[0])),
		C.size_t(len(words)),
		(*C.uint64_t)(unsafe.Pointer(&out[0]))))
	return out[:n]
}
