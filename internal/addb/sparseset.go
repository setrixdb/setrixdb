// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

package addb

import "sort"

// SparseSet é a representação de baixo consumo para ranges "frios": um slice
// ordenado de IDs (uint64). Membership O(log n) via busca binária; interseção
// O(n+m) via merge two-pointer. Complementa o Bitset denso — usado quando o
// universo é grande demais para o bitset (2³⁴+) ou a densidade é muito baixa.
type SparseSet struct {
	ids []uint64
}

// NewSparseSet cria a partir de uma lista (qualquer ordem; duplicatas removidas).
func NewSparseSet(ids []uint64) *SparseSet {
	s := &SparseSet{ids: append([]uint64(nil), ids...)}
	s.Finalize()
	return s
}

// Add insere um ID (sem ordenar; chame Finalize antes de consultar).
func (s *SparseSet) Add(id uint64) { s.ids = append(s.ids, id) }

// Finalize ordena e remove duplicatas.
func (s *SparseSet) Finalize() {
	sort.Slice(s.ids, func(i, j int) bool { return s.ids[i] < s.ids[j] })
	if len(s.ids) < 2 {
		return
	}
	w := 1
	for r := 1; r < len(s.ids); r++ {
		if s.ids[r] != s.ids[w-1] {
			s.ids[w] = s.ids[r]
			w++
		}
	}
	s.ids = s.ids[:w]
}

// Len devolve o número de elementos.
func (s *SparseSet) Len() int { return len(s.ids) }

// Has informa se o ID está no conjunto (busca binária).
func (s *SparseSet) Has(id uint64) bool {
	i := sort.Search(len(s.ids), func(i int) bool { return s.ids[i] >= id })
	return i < len(s.ids) && s.ids[i] == id
}

// AndCount devolve |s ∩ o| via merge two-pointer (ambos ordenados).
func (s *SparseSet) AndCount(o *SparseSet) int64 {
	var i, j int
	var c int64
	for i < len(s.ids) && j < len(o.ids) {
		a, b := s.ids[i], o.ids[j]
		switch {
		case a == b:
			c++
			i++
			j++
		case a < b:
			i++
		default:
			j++
		}
	}
	return c
}

// MemBytes devolve a memória aproximada (8 bytes por elemento).
func (s *SparseSet) MemBytes() int64 { return int64(len(s.ids) * 8) }
