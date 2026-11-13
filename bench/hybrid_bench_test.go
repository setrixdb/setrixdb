// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package bench

// Híbrido por shard: em dados esparsos o bitset denso varre o universo inteiro
// enquanto o SparseSet paga só pelos IDs existentes. Estes benchmarks usam o
// MESMO dado nos dois caminhos (o modo é decidido por memória, em Finalize).

import (
	"math/bits"
	"testing"

	"github.com/setrixdb/setrixdb/internal/addb"
)

func sparseSharded(universe uint64, shards int, perShard int, seed uint64) *addb.ShardedBitset {
	sb := addb.NewShardedBitset(universe, shards)
	shardBits := sb.ShardBits
	span := shardBits / uint64(perShard+1)
	if span == 0 {
		span = 1
	}
	x := seed | 1
	for s := 0; s < shards; s++ {
		for i := 0; i < perShard; i++ {
			x = x*6364136223846793005 + 1442695040888963407 // LCG: determinístico
			off := (x >> 16) % span
			sb.Set(uint64(s)*shardBits + uint64(i)*span + off)
		}
	}
	sb.Finalize()
	return sb
}

func BenchmarkShardedSparseHibrido(b *testing.B) {
	a := sparseSharded(1<<20, 8, 100, 1)
	c := sparseSharded(1<<20, 8, 100, 2)
	if m := a.Modes(); m[0] != "sparse" {
		b.Fatalf("esperava modo esparso, veio %v", m)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.IntersectCount(c)
	}
}

// Referência do comportamento ANTERIOR (dense-only): mesmo universo, mesmos
// IDs, contagem varrendo as palavras dos 8 shards.
func BenchmarkShardedSparseDensoAntigo(b *testing.B) {
	a := sparseSharded(1<<20, 8, 100, 1)
	c := sparseSharded(1<<20, 8, 100, 2)

	// reconstrói os mesmos conjuntos SEM Finalize (modo denso forçado)
	da := shardedDenseOnly(1<<20, 8, a)
	dc := shardedDenseOnly(1<<20, 8, c)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var total int64
		for sh := 0; sh < 8; sh++ {
			wa, wc := da.Words(sh), dc.Words(sh)
			n := len(wa)
			if len(wc) < n {
				n = len(wc)
			}
			for j := 0; j < n; j++ {
				total += int64(bits.OnesCount64(wa[j] & wc[j]))
			}
		}
		_ = total
	}
}

// shardedDenseOnly copia os IDs de um ShardedBitset híbrido para um denso.
func shardedDenseOnly(universe uint64, shards int, src *addb.ShardedBitset) *addb.ShardedBitset {
	dst := addb.NewShardedBitset(universe, shards)
	for id := uint64(0); id < universe; id++ {
		if src.Has(id) {
			dst.Set(id)
		}
	}
	return dst // sem Finalize: todas as palavras densas
}
