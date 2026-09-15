// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

// Command sparsebench — testa o cenário ESPARSO (IDs raros em universo grande),
// onde o bitset denso teoricamente perde pro Roaring (que comprime).
//
// Uso: go run ./cmd/sparsebench
package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/setrixdb/setrixdb/internal/addb"
	"github.com/setrixdb/setrixdb/internal/simd"
)

func unique(a []uint64) []uint64 {
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
	universe := uint64(1) << 26 // 67M
	n := 1_000_000
	rng := rand.New(rand.NewSource(1))
	A := make([]uint64, n)
	B := make([]uint64, n)
	for i := range A {
		A[i] = uint64(rng.Int63n(int64(universe)))
	}
	for i := 0; i < n/2; i++ {
		B[i] = A[rng.Intn(n)]
	}
	for i := n / 2; i < n; i++ {
		B[i] = uint64(rng.Int63n(int64(universe)))
	}

	// bitset denso
	bsA := addb.NewBitset(universe)
	bsB := addb.NewBitset(universe)
	for _, k := range A {
		bsA.Set(k)
	}
	for _, k := range B {
		bsB.Set(k)
	}
	t := time.Now()
	cGo := bsA.AndPopcount(bsB)
	dGo := time.Since(t)
	t = time.Now()
	cSimd := simd.AndPopcount(bsA.Words, bsB.Words)
	dSimd := time.Since(t)

	// Roaring
	ra := roaring64.New()
	rb := roaring64.New()
	for _, k := range A {
		ra.Add(k)
	}
	for _, k := range B {
		rb.Add(k)
	}
	t = time.Now()
	rc := roaring64.And(ra, rb)
	dR := time.Since(t)

	// merge
	As := append([]uint64(nil), A...)
	Bs := append([]uint64(nil), B...)
	addb.SortShard(As)
	addb.SortShard(Bs)
	As = unique(As)
	Bs = unique(Bs)
	t = time.Now()
	res := addb.IntersectSorted(As, Bs)
	dM := time.Since(t)

	fmt.Printf("== ESPARSO: universo 2^26 (67M), A=B=1M chaves ==\n")
	fmt.Printf("  bitset AND (Go)      %10s  mem %d KB   result=%d\n", dGo.Round(time.Microsecond), bsA.Bytes()/1024, cGo)
	fmt.Printf("  bitset AND (AVX-512) %10s              result=%d\n", dSimd.Round(time.Microsecond), cSimd)
	fmt.Printf("  Roaring64            %10s  mem %d B    result=%d\n", dR.Round(time.Microsecond), ra.GetSizeInBytes(), rc.GetCardinality())
	fmt.Printf("  sorted merge         %10s              result=%d\n", dM.Round(time.Microsecond), len(res))
}
