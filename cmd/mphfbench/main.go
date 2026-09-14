// Command mphfbench constrói um MPHF (CHD) sobre N chaves e mede, com números:
//
//	- colisões (deve ser 0, por construção)
//	- bits/chave da estrutura
//	- tempo de build
//	- ns/op e ops/s de lookup (1 thread e paralelo)
//	- comparação com o kernel de SCAN atual (ADDB) e com map[uint64]uint32
//
// Uso:
//
//	go run ./cmd/mphfbench -n 1000000
//	go run ./cmd/mphfbench -n 50000000 -lambda 2.0 -epsilon 0.23
package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/tgosoul2019/addb/internal/addb"
	"github.com/tgosoul2019/addb/internal/mphf"
)

// makeKeys gera N chaves uint64 distintas (mix é bijeção ⇒ distintas).
func makeKeys(n int) []uint64 {
	keys := make([]uint64, n)
	for i := 0; i < n; i++ {
		keys[i] = mixKey(uint64(i))
	}
	return keys
}

func mixKey(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

func main() {
	n := flag.Int("n", 1_000_000, "número de chaves")
	lambda := flag.Float64("lambda", 2.0, "load factor dos buckets (chaves/bucket)")
	epsilon := flag.Float64("epsilon", 0.23, "folga da tabela (1+eps)")
	seed := flag.Uint64("seed", 0xADDB, "seed do hash")
	doMap := flag.Bool("map", true, "also bench Go map (pula se n grande)")
	mapLimit := flag.Int("maplimit", 8_000_000, "n máximo para rodar o bench de map")
	flag.Parse()

	keys := makeKeys(*n)
	fmt.Printf("== ADDB MPHF (CHD v2) — n=%d lambda=%.2f epsilon=%.2f ==\n", *n, *lambda, *epsilon)

	// --- build ---
	t0 := time.Now()
	h, err := mphf.BuildOpts(keys, *lambda, *epsilon, *seed)
	buildDur := time.Since(t0)
	if err != nil {
		fmt.Println("ERRO no build:", err)
		return
	}
	fmt.Printf("build: %s (%.0f chaves/s)\n", buildDur.Round(time.Millisecond), float64(*n)/buildDur.Seconds())

	// --- verificação: 0 colisão e cobertura completa ---
	seen := make([]uint64, (uint64(*n)+63)/64)
	collisions := 0
	misses := 0
	tv := time.Now()
	for _, k := range keys {
		id, ok := h.Lookup(k)
		if !ok {
			misses++
			continue
		}
		w := id >> 6
		bit := uint64(1) << (id & 63)
		if seen[w]&bit != 0 {
			collisions++
		}
		seen[w] |= bit
	}
	verDur := time.Since(tv)
	// chaves fora do conjunto devem retornar ok=false
	falsePos := 0
	for i := 0; i < 100000; i++ {
		if _, ok := h.Lookup(mixKey(uint64(1<<62) + uint64(i))); ok {
			falsePos++
		}
	}
	fmt.Printf("verificação: %s | colisões=%d  misses=%d  nao-membros-com-id-valido=%d/100k (esperado ~load factor; membership exige checar o termo)\n",
		verDur.Round(time.Millisecond), collisions, misses, falsePos)
	fmt.Printf("estrutura: %.2f bits/chave | %.1f MiB | max displacement=%d (%d bits/bucket)\n", h.BitsPerKey(), float64(h.Bytes())/(1024*1024), h.MaxD(), h.GWidth())

	// --- lookup single-thread ---
	t1 := time.Now()
	var chk uint64
	for _, k := range keys {
		id, _ := h.Lookup(k)
		chk += uint64(id)
	}
	d1 := time.Since(t1)
	nsOp := float64(d1.Nanoseconds()) / float64(*n)
	fmt.Printf("lookup 1 thread: %.2f ns/op | %.2f M ops/s (checksum=%d)\n",
		nsOp, float64(*n)/d1.Seconds()/1e6, chk)

	// --- lookup paralelo ---
	workers := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var totalOps int
	t2 := time.Now()
	for w := 0; w < workers; w++ {
		lo := w * *n / workers
		hi := (w + 1) * *n / workers
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			var local uint64
			for _, k := range keys[lo:hi] {
				id, _ := h.Lookup(k)
				local += uint64(id)
			}
			mu.Lock()
			totalOps += hi - lo
			mu.Unlock()
			_ = local
		}(lo, hi)
	}
	wg.Wait()
	d2 := time.Since(t2)
	fmt.Printf("lookup %d threads: %.2f M ops/s | %.2f ns/op agregado\n",
		workers, float64(totalOps)/d2.Seconds()/1e6, float64(d2.Nanoseconds())/float64(totalOps))

	// --- comparação: scan linear (kernel atual do ADDB) ---
	kq := 200
	if *n < 1000 {
		kq = 5
	}
	t3 := time.Now()
	for i := 0; i < kq; i++ {
		_ = addb.Contains(keys, keys[i])
	}
	d3 := time.Since(t3)
	scanPer := float64(d3.Nanoseconds()) / float64(kq)
	fmt.Printf("SCAN atual (addb.Contains): %.1f µs/consulta | %.0f consultas/s  ← O(n)\n",
		scanPer/1000, 1e9/scanPer)

	// --- comparação: map[uint64]uint32 ---
	if *doMap && *n <= *mapLimit {
		runtime.GC()
		var m0 runtime.MemStats
		runtime.ReadMemStats(&m0)
		m := make(map[uint64]uint32, *n)
		for i, k := range keys {
			m[k] = uint32(i)
		}
		runtime.GC()
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		mapBytes := int64(m1.HeapAlloc) - int64(m0.HeapAlloc)
		t4 := time.Now()
		var chk2 uint64
		for _, k := range keys {
			chk2 += uint64(m[k])
		}
		d4 := time.Since(t4)
		fmt.Printf("map[uint64]uint32: %.2f ns/op | %.2f bytes/chave (%.1f MiB) (checksum=%d)\n",
			float64(d4.Nanoseconds())/float64(*n), float64(mapBytes)/float64(*n),
			float64(mapBytes)/(1024*1024), chk2)
	}

	fmt.Printf("\nRESUMO n=%d: colisões=%d | %.2f bits/chave | build %.1fs | %.1f M lookups/s (1 thread)\n",
		*n, collisions, h.BitsPerKey(), buildDur.Seconds(), float64(*n)/d1.Seconds()/1e6)

	// --- varredura λ × ε (para achar o ponto ótimo de bits/chave) ---
	if *n <= 2_000_000 {
		fmt.Printf("\n== varredura λ × ε (n=%d) — bits/chave ==\n", *n)
		fmt.Print("        ")
		for _, e := range []float64{0.10, 0.23, 0.40, 0.60} {
			fmt.Printf("  ε=%.2f ", e)
		}
		fmt.Println()
		for _, l := range []float64{1.2, 1.6, 2.0, 2.5, 3.0} {
			fmt.Printf("  λ=%.1f  ", l)
			for _, e := range []float64{0.10, 0.23, 0.40, 0.60} {
				hh, err := mphf.BuildOpts(keys, l, e, 0xADDB)
				if err != nil {
					fmt.Printf("   FAIL  ")
					continue
				}
				fmt.Printf("  %5.2f ", hh.BitsPerKey())
			}
			fmt.Println()
		}
	}
}
