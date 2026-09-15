package setrixdb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetBasics(t *testing.T) {
	s := NewSet(5, 1, 3, 3, 1)
	if s.Len() != 3 { // {1,3,5}, dedup
		t.Fatalf("Len = %d, esperado 3", s.Len())
	}
	if !s.Has(3) || s.Has(4) {
		t.Fatalf("Has inválido: 3=%v 4=%v", s.Has(3), s.Has(4))
	}
}

func TestIntersect(t *testing.T) {
	a := NewSet(1, 2, 3, 4)
	b := NewSet(3, 4, 5, 6)
	c := NewSet(4, 5, 6, 7)
	got := Intersect(a, b)
	if got.Len() != 2 || !got.Has(3) || !got.Has(4) {
		t.Fatalf("Intersect(a,b) = %v", got.IDs())
	}
	if all := Intersect(a, b, c); all.Len() != 1 || !all.Has(4) {
		t.Fatalf("Intersect(a,b,c) = %v, esperado {4}", all.IDs())
	}
	if Intersect().Len() != 0 {
		t.Fatal("Intersect() deveria ser vazio")
	}
}

func TestUnion(t *testing.T) {
	u := Union(NewSet(1, 2), NewSet(2, 3), NewSet(10))
	if u.Len() != 4 {
		t.Fatalf("Union = %v, esperado 4 elementos", u.IDs())
	}
}

func TestWriteReadFile(t *testing.T) {
	orig := NewSet(9, 3, 7, 1)
	path := filepath.Join(t.TempDir(), "s.sxset")
	if err := orig.WriteFile(path); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Len() != orig.Len() {
		t.Fatalf("Len = %d, esperado %d", got.Len(), orig.Len())
	}
	if !got.Has(7) || got.Has(8) {
		t.Fatal("conteúdo divergente após round-trip")
	}
}

func TestReadFileInvalido(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.sxset")
	if err := os.WriteFile(path, []byte("nao-e-sxset"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(path); err == nil {
		t.Fatal("arquivo inválido deveria dar erro")
	}
}

func TestIndex(t *testing.T) {
	keys := []uint64{10, 20, 30, 40}
	idx, err := BuildIndex(keys)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Len() != 4 {
		t.Fatalf("Len = %d", idx.Len())
	}
	seen := map[uint32]bool{}
	for _, k := range keys {
		id, ok := idx.Lookup(k)
		if !ok {
			t.Fatalf("chave %d deveria resolver", k)
		}
		if seen[id] {
			t.Fatalf("IDs densos devem ser únicos (colisão em %d)", id)
		}
		seen[id] = true
	}
}
