// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

// Command hybridbench — demonstra a "parede de memória" do bitset denso e onde
// entra o híbrido denso (bitset AVX-512) + esparso (SparseSet/Roaring).
//
// Uso: go run ./cmd/hybridbench
package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/setrixdb/setrixdb/internal/addb"
	"github.com/setrixdb/setrixdb/internal/simd"
)

func main() {
	const keys = 4_000_000

	fmt.Printf("== SetrixDB híbrido — universo 2^36, %d chaves, kernel %s ==\n\n", keys, simd.Name())

	// ---- 1. Memória por representação (universo 2^36) ----
	fmt.Println("-- 1) Memória (universo 2^36 = 68,7 bi IDs) --")
	universe := uint64(1) << 36
	fmt.Printf("  bitset denso  (universo/8):  %.2f GB   <- NÃO cabe em RAM (3,8 GB)\n", float64(universe)/8/1e9)

	rng := rand.New(rand.NewSource(7))
	ids := make([]uint64, keys)
	for i := range ids {
		ids[i] = uint64(rng.Int63n(int64(universe)))
	}
	sp := addb.NewSparseSet(ids)
	fmt.Printf("  SparseSet     (chaves*8):    %.2f MB\n", float64(sp.MemBytes())/1e6)

	rb := roaring64.New()
	for _, id := range ids {
		rb.Add(id)
	}
	fmt.Printf("  Roaring64     (comprimido):  %.2f MB\n", float64(rb.GetSizeInBytes())/1e6)

	// ---- 2. Híbrido (95% hot em 2^20 + 5% cauda esparsa) ----
	fmt.Println("\n-- 2) Híbrido: 95% chaves numa faixa densa 2^20 + 5% cauda esparsa --")
	hotUniverse := uint64(1) << 20
	hot := addb.NewBitset(hotUniverse)
	hotKeys := keys * 95 / 100
	tail := keys - hotKeys
	rh := rand.New(rand.NewSource(11))
	for i := 0; i < hotKeys; i++ {
		hot.Set(uint64(rh.Int63n(int64(hotUniverse))))
	}
	tailSet := &addb.SparseSet{}
	rt := rand.New(rand.NewSource(13))
	for i := 0; i < tail; i++ {
		tailSet.Add(uint64(rt.Int63n(int64(universe))))
	}
	tailSet.Finalize()
	fmt.Printf("  hot  (bitset 2^20):  %.2f MB\n", float64(hot.Bytes())/1e6)
	fmt.Printf("  tail (SparseSet):    %.2f MB\n", float64(tailSet.MemBytes())/1e6)
	fmt.Printf("  TOTAL híbrido:       %.2f MB   (vs 8 GB denso-cheio)\n",
		float64(hot.Bytes()+int(tailSet.MemBytes()))/1e6)

	// ---- 3. Interseção: denso (AVX-512) vs esparso (merge) vs roaring ----
	fmt.Println("\n-- 3) Interseção A∩B (1M chaves cada) --")
	mkHot := func(seed int64) *addb.Bitset {
		b := addb.NewBitset(hotUniverse)
		r := rand.New(rand.NewSource(seed))
		for i := 0; i < 1_000_000; i++ {
			b.Set(uint64(r.Int63n(int64(hotUniverse))))
		}
		return b
	}
	A, B := mkHot(21), mkHot(22)
	t := time.Now()
	cDense := simd.AndPopcount(A.Words, B.Words)
	dDense := time.Since(t)

	mkSparse := func(seed int64) *addb.SparseSet {
		s := &addb.SparseSet{}
		r := rand.New(rand.NewSource(seed))
		for i := 0; i < 1_000_000; i++ {
			s.Add(uint64(r.Int63n(int64(universe))))
		}
		s.Finalize()
		return s
	}
	SA, SB := mkSparse(31), mkSparse(32)
	t = time.Now()
	cSparse := SA.AndCount(SB)
	dSparse := time.Since(t)

	ra, rbb := roaring64.New(), roaring64.New()
	for i := 0; i < 1_000_000; i++ {
		ra.Add(uint64(rh.Int63n(int64(universe))))
		rbb.Add(uint64(rh.Int63n(int64(universe))))
	}
	t = time.Now()
	cRoar := roaring64.And(ra, rbb).GetCardinality()
	dRoar := time.Since(t)

	fmt.Printf("  bitset AVX-512:  %8s   (%d)\n", dDense.Round(time.Microsecond), cDense)
	fmt.Printf("  SparseSet merge: %8s   (%d)\n", dSparse.Round(time.Microsecond), cSparse)
	fmt.Printf("  Roaring64:       %8s   (%d)\n", dRoar.Round(time.Microsecond), cRoar)
}
