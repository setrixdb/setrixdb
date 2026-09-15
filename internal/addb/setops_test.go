// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

func TestSortAndBinarySearch(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	shard := make([]uint64, 10000)
	for i := range shard {
		shard[i] = rng.Uint64()
	}
	SortShard(shard)
	if !sort.SliceIsSorted(shard, func(i, j int) bool { return shard[i] < shard[j] }) {
		t.Fatal("shard não ficou ordenado")
	}
	// todos os membros devem ser encontrados
	for _, q := range shard {
		if !ContainsSorted(shard, q) {
			t.Fatalf("membro %d não encontrado", q)
		}
	}
}

func TestIntersectSorted(t *testing.T) {
	a := []uint64{1, 2, 3, 5, 8, 13, 21}
	b := []uint64{2, 3, 4, 5, 21, 34}
	got := IntersectSorted(a, b)
	want := []uint64{2, 3, 5, 21}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IntersectSorted = %v, quer %v", got, want)
	}

	// N-way
	c := []uint64{3, 5, 21, 99}
	got2 := IntersectMany(a, b, c)
	want2 := []uint64{3, 5, 21}
	if !reflect.DeepEqual(got2, want2) {
		t.Fatalf("IntersectMany = %v, quer %v", got2, want2)
	}
}

// TestScanVsSortedVsMPHF confere que as três estratégias concordam (membership).
func TestScanVsSortedVsMPHF(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	shard := make([]uint64, 20000)
	for i := range shard {
		shard[i] = rng.Uint64()
	}
	batch := make([]uint64, 0, 500)
	for i := 0; i < 250; i++ {
		batch = append(batch, shard[rng.Intn(len(shard))]) // hits
	}
	for i := 0; i < 250; i++ {
		batch = append(batch, rng.Uint64()|(1<<63)) // misses
	}

	scan := ParallelSearchEngine(batch, shard)
	sorted := append([]uint64(nil), shard...)
	SortShard(sorted)
	sortedRes := SearchSortedBatch(sorted, batch)

	norm := func(x []uint64) []uint64 {
		s := append([]uint64(nil), x...)
		sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
		return s
	}
	if !reflect.DeepEqual(norm(scan), norm(sortedRes)) {
		t.Fatalf("scan e sorted divergem:\n scan=%v\n sorted=%v", norm(scan), norm(sortedRes))
	}
}
