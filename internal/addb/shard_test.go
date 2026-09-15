// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import "testing"

func TestShardedBitset(t *testing.T) {
	// universo 1000, 4 shards (250 bits cada)
	s := NewShardedBitset(1000, 4)
	ids := []uint64{0, 1, 249, 250, 500, 999, 250, 500}
	for _, id := range ids {
		s.Set(id)
	}
	for _, id := range []uint64{0, 249, 250, 500, 999} {
		if !s.Has(id) {
			t.Fatalf("Has(%d) = false, esperava true", id)
		}
	}
	if s.Has(2) || s.Has(1000) || s.Has(1<<40) {
		t.Fatal("falso positivo em Has")
	}

	// interseção com outro sharded do mesmo shape
	o := NewShardedBitset(1000, 4)
	for _, id := range []uint64{250, 500, 999, 123} {
		o.Set(id)
	}
	got := s.IntersectCountSerial(o)
	if got != 3 { // 250, 500, 999
		t.Fatalf("IntersectCountSerial = %d, esperava 3", got)
	}
	if s.IntersectCount(o) != 3 {
		t.Fatalf("IntersectCount (paralela) != 3")
	}
}
