// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import (
	"math/bits"
	"math/rand"
	"testing"
)

// O caminho denso tem de devolver EXATAMENTE o mesmo que o merge ordenado.
func TestIntersectIDsMatchesMerge(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for _, tc := range []struct {
		name     string
		universe uint64
		sets     int
		fill     int // IDs por conjunto
	}{
		{"2 conjuntos densos", 4096, 2, 3000},
		{"3 conjuntos densos", 8192, 3, 6000},
		{"esparsos no universo", 1 << 20, 2, 100},
		{"universo de 1 palavra", 64, 2, 40},
		{"universo de 0", 1, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := make([][]uint64, tc.sets)
			for i := range raw {
				m := map[uint64]bool{}
				for len(m) < tc.fill && uint64(len(m)) < tc.universe {
					m[uint64(rng.Int63n(int64(tc.universe)))] = true
				}
				ids := make([]uint64, 0, len(m))
				for id := range m {
					ids = append(ids, id)
				}
				SortShard(ids)
				raw[i] = ids
			}
			want := IntersectMany(raw...)
			got := IntersectIDs(raw, tc.universe)
			if len(got) != len(want) {
				t.Fatalf("tamanho: denso=%d merge=%d", len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("posição %d: denso=%d merge=%d", i, got[i], want[i])
				}
			}
		})
	}
}

// Conjunto vazio no meio zera o resultado nos dois caminhos.
func TestIntersectIDsEmptySet(t *testing.T) {
	raw := [][]uint64{{1, 2, 3}, {}, {2, 3, 4}}
	if got := IntersectIDs(raw, 5); len(got) != 0 {
		t.Fatalf("esperava vazio, veio %v", got)
	}
	if got := IntersectMany(raw...); len(got) != 0 {
		t.Fatalf("merge: esperava vazio, veio %v", got)
	}
}

// Bitset.Intersect/Extract pelo kernel x implementação ingênua.
func TestBitsetIntersectParity(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for _, n := range []int{0, 1, 2, 7, 8, 9, 33} {
		a := NewBitset(uint64(n)*64 + 1)
		b := NewBitset(uint64(n)*64 + 1)
		for i := 0; i < n*8; i++ {
			if rng.Intn(2) == 0 {
				a.Set(uint64(rng.Intn(n * 64)))
			}
			if rng.Intn(2) == 0 {
				b.Set(uint64(rng.Intn(n * 64)))
			}
		}
		inter := a.Intersect(b)
		// referência: AND escalar palavra a palavra
		want := make([]uint64, len(a.Words))
		for i := range a.Words {
			want[i] = a.Words[i] & b.Words[i]
		}
		for i := range want {
			if inter.Words[i] != want[i] {
				t.Fatalf("n=%d palavra %d: kernel=%x esperado=%x", n, i, inter.Words[i], want[i])
			}
		}
		// extração vetorizada x contagem escalar
		ids := inter.Extract()
		var ones int
		for _, w := range want {
			ones += bits.OnesCount64(w)
		}
		if len(ids) != ones {
			t.Fatalf("n=%d: extract=%d popcount=%d", n, len(ids), ones)
		}
		for i := 1; i < len(ids); i++ {
			if ids[i] <= ids[i-1] {
				t.Fatalf("extração fora de ordem em %d: %v", i, ids[i-1:i+1])
			}
		}
	}
}

// AndPopcount (kernel) x laço escalar — a conta que o produto usa.
func TestAndPopcountParity(t *testing.T) {
	rng := rand.New(rand.NewSource(23))
	for _, n := range []int{0, 1, 5, 8, 9, 64, 129} {
		a := &Bitset{Words: make([]uint64, n)}
		b := &Bitset{Words: make([]uint64, n)}
		for i := 0; i < n; i++ {
			a.Words[i] = rng.Uint64()
			b.Words[i] = rng.Uint64()
		}
		var want int64
		for i := 0; i < n; i++ {
			want += int64(bits.OnesCount64(a.Words[i] & b.Words[i]))
		}
		if got := a.AndPopcount(b); got != want {
			t.Fatalf("n=%d: kernel=%d escalar=%d", n, got, want)
		}
	}
	// comprimentos diferentes: vale o menor (palavra ausente é vazio)
	a := &Bitset{Words: []uint64{0xFF, 0xFF}}
	b := &Bitset{Words: []uint64{0x0F}}
	if got := a.AndPopcount(b); got != 4 {
		t.Fatalf("comprimento diferente: %d, esperava 4", got)
	}
}
