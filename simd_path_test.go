// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package setrixdb

import (
	"math/rand"
	"testing"

	"github.com/setrixdb/setrixdb/internal/addb"
)

// densePlan decide o caminho pelo custo, não por flag. Este teste fixa a régua:
// denso quando os IDs ocupam pouco universo, esparso quando ocupam muito.
func TestDensePlan(t *testing.T) {
	dense := [][]uint64{{0, 1, 2, 3}, {2, 3, 4, 5}}
	if _, ok := densePlan(dense); !ok {
		t.Fatal("4 IDs em universo de 6 deveria escolher o caminho denso")
	}

	// 100 IDs espalhados em 1 Mi de universo: bitset custaria mais que varrer.
	sparse := make([][]uint64, 2)
	var all []uint64
	for i := uint64(0); i < 100; i++ {
		all = append(all, i<<13)
	}
	sparse[0] = append([]uint64(nil), all...)
	sparse[1] = append([]uint64(nil), all...)
	if _, ok := densePlan(sparse); ok {
		t.Fatal("IDs esparsos deveriam ficar no merge ordenado")
	}

	if _, ok := densePlan(nil); ok {
		t.Fatal("sem conjuntos não há caminho denso")
	}
	if _, ok := densePlan([][]uint64{{1, 2, 3}}); ok {
		t.Fatal("um conjunto só não compensa bitset")
	}
	if _, ok := densePlan([][]uint64{{}, {}}); ok {
		t.Fatal("conjuntos vazios não têm universo")
	}
}

// A API pública não pode mudar de resposta por causa da otimização: os dois
// caminhos são exercitados e comparados com o merge de referência.
func TestIntersectBothPathsAgree(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for _, tc := range []struct {
		name     string
		universe uint64
		sets     int
		fill     int
	}{
		{"denso", 5000, 3, 4000},
		{"esparso", 1 << 20, 2, 300},
		{"disjunto", 1000, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sets := make([]*Set, tc.sets)
			raw := make([][]uint64, tc.sets)
			for i := range sets {
				m := map[uint64]bool{}
				for len(m) < tc.fill && uint64(len(m)) < tc.universe {
					m[uint64(rng.Int63n(int64(tc.universe)))] = true
				}
				ids := make([]uint64, 0, len(m))
				for id := range m {
					ids = append(ids, id)
				}
				sets[i] = NewSet(ids...)
				// referência é o merge em listas ORDENADAS, como o produto faz
				raw[i] = sets[i].IDs()
			}
			want := addb.IntersectMany(raw...)
			got := Intersect(sets...)
			if got.Len() != len(want) {
				t.Fatalf("Len()=%d, merge=%d", got.Len(), len(want))
			}
			for i, id := range got.IDs() {
				if id != want[i] {
					t.Fatalf("posição %d: %d != %d", i, id, want[i])
				}
			}
			// um conjunto só / nenhum continuam valendo
			if n := Intersect(sets[0]).Len(); n != sets[0].Len() {
				t.Fatalf("um conjunto deveria devolver ele mesmo: %d != %d", n, sets[0].Len())
			}
			if Intersect().Len() != 0 {
				t.Fatal("sem conjuntos deveria ser vazio")
			}
		})
	}
}

// Duplicatas e ordem não importam: o Set finaliza antes da interseção.
func TestIntersectWithDuplicates(t *testing.T) {
	a := NewSet(9, 1, 5, 5, 1)
	b := NewSet(5, 9, 7)
	got := Intersect(a, b)
	if got.Len() != 2 || got.IDs()[0] != 5 || got.IDs()[1] != 9 {
		t.Fatalf("interseção = %v", got.IDs())
	}
}
