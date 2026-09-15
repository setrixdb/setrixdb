package simd

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/setrixdb/setrixdb/internal/addb"
)

func TestExtractSetMatchesScalar(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	words := make([]uint64, 2000)
	for i := range words {
		words[i] = rng.Uint64()
	}
	got := ExtractSet(words)
	want := addb.ExtractSet(words)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("simd.ExtractSet diverge do escalar:\n got=%v\n want=%v", got[:min(20,len(got))], want[:min(20,len(want))])
	}
}

func min(a, b int) int { if a < b { return a }; return b }

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
		_ = addb.ExtractSet(w)
	}
}

func BenchmarkExtractSetSIMD(b *testing.B) {
	w := denseWords()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtractSet(w)
	}
}
