// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import "github.com/setrixdb/setrixdb/internal/simd"

// ExtractSet devolve, em ordem crescente, todas as posições de bit setadas (1)
// do bitset `words`.
//
// Usa o kernel de extração vetorizada (AVX-512 com dispatch em runtime, escalar
// no fallback) — antes era um laço por bit em Go, exercitado só por teste. O
// caminho denso da interseção termina aqui, então a extração também passou a
// rodar no produto.
func ExtractSet(words []uint64) []uint64 {
	return simd.ExtractSet(words)
}
