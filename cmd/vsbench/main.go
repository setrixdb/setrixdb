// Command vsbench — head-to-head do ADDB contra estruturas de mercado.
//
// MEMBERSHIP sobre N chaves uint64:
//   - Go map[uint64]struct{}      (hash table)
//   - MPHF (CHD) + 1 verificação  (ADDB)  [exato]
//   - Bloom filter (1% FP)        [aproximado]
//
// INTERSEÇÃO de dois conjuntos (A,B), em 2 cenários:
//   - denso32 : IDs 0..2N (como no ADDB real: IDs do MPHF, densos/32-bit)
//   - aleat64 : uint64 aleatórios (pior caso p/ bitmaps)
//   estratégias: sorted merge (ADDB) · Roaring · hash join (map)
//
// Uso: go run ./cmd/vsbench -n 1000000
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"github.com/RoaringBitmap/roaring"
	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/bits-and-blooms/bloom/v3"
	"github.com/tgosoul2019/addb/internal/addb"
	"github.com/tgosoul2019/addb/internal/mphf"
	"github.com/tgosoul2019/addb/internal/simd"
)

func mix(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

func heap() uint64 { runtime.GC(); var m runtime.MemStats; runtime.ReadMemStats(&m); return m.HeapAlloc }
func d64(a, b uint64) int64 { return int64(b) - int64(a) }

func uniqueU64(a []uint64) []uint64 {
	if len(a) == 0 {
		return a
	}
	j := 1
	for i := 1; i < len(a); i++ {
		if a[i] != a[j-1] {
			a[j] = a[i]
			j++
		}
	}
	return a[:j]
}

func main() {
	n := flag.Int("n", 1_000_000, "nº de chaves")
	reps := flag.Int("reps", 300, "repetições de lookup")
	flag.Parse()

	keys := make([]uint64, *n)
	for i := range keys {
		keys[i] = mix(uint64(i))
	}
	batch := make([]uint64, 1000)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 500; i++ {
		batch[i] = keys[rng.Intn(*n)]
	}
	for i := 500; i < 1000; i++ {
		batch[i] = mix(uint64(1<<62) + uint64(i))
	}
	bb := make([][]byte, len(batch))
	for i, k := range batch {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, k)
		bb[i] = b
	}

	fmt.Printf("== ADDB vs mercado — n=%d ==\n\n", *n)
	fmt.Printf("%-24s %12s %12s %12s\n", "MEMBERSHIP", "build", "memória", "ops/s")
	fmt.Println("------------------------------------------------------------------")

	// Go map
	a := heap()
	mp := make(map[uint64]struct{}, *n)
	for _, k := range keys {
		mp[k] = struct{}{}
	}
	mapMem := d64(a, heap())
	t := time.Now()
	for i := 0; i < *reps; i++ {
		for _, q := range batch {
			_, _ = mp[q]
		}
	}
	mapOps := float64(*reps*len(batch)) / time.Since(t).Seconds()
	fmt.Printf("%-24s %12s %12.1f B/k %12.0f\n", "Go map[uint64]", "-", float64(mapMem)/float64(*n), mapOps)

	// MPHF
	t0 := time.Now()
	h, _ := mphf.Build(keys, 0.95, 0xADDB)
	order := make([]uint64, *n)
	for _, k := range keys {
		id, _ := h.Lookup(k)
		order[id] = k
	}
	build := time.Since(t0)
	t = time.Now()
	for i := 0; i < *reps; i++ {
		for _, q := range batch {
			id, _ := h.Lookup(q)
			_ = order[id] == q
		}
	}
	mphfOps := float64(*reps*len(batch)) / time.Since(t).Seconds()
	fmt.Printf("%-24s %12s %12.1f B/k %12.0f\n", "MPHF (CHD) [exato]", build.Round(time.Millisecond), float64(h.Bytes()+uint64(*n)*8)/float64(*n), mphfOps)

	// Bloom
	a = heap()
	bl := bloom.NewWithEstimates(uint(*n), 0.01)
	for _, k := range keys {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, k)
		bl.Add(b)
	}
	_ = a
	bloomMem := int64(bl.Cap() / 8) // bits do filtro / 8
	_ = heap
	t = time.Now()
	for i := 0; i < *reps; i++ {
		for _, b := range bb {
			_ = bl.Test(b)
		}
	}
	bloomOps := float64(*reps*len(batch)) / time.Since(t).Seconds()
	fmt.Printf("%-24s %12s %12.1f B/k %12.0f\n", "Bloom (1% FP) [aprox]", "-", float64(bloomMem)/float64(*n), bloomOps)

	fmt.Println()
	fmt.Printf("%-24s %12s %12s %10s\n", "INTERSEÇÃO", "tempo", "memória", "resultado")
	fmt.Println("------------------------------------------------------------------")
	runIntersect("denso32 (realista)", *n, rng, true)
	runIntersect("aleat64 (pior caso)", *n, rng, false)
}

