// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import "github.com/setrixdb/setrixdb/internal/simd"

// Bitset representa um conjunto de IDs densos em [0, maxID) — a forma natural
// depois que o MPHF transforma termos em IDs em [0, N). Interseção = AND bit a bit.
type Bitset struct {
	Words []uint64
}

// NewBitset cria um bitset capaz de representar IDs em [0, maxID].
func NewBitset(maxID uint64) *Bitset {
	return &Bitset{Words: make([]uint64, maxID/64+1)}
}

// Set liga o bit do ID.
func (b *Bitset) Set(id uint64) { b.Words[id>>6] |= 1 << (id & 63) }

// Test informa se o ID está no conjunto.
func (b *Bitset) Test(id uint64) bool { return b.Words[id>>6]&(1<<(id&63)) != 0 }

// AndPopcount devolve |A ∩ B| (popcount do AND). Usa o kernel vetorizado
// (AVX-512 quando a CPU tem, escalar caso contrário) — este é o caminho quente
// da contagem de interseção (ShardedBitset.IntersectCount).
func (b *Bitset) AndPopcount(o *Bitset) int64 {
	return simd.AndPopcount(b.Words, o.Words)
}

// Intersect devolve um novo bitset com A ∩ B (AND palavra a palavra pelo
// kernel). O comprimento é o menor dos dois: palavra ausente é conjunto vazio.
func (b *Bitset) Intersect(o *Bitset) *Bitset {
	n := len(b.Words)
	if len(o.Words) < n {
		n = len(o.Words)
	}
	out := &Bitset{Words: make([]uint64, n)}
	simd.And(out.Words, b.Words, o.Words)
	return out
}

// Extract devolve as posições (IDs) dos bits setados, em ordem crescente, pelo
// kernel de extração vetorizada.
func (b *Bitset) Extract() []uint64 { return simd.ExtractSet(b.Words) }

// Bytes devolve o tamanho em bytes.
func (b *Bitset) Bytes() int { return len(b.Words) * 8 }
