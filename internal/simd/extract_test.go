// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package simd

import (
	"math/bits"
	"math/rand"
	"reflect"
	"testing"
)

func TestExtractSetMatchesScalar(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	words := make([]uint64, 2000)
	for i := range words {
		words[i] = rng.Uint64()
	}
	got := ExtractSet(words)
	want := scalarExtract(words)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("simd.ExtractSet diverge do escalar:\n got=%v\n want=%v", got[:min(20, len(got))], want[:min(20, len(want))])
	}
}

// scalarExtract é a referência escalar do kernel de extração. Fica aqui (e não
// em internal/addb) porque addb passou a importar este pacote na v1.1 — um
// import de volta criaria ciclo de teste.
func scalarExtract(words []uint64) []uint64 {
	var positions []uint64
	for i, word := range words {
		for word != 0 {
			j := bits.TrailingZeros64(word)
			positions = append(positions, uint64(i*64+j))
			word &= word - 1
		}
	}
	return positions
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// caso denso: 1M bits setados em 16K palavras
func denseWords() []uint64 {
	words := make([]uint64, 16384)
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 1_000_000; i++ {
		words[rng.Intn(len(words))] |= 1 << (rng.Intn(64))
	}
	return words
}

func BenchmarkExtractSetScalar(b *testing.B) {
	w := denseWords()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scalarExtract(w)
	}
}

func BenchmarkExtractSetSIMD(b *testing.B) {
	w := denseWords()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractSet(w)
	}
}
