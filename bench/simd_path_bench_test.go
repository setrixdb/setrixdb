// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package bench

// Comparação dos dois caminhos de interseção (v1.1): merge em listas
// ordenadas x caminho denso com kernel vetorizado. O kernel escolhe AVX-512 em
// runtime; o nome do tier aparece no log do teste.

import (
	"math/bits"
	"math/rand"
	"testing"

	"github.com/setrixdb/setrixdb"
	"github.com/setrixdb/setrixdb/internal/addb"
	"github.com/setrixdb/setrixdb/internal/simd"
)

func denseDatasets(universe uint64, fill, k int) [][]uint64 {
	rng := rand.New(rand.NewSource(42))
	out := make([][]uint64, k)
	for i := range out {
		m := map[uint64]bool{}
		for len(m) < fill {
			m[uint64(rng.Int63n(int64(universe)))] = true
		}
		ids := make([]uint64, 0, len(m))
		for id := range m {
			ids = append(ids, id)
		}
		addb.SortShard(ids)
		out[i] = ids
	}
	return out
}

func TestKernelTier(t *testing.T) {
	t.Logf("kernel efetivo: %s", simd.Name())
}

func BenchmarkIntersectMergeDenso(b *testing.B) {
	raw := denseDatasets(1<<20, 200_000, 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = addb.IntersectMany(raw...)
	}
}

func BenchmarkIntersectDenso(b *testing.B) {
	raw := denseDatasets(1<<20, 200_000, 2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = addb.IntersectIDs(raw, 1<<20)
	}
}

func BenchmarkIntersectPublicoDenso(b *testing.B) {
	raw := denseDatasets(1<<20, 200_000, 2)
	sets := make([]*setrixdb.Set, len(raw))
	for i, ids := range raw {
		sets[i] = setrixdb.NewSet(ids...)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = setrixdb.Intersect(sets...)
	}
}

// Dados esparsos: a régua tem de escolher o merge, sem regressão nenhuma.
func BenchmarkIntersectPublicoEsparso(b *testing.B) {
	raw := denseDatasets(1<<24, 2_000, 2)
	sets := make([]*setrixdb.Set, len(raw))
	for i, ids := range raw {
		sets[i] = setrixdb.NewSet(ids...)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = setrixdb.Intersect(sets...)
	}
}

// Referência do código antigo (laço escalar em Go) x kernel.
func BenchmarkAndPopcountEscalarRef(b *testing.B) {
	a := &addb.Bitset{Words: make([]uint64, 16384)}
	c := &addb.Bitset{Words: make([]uint64, 16384)}
	rng := rand.New(rand.NewSource(1))
	for i := range a.Words {
		a.Words[i] = rng.Uint64()
		c.Words[i] = rng.Uint64()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var s int64
		for j := range a.Words {
			s += int64(bits.OnesCount64(a.Words[j] & c.Words[j]))
		}
		_ = s
	}
}

func BenchmarkAndPopcountKernel(b *testing.B) {
	a := &addb.Bitset{Words: make([]uint64, 16384)}
	c := &addb.Bitset{Words: make([]uint64, 16384)}
	rng := rand.New(rand.NewSource(1))
	for i := range a.Words {
		a.Words[i] = rng.Uint64()
		c.Words[i] = rng.Uint64()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.AndPopcount(c)
	}
}

// Comportamento ANTERIOR da API pública em dados densos: copiar os IDs
// (Set.IDs) e fazer merge ordenado. É o baseline do antes/depois da v1.1.
func BenchmarkIntersectPublicoAntigo(b *testing.B) {
	sets := make([]*setrixdb.Set, 2)
	for i, ids := range denseDatasets(1<<20, 200_000, 2) {
		sets[i] = setrixdb.NewSet(ids...)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw := make([][]uint64, len(sets))
		for j, s := range sets {
			raw[j] = s.IDs()
		}
		_ = addb.IntersectMany(raw...)
	}
}
