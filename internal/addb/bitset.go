// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

package addb

import "math/bits"

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

// AndPopcount devolve |A ∩ B| (popcount do AND), versão escalar em Go.
func (b *Bitset) AndPopcount(o *Bitset) int64 {
	n := len(b.Words)
	if len(o.Words) < n {
		n = len(o.Words)
	}
	var c int64
	for i := 0; i < n; i++ {
		c += int64(bits.OnesCount64(b.Words[i] & o.Words[i]))
	}
	return c
}

// Bytes devolve o tamanho em bytes.
func (b *Bitset) Bytes() int { return len(b.Words) * 8 }
