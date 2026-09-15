// Command setbench compara as estratégias de CONSULTA do ADDB sobre o mesmo shard:
//
//  1. scan      — kernel atual (varredura O(n) por consulta)
//  2. sorted    — shard ordenado + busca binária O(log n)
//  3. mphf       — MPHF (CHD) + 1 comparação (O(1))  ← resolve o "Killer 1" (colisão)
//
// Mede ops/s de consulta e o custo de memória/preparação de cada uma.
//
// Uso: go run ./cmd/setbench -n 1000000 -batch 512
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/setrixdb/setrixdb/internal/addb"
	"github.com/setrixdb/setrixdb/internal/mphf"
)

func makeShard(n int, seed int64) []uint64 {
	rng := rand.New(rand.NewSource(seed))
	s := make([]uint64, n)
	for i := range s {
		s[i] = rng.Uint64()
	}
	return s
}

func makeBatch(shard []uint64, size int, hit float64, seed int64) []uint64 {
	rng := rand.New(rand.NewSource(seed))
	b := make([]uint64, size)
	nh := int(float64(size) * hit)
	for i := range b {
		if i < nh {
			b[i] = shard[rng.Intn(len(shard))]
		} else {
			b[i] = rng.Uint64() | (1 << 63)
		}
	}
	return b
}

func main() {
	n := flag.Int("n", 1_000_000, "tamanho do shard")
	batch := flag.Int("batch", 512, "consultas por busca")
	reps := flag.Int("reps", 200, "repetições")
	flag.Parse()

	shard := makeShard(*n, 42)
	b := makeBatch(shard, *batch, 0.5, 7)

	fmt.Printf("== ADDB setbench — n=%d batch=%d ==\n", *n, *batch)

	// 1) SCAN (atual)
	t := time.Now()
	for i := 0; i < *reps; i++ {
		_ = addb.ParallelSearchEngine(b, shard)
	}
	d := time.Since(t)
	scanOps := float64(*reps*len(b)) / d.Seconds()
	fmt.Printf("scan (O(n))     : %.0f consultas/s | %.1f µs/busca\n", scanOps, d.Seconds()*1e6/float64(*reps))

	// 2) SORTED (ordena 1x + busca binária)
	sorted := append([]uint64(nil), shard...)
	ts := time.Now()
	addb.SortShard(sorted)
	sortTime := time.Since(ts)
	_ = addb.SearchSortedBatch(sorted, b) // warmup
	t = time.Now()
	for i := 0; i < *reps; i++ {
		_ = addb.SearchSortedBatch(sorted, b)
	}
	d = time.Since(t)
	sortedOps := float64(*reps*len(b)) / d.Seconds()
	fmt.Printf("sorted (O(log n)): %.0f consultas/s | %.1f µs/busca | sort 1x = %s\n",
		sortedOps, d.Seconds()*1e6/float64(*reps), sortTime.Round(time.Millisecond))

	// 3) MPHF (CHD): id em O(1) + 1 comparação de verificação
	t = time.Now()
	h, err := mphf.Build(shard, 0.95, 0xADDB)
	buildTime := time.Since(t)
	if err != nil {
		fmt.Println("MPHF erro:", err)
		return
	}
	// tabela order[id] = chave (para verificar membership com 1 comparação)
	order := make([]uint64, *n)
	for _, k := range shard {
		id, _ := h.Lookup(k)
		order[id] = k
	}
	lookupBatch := func() int {
		hit := 0
		for _, q := range b {
			id, _ := h.Lookup(q)
			if order[id] == q {
				hit++
			}
		}
		return hit
	}
	_ = lookupBatch()
	t = time.Now()
	var hits int
	for i := 0; i < *reps; i++ {
		hits = lookupBatch()
	}
	d = time.Now().Sub(t)
	mphfOps := float64(*reps*len(b)) / d.Seconds()
	fmt.Printf("mphf (O(1))     : %.0f consultas/s | %.1f µs/busca | build 1x = %s | bits/chave %.2f\n",
		mphfOps, d.Seconds()*1e6/float64(*reps), buildTime.Round(time.Millisecond), h.BitsPerKey())
	_ = hits
	_ = sort.Sort
	fmt.Printf("\nGanho vs scan: sorted %.0fx · mphf %.0fx\n", sortedOps/scanOps, mphfOps/scanOps)
}
