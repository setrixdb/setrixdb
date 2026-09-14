package addb

import "testing"

func TestSparseSet(t *testing.T) {
	s := NewSparseSet([]uint64{5, 1, 5, 3, 100, 3, 1})
	if s.Len() != 4 { // 1,3,5,100
		t.Fatalf("Len = %d, esperava 4", s.Len())
	}
	for _, id := range []uint64{1, 3, 5, 100} {
		if !s.Has(id) {
			t.Fatalf("Has(%d) = false", id)
		}
	}
	for _, id := range []uint64{0, 2, 4, 99, 101} {
		if s.Has(id) {
			t.Fatalf("Has(%d) = true (falso positivo)", id)
		}
	}

	o := NewSparseSet([]uint64{3, 100, 200, 5})
	if got := s.AndCount(o); got != 3 { // 3,5,100
		t.Fatalf("AndCount = %d, esperava 3", got)
	}
}
