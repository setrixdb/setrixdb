// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import (
	"math/rand"
	"testing"
)

func idsOf(sb *ShardedBitset, universe uint64) map[uint64]bool {
	m := map[uint64]bool{}
	for id := uint64(0); id < universe; id++ {
		if sb.Has(id) {
			m[id] = true
		}
	}
	return m
}

// A decisão é por MEMÓRIA, não por heurística de performance: shard esparso vira
// SparseSet, shard denso continua bitset.
func TestShardedHybridModesChooseByMemory(t *testing.T) {
	universe := uint64(1 << 20)
	sb := NewShardedBitset(universe, 8)  // 131072 bits por shard, virada em 2048 IDs
	for id := uint64(0); id < 10; id++ { // shard 0: 10 IDs → esparso
		sb.Set(id)
	}
	for id := uint64(131072); id < 131072+50000; id++ { // shard 1: 50k IDs → denso
		sb.Set(id)
	}
	modes := sb.Modes()
	if modes[0] != "sparse" {
		t.Fatalf("shard 0 (10 IDs) deveria ser sparse, veio %s", modes[0])
	}
	if modes[1] != "dense" {
		t.Fatalf("shard 1 (50k IDs) deveria ser dense, veio %s", modes[1])
	}
	for i := 2; i < 8; i++ {
		if modes[i] != "sparse" { // vazio também é esparso: custo zero, não um bitset
			t.Fatalf("shard vazio %d deveria ser sparse (custo zero): %s", i, modes[i])
		}
	}
	if sb.MemBytes() == 0 || sb.MemBytes() >= 8*16392 {
		t.Fatalf("shards vazios não podem custar bitset: %d B", sb.MemBytes())
	}
	// memória: o esparso não pode custar o bitset inteiro
	if got, full := sb.MemBytes(), int64(8)*int64(len(sb.shards[1].Words)*8); got >= full {
		t.Fatalf("MemBytes=%d não pode alcançar o custo denso total (%d)", got, full)
	}
	if !sb.Has(5) || sb.Has(10) {
		t.Fatal("Has quebrou no modo esparso")
	}
	if sb.Words(0) != nil {
		t.Fatal("shard esparso não tem palavras")
	}
}

// Híbrido tem de devolver a MESMA contagem que a referência por mapa, em
// qualquer densidade — e o modo de cada shard muda com a densidade.
func TestShardedHybridParity(t *testing.T) {
	universe := uint64(1 << 16)
	for _, fill := range []int{0, 50, 2000, 30000} { // vazio, bem esparso, na virada, denso
		s := NewShardedBitset(universe, 8)
		o := NewShardedBitset(universe, 8)
		rng := rand.New(rand.NewSource(int64(fill) + 1))
		for i := 0; i < fill; i++ {
			s.Set(uint64(rng.Int63n(int64(universe))))
			o.Set(uint64(rng.Int63n(int64(universe))))
		}
		a, b := idsOf(s, universe), idsOf(o, universe)
		var want int64
		for id := range a {
			if b[id] {
				want++
			}
		}
		if got := s.IntersectCountSerial(o); got != want {
			t.Fatalf("fill=%d serial: got=%d want=%d (modos %v)", fill, got, want, s.Modes())
		}
		if got := s.IntersectCount(o); got != want {
			t.Fatalf("fill=%d paralela: got=%d want=%d", fill, got, want)
		}
	}
}

// Um lado esparso e o outro denso NO MESMO SHARD: o dispatch por par tem de
// tratar os quatro casos.
func TestShardedHybridMixedModes(t *testing.T) {
	universe := uint64(1 << 20) // 8 shards de 131072 bits; virada em 2048 IDs
	sparse := NewShardedBitset(universe, 8)
	dense := NewShardedBitset(universe, 8)

	// sparse: 100 IDs no shard 3
	for id := uint64(0); id < 100; id++ {
		sparse.Set(3*131072 + id)
	}
	// dense: 60k IDs no shard 3, cobrindo os 100 do outro
	for id := uint64(0); id < 60000; id++ {
		dense.Set(3*131072 + id)
	}
	if m := sparse.Modes(); m[3] != "sparse" {
		t.Fatalf("esperava shard 3 esparso no primeiro conjunto: %v", m)
	}
	if m := dense.Modes(); m[3] != "dense" {
		t.Fatalf("esperava shard 3 denso no segundo conjunto: %v", m)
	}

	want := int64(100) // todos os 100 do esparso existem no denso
	if got := sparse.IntersectCountSerial(dense); got != want {
		t.Fatalf("esparso ∩ denso = %d, esperava %d", got, want)
	}
	if got := dense.IntersectCountSerial(sparse); got != want {
		t.Fatalf("denso ∩ esparso = %d, esperava %d", got, want)
	}
	if got := sparse.IntersectCount(dense); got != want {
		t.Fatalf("paralela esparso ∩ denso = %d, esperava %d", got, want)
	}
}

// Escrita depois de Finalize volta o shard tocado para denso e não corrompe a
// contagem.
func TestShardedSetAfterFinalize(t *testing.T) {
	universe := uint64(1 << 20)
	s := NewShardedBitset(universe, 8)
	o := NewShardedBitset(universe, 8)
	s.Set(10)
	o.Set(10)
	_ = s.Modes() // força a decisão (esparso)
	_ = o.Modes()
	if s.IntersectCountSerial(o) != 1 {
		t.Fatal("interseção inicial")
	}

	s.Set(11)
	o.Set(11)
	if !s.Has(11) {
		t.Fatal("Set depois de Finalize não valeu")
	}
	if got := s.IntersectCountSerial(o); got != 2 {
		t.Fatalf("após escrita: got=%d, esperava 2", got)
	}
	if m := s.Modes(); m[0] != "dense" {
		t.Fatalf("shard reescrito deveria voltar a denso: %v", m)
	}
}

// O ganho que justifica o híbrido: conjunto esparso ocupa muito menos memória.
func TestShardedHybridSavesMemory(t *testing.T) {
	universe := uint64(1 << 20)
	s := NewShardedBitset(universe, 8)
	for id := uint64(0); id < 800; id++ { // 100 por shard, bem abaixo da virada
		s.Set(id * 1000)
	}
	s.Finalize()
	sparseBytes := s.MemBytes()

	full := NewShardedBitset(universe, 8) // referência: todos densos, sem Finalize
	if len(full.shards) != 8 {
		t.Fatal("sanidade")
	}
	var denseBytes int64
	for i := range full.shards {
		denseBytes += int64(len(full.shards[i].Words) * 8)
	}
	t.Logf("memória: híbrido=%d B · denso=%d B (%.1fx menos)", sparseBytes, denseBytes, float64(denseBytes)/float64(sparseBytes))
	if sparseBytes >= denseBytes/2 {
		t.Fatalf("híbrido deveria economizar ao menos 50%%: esparso=%d denso=%d", sparseBytes, denseBytes)
	}
}
