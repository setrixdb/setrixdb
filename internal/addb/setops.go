// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import "sort"

// SortShard ordena o shard in-place (uint64). Habilita busca binária (O(log n))
// e interseção por merge (O(n+m)) — a "camada de consulta" que substitui a
// varredura linear O(n) do kernel antigo.
func SortShard(shard []uint64) {
	sort.Slice(shard, func(i, j int) bool { return shard[i] < shard[j] })
}

// ContainsSorted faz busca binária em um shard ORDENADO. O(log n).
func ContainsSorted(sorted []uint64, q uint64) bool {
	i := sort.Search(len(sorted), func(i int) bool { return sorted[i] >= q })
	return i < len(sorted) && sorted[i] == q
}

// SearchSortedBatch devolve os IDs do batch presentes no shard ordenado.
// Ordem preservada, sem duplicatas.
func SearchSortedBatch(sorted, batch []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(batch))
	out := make([]uint64, 0, len(batch))
	for _, q := range batch {
		if _, dup := seen[q]; dup {
			continue
		}
		seen[q] = struct{}{}
		if ContainsSorted(sorted, q) {
			out = append(out, q)
		}
	}
	return out
}

// IntersectSorted devolve a interseção de dois arrays ORDENADOS (merge linear,
// amigável a SIMD: comparações previsíveis, sem branch no dado).
func IntersectSorted(a, b []uint64) []uint64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	out := make([]uint64, 0, n)
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	return out
}

// IntersectMany intersecta N conjuntos ordenados (folding pairwise).
func IntersectMany(sets ...[]uint64) []uint64 {
	if len(sets) == 0 {
		return nil
	}
	acc := sets[0]
	for _, s := range sets[1:] {
		acc = IntersectSorted(acc, s)
		if len(acc) == 0 {
			return nil
		}
	}
	return acc
}
