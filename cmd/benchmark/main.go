// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

// Command benchmark mede o desempenho do SetrixDB em ops/segundo.
//
// Gera um shard sintético, monta um batch de consultas e mede:
//   - buscas/s      (chamadas completas a ParallelSearchEngine)
//   - consultas/s   (IDs consultados por segundo)
//
// Uso:
//
//	go run ./cmd/benchmark
//	go run ./cmd/benchmark -shard 1000000 -batch 4096 -iters 2000
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	"github.com/setrixdb/setrixdb/internal/addb"
)

func main() {
	shardSize := flag.Int("shard", 1<<14, "número de IDs no shard (default 16.384)")
	batchSize := flag.Int("batch", 256, "número de consultas por busca")
	iters := flag.Int("iters", 2000, "número de buscas cronometradas")
	hitRate := flag.Float64("hit", 0.5, "fração de consultas que acerta (0..1)")
	seed := flag.Int64("seed", 42, "semente do gerador aleatório")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))

	// Shard: IDs aleatórios contíguos em RAM.
	shard := make([]uint64, *shardSize)
	for i := range shard {
		shard[i] = rng.Uint64()
	}

	// Batch: mistura de IDs que existem no shard (hits) e IDs ausentes (misses).
	nhits := int(float64(*batchSize) * *hitRate)
	batch := make([]uint64, 0, *batchSize)
	for i := 0; i < nhits; i++ {
		batch = append(batch, shard[rng.Intn(len(shard))])
	}
	for i := nhits; i < *batchSize; i++ {
		batch = append(batch, rng.Uint64()|1<<63) // provavelmente ausente
	}

	// Aquecimento (JIT/caches).
	_ = addb.ParallelSearchEngine(batch, shard)

	var lastMatches int
	start := time.Now()
	for i := 0; i < *iters; i++ {
		lastMatches = len(addb.ParallelSearchEngine(batch, shard))
	}
	elapsed := time.Since(start)

	secs := elapsed.Seconds()
	searchesPerSec := float64(*iters) / secs
	queriesPerSec := searchesPerSec * float64(*batchSize)

	fmt.Printf("SetrixDB — benchmark de busca paralela\n")
	fmt.Printf("  shard      : %d IDs (%.1f MiB)\n", *shardSize, float64(*shardSize*8)/(1024*1024))
	fmt.Printf("  batch      : %d consultas (hit rate %.0f%%)\n", *batchSize, *hitRate*100)
	fmt.Printf("  iterações  : %d\n", *iters)
	fmt.Printf("  tempo      : %s\n", elapsed.Round(time.Microsecond))
	fmt.Printf("  matches    : %d (última busca)\n", lastMatches)
	fmt.Printf("  ------------------------------------------\n")
	fmt.Printf("  %.2f buscas/s\n", searchesPerSec)
	fmt.Printf("  %.2f M consultas/s\n", queriesPerSec/1e6)
}
