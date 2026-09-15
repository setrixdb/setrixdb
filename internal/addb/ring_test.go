// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import "testing"

func TestRingRoute(t *testing.T) {
	r := NewRing(100)
	r.AddNode("a")
	r.AddNode("b")
	r.AddNode("c")

	seen := map[string]int{}
	for i := uint64(0); i < 10000; i++ {
		n, ok := r.Route(i)
		if !ok {
			t.Fatal("Route retornou ok=false com anel não-vazio")
		}
		seen[n]++
	}
	if len(seen) != 3 {
		t.Fatalf("esperava 3 nós usados, veio %d", len(seen))
	}

	// determinismo
	n1, _ := r.Route(12345)
	n2, _ := r.Route(12345)
	if n1 != n2 {
		t.Fatal("Route não é determinístico")
	}

	// add idempotente
	r.AddNode("a")
	if len(r.Nodes()) != 3 {
		t.Fatal("AddNode duplicado mudou o conjunto de nós")
	}

	// remoção tira o nó da rota
	r.RemoveNode("b")
	if got, ok := r.Route(12345); !ok || got == "b" {
		t.Fatalf("nó removido ainda é roteado: %q", got)
	}
	if len(r.Nodes()) != 2 {
		t.Fatalf("esperava 2 nós, veio %d", len(r.Nodes()))
	}

	// anel vazio
	r2 := NewRing(10)
	if _, ok := r2.Route(1); ok {
		t.Fatal("anel vazio não deveria rotear")
	}
}
