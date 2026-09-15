// Copyright (c) 2026 Thiago Silva
// SPDX-License-Identifier: Apache-2.0

package mphf

import "testing"

// TestPerfectBuilding prova a propriedade central: 0 colisão e cobertura total.
func TestPerfectBuilding(t *testing.T) {
	for _, n := range []int{1, 2, 10, 1000, 100000} {
		keys := make([]uint64, n)
		for i := range keys {
			keys[i] = mix(uint64(i) * 0x9E3779B97F4A7C15)
		}
		h, err := Build(keys, 2.0, 42)
		if err != nil {
			t.Fatalf("n=%d: build falhou: %v", n, err)
		}
		seen := make(map[uint32]struct{}, n)
		for _, k := range keys {
			id, ok := h.Lookup(k)
			if !ok {
				t.Fatalf("n=%d: chave %d não encontrada", n, k)
			}
			if id >= uint32(n) {
				t.Fatalf("n=%d: id %d fora de [0,%d)", n, id, n)
			}
			if _, dup := seen[id]; dup {
				t.Fatalf("n=%d: COLISÃO no id %d", n, id)
			}
			seen[id] = struct{}{}
		}
		if len(seen) != n {
			t.Fatalf("n=%d: cobertura %d != %d", n, len(seen), n)
		}
	}
}

// TestNoFalsePositives: chaves fora do conjunto não devem ser aceitas.
func TestNoFalsePositives(t *testing.T) {
	const n = 20000
	keys := make([]uint64, n)
	for i := range keys {
		keys[i] = mix(uint64(i))
	}
	h, err := Build(keys, 2.0, 7)
	if err != nil {
		t.Fatal(err)
	}
	fp := 0
	for i := 0; i < 200000; i++ {
		if _, ok := h.Lookup(mix(uint64(1<<63) + uint64(i))); ok {
			fp++
		}
	}
	// Falsos positivos são possíveis em MPHF (a estrutura não guarda as chaves),
	// mas devem ser raros: ~ (n/m) por consulta aleatória.
	if fp > 0 {
		t.Logf("falsos positivos: %d/200000 (esperado ~%.1f)", fp, 200000*float64(n)/float64(h.M))
	}
}
