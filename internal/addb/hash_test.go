// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import "testing"

func TestComputeDeterministicID_KnownValues(t *testing.T) {
	cases := map[string]uint64{
		"casa":    3125458,
		"moradia": 95578313231,
		"lar":     115615,
	}
	for term, want := range cases {
		if got := ComputeDeterministicID(term); got != want {
			t.Errorf("ComputeDeterministicID(%q) = %d, quer %d", term, got, want)
		}
	}
}

// TestKnownCollision documenta uma FALHA conhecida da função posicional atual:
// o ID não é injetivo (dígitos > base 31 ⇒ representação não canônica).
// Ver docs/REVISAO-ARQUITETURA.md, item 1. Enquanto esse teste passar,
// a colisão existe — e é exatamente o motivo de NÃO tratar o ID como único.
func TestKnownCollision(t *testing.T) {
	if got, want := ComputeDeterministicID("Oa"), ComputeDeterministicID("0b"); got != want {
		t.Fatalf("colisão esperada deixou de existir: %d != %d (função mudou?)", got, want)
	}

	if got, want := ComputeDeterministicID("AA"), ComputeDeterministicID("\u085e"); got != want {
		t.Fatalf("colisão esperada deixou de existir: %d != %d (função mudou?)", got, want)
	}
}

// TestCollisionRate mede a taxa de colisão num corpus sintético — a métrica que
// falta na spec. Serve de base para o card "validar colisão do hash".
func TestCollisionRate(t *testing.T) {
	const n = 200000
	seen := make(map[uint64]string, n)
	collisions := 0
	for i := 0; i < n; i++ {
		term := termFor(i)
		id := ComputeDeterministicID(term)
		if prev, ok := seen[id]; ok && prev != term {
			collisions++
			continue
		}
		seen[id] = term
	}
	t.Logf("corpus=%d colisoes=%d taxa=%.3g", n, collisions, float64(collisions)/float64(n))
}

// termFor gera termos alfanuméricos curtos (o pior caso da função posicional).
func termFor(i int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if i == 0 {
		return "0"
	}
	var buf []byte
	for i > 0 {
		buf = append(buf, alphabet[i%len(alphabet)])
		i /= len(alphabet)
	}
	return string(buf)
}
