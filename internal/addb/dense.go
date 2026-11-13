// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import "github.com/setrixdb/setrixdb/internal/simd"

// IntersectIDs devolve, em ordem crescente, os IDs comuns a TODOS os conjuntos,
// usando representação densa (bitset) e os kernels vetorizados.
//
// É o mesmo resultado de IntersectMany (merge em listas ordenadas), por outro
// caminho: quando os conjuntos são densos no universo, o AND palavra a palavra
// custa `universe/64` operações por conjunto — menos que varrer as listas.
// Quem decide o caminho é a camada pública (setrixdb.Intersect); esta função é a
// metade densa.
//
// `universe` é o maior ID + 1 (os conjuntos precisam estar ordenados).
func IntersectIDs(sets [][]uint64, universe uint64) []uint64 {
	if len(sets) == 0 || universe == 0 {
		return nil
	}
	words := int(universe/64) + 1
	acc := make([]uint64, words)
	buf := make([]uint64, words)

	for i, ids := range sets {
		for j := range buf {
			buf[j] = 0
		}
		for _, id := range ids {
			if w := int(id >> 6); w < words {
				buf[w] |= 1 << (id & 63)
			}
		}
		if i == 0 {
			copy(acc, buf)
			continue
		}
		simd.And(acc, acc, buf)
	}
	return simd.ExtractSet(acc)
}
