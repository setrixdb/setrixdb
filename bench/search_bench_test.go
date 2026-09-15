// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"math/rand"
	"testing"

	"github.com/setrixdb/setrixdb/internal/addb"
)

// makeWorkload gera um shard e um batch sintéticos para os benchmarks.
func makeWorkload(shardSize, batchSize int) (shard, batch []uint64) {
	rng := rand.New(rand.NewSource(1))
	shard = make([]uint64, shardSize)
	for i := range shard {
		shard[i] = rng.Uint64()
	}
	batch = make([]uint64, batchSize)
	for i := range batch {
		if i%2 == 0 {
			batch[i] = shard[rng.Intn(len(shard))] // hit
		} else {
			batch[i] = rng.Uint64() | 1<<63 // miss
		}
	}
	return shard, batch
}

// BenchmarkParallelSearchEngine mede buscas/s e reporta consultas/s.
func BenchmarkParallelSearchEngine(b *testing.B) {
	shard, batch := makeWorkload(1<<14, 512)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = addb.ParallelSearchEngine(batch, shard)
	}
	b.StopTimer()
	secs := b.Elapsed().Seconds()
	b.ReportMetric(float64(b.N)/secs, "buscas/s")
	b.ReportMetric(float64(b.N)*float64(len(batch))/secs, "consultas/s")
}

// BenchmarkContains mede o kernel aritmético puro (uma varredura).
func BenchmarkContains(b *testing.B) {
	shard, _ := makeWorkload(1<<14, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = addb.Contains(shard, shard[i%len(shard)])
	}
}

// BenchmarkComputeDeterministicID mede a transmutação termo -> uint64 (termos/s).
func BenchmarkComputeDeterministicID(b *testing.B) {
	terms := []string{"casa", "moradia", "lar", "computação", "distribuído", "sinônimo"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = addb.ComputeDeterministicID(terms[i%len(terms)])
	}
	b.StopTimer()
	secs := b.Elapsed().Seconds()
	b.ReportMetric(float64(b.N)/secs, "termos/s")
}