func runIntersect(label string, n int, rng *rand.Rand, dense bool) {
	// conjuntos A e B
	A := make([]uint64, n)
	B := make([]uint64, n)
	if dense {
		for i := range A {
			A[i] = uint64(rng.Intn(2 * n))
		}
		for i := range B {
			B[i] = uint64(rng.Intn(2 * n))
		}
	} else {
		for i := range A {
			A[i] = mix(uint64(i))
		}
		for i := 0; i < n/2; i++ {
			B[i] = A[rng.Intn(n)]
		}
		for i := n / 2; i < n; i++ {
			B[i] = mix(uint64(1<<61) + uint64(i))
		}
	}

	As := append([]uint64(nil), A...)
	Bs := append([]uint64(nil), B...)
	addb.SortShard(As)
	addb.SortShard(Bs)
	As = uniqueU64(As)
	Bs = uniqueU64(Bs)
	t := time.Now()
	res := addb.IntersectSorted(As, Bs)
	dS := time.Since(t)

	var dR time.Duration
	var rcard uint64
	if dense {
		ra, rb := roaring.New(), roaring.New()
		for _, k := range A {
			ra.Add(uint32(k))
		}
		for _, k := range B {
			rb.Add(uint32(k))
		}
		t = time.Now()
		rc := roaring.And(ra, rb)
		dR = time.Since(t)
		rcard = rc.GetCardinality()
	} else {
		ra, rb := roaring64.New(), roaring64.New()
		for _, k := range A {
			ra.Add(k)
		}
		for _, k := range B {
			rb.Add(k)
		}
		t = time.Now()
		rc := roaring64.And(ra, rb)
		dR = time.Since(t)
		rcard = rc.GetCardinality()
	}

	hm := make(map[uint64]struct{}, n)
	for _, k := range As { // dedup A
		hm[k] = struct{}{}
	}
	t = time.Now()
	seen := make(map[uint64]struct{}, 1024)
	for _, k := range Bs { // dedup B no resultado
		if _, ok := hm[k]; ok {
			seen[k] = struct{}{}
		}
	}
	dH := time.Since(t)

	fmt.Printf("\n[%s]  A=B=%d chaves\n", label, n)
	fmt.Printf("  %-22s %12s %12s %10d\n", "sorted merge (ADDB)", dS.Round(time.Microsecond), "-", len(res))
	fmt.Printf("  %-22s %12s %12s %10d\n", "Roaring", dR.Round(time.Microsecond), "-", rcard)
	fmt.Printf("  %-22s %12s %12s %10d\n", "hash join (map)", dH.Round(time.Microsecond), "-", len(seen))

	if dense {
		bsA := addb.NewBitset(uint64(2 * n))
		bsB := addb.NewBitset(uint64(2 * n))
		for _, k := range A {
			bsA.Set(k)
		}
		for _, k := range B {
			bsB.Set(k)
		}
		t = time.Now()
		cGo := bsA.AndPopcount(bsB)
		dGo := time.Since(t)
		t = time.Now()
		cSimd := simd.AndPopcount(bsA.Words, bsB.Words)
		dSimd := time.Since(t)
		fmt.Printf("  %-22s %12s %12s %10d\n", "bitset AND (Go)", dGo.Round(time.Microsecond), fmt.Sprintf("%d KB", bsA.Bytes()/1024), cGo)
		fmt.Printf("  %-22s %12s %12s %10d  <== %s\n", "bitset AND (AVX-512)", dSimd.Round(time.Microsecond), "", cSimd, simd.Name())
	}
}
