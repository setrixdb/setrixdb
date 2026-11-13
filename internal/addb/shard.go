// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

import (
	"runtime"
	"sync"
)

// shardMode — representação escolhida para um shard.
type shardMode uint8

const (
	// modeDense = bitset (ShardBits/8 bytes, custo fixo pelo universo).
	modeDense shardMode = iota
	// modeSparse = lista ordenada de offsets (8 bytes por ID).
	modeSparse
)

// ShardedBitset particiona o espaço de IDs (uint64) em `NumShards` ranges
// CONTÍGUOS. É a unidade de escala horizontal do SetrixDB: membership roteia
// O(1) para o shard certo; a interseção é broadcast por shard (embarrassingly
// parallel).
//
// **Híbrido por shard (v1.1):** cada shard escolhe a representação pelo custo
// de memória. Densidade alta → bitset (custo fixo, AND vetorizado). Densidade
// baixa → SparseSet (paga só pelos IDs que existem). A escolha é feita em
// Finalize (ou na primeira consulta) e os dois lados da interseção podem estar
// em modos diferentes — o dispatch é por par de shards.
//
// Dois ShardedBitset só são compatíveis para interseção se tiverem o MESMO
// NumShards (mesmo ShardBits).
type ShardedBitset struct {
	NumShards int
	ShardBits uint64

	shards []*Bitset
	sparse []*SparseSet
	modes  []shardMode
	mu     sync.Mutex
	hybrid bool // já decidiu os modos?
}

// NewShardedBitset cria um espaço de `universe` IDs fatiado em `numShards` ranges.
func NewShardedBitset(universe uint64, numShards int) *ShardedBitset {
	sb := &ShardedBitset{NumShards: numShards}
	sb.ShardBits = (universe + uint64(numShards) - 1) / uint64(numShards)
	sb.shards = make([]*Bitset, numShards)
	sb.sparse = make([]*SparseSet, numShards)
	sb.modes = make([]shardMode, numShards)
	for i := range sb.shards {
		sb.shards[i] = NewBitset(sb.ShardBits)
	}
	return sb
}

func (s *ShardedBitset) shardOf(id uint64) int {
	sh := int(id / s.ShardBits)
	if sh < 0 || sh >= s.NumShards {
		return -1
	}
	return sh
}

// Set liga o bit do ID no shard correspondente (roteamento O(1)).
//
// Escrita depois de Finalize volta o shard tocado para a representação densa
// (o custo de reconstruir o bitset é o mesmo de criar um shard novo); os outros
// shards ficam como estavam.
func (s *ShardedBitset) Set(id uint64) {
	sh := s.shardOf(id)
	if sh < 0 {
		return
	}
	off := id - uint64(sh)*s.ShardBits

	s.mu.Lock()
	if s.modes[sh] == modeSparse {
		s.shards[sh] = bitsetFromIDs(s.sparse[sh].ids, s.ShardBits)
		s.sparse[sh] = nil
		s.modes[sh] = modeDense
	}
	s.mu.Unlock()

	s.shards[sh].Set(off)
}

// bitsetFromIDs monta um Bitset a partir de IDs (offsets) do shard.
func bitsetFromIDs(ids []uint64, shardBits uint64) *Bitset {
	b := NewBitset(shardBits)
	for _, id := range ids {
		b.Set(id)
	}
	return b
}

// sparseModeThreshold — ponto de virada por MEMÓRIA: denso custa ShardBits/8
// bytes, esparso custa 8 bytes por ID. Esparso vale quando
// 8*n < ShardBits/8, isto é, n < ShardBits/64.
func sparseModeThreshold(shardBits uint64) uint64 { return shardBits / 64 }

// Finalize decide a representação de cada shard (idempotente).
func (s *ShardedBitset) Finalize() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finalizeLocked()
}

func (s *ShardedBitset) finalizeLocked() {
	if s.hybrid {
		return
	}
	limit := sparseModeThreshold(s.ShardBits)
	for i := 0; i < s.NumShards; i++ {
		n := uint64(s.shards[i].AndPopcount(s.shards[i])) // |shard|
		// n == 0 também vira esparso: shard vazio não deve custar um bitset
		// inteiro (16 KB num universo de 1 Mi com 8 shards).
		if n < limit {
			if n == 0 {
				s.sparse[i] = NewSparseSet(nil)
			} else {
				s.sparse[i] = NewSparseSet(s.shards[i].Extract())
				s.filterShardOverflow(i)
			}
			s.shards[i] = nil
			s.modes[i] = modeSparse
		}
	}
	s.hybrid = true
}

// filterShardOverflow remove offsets >= ShardBits (o Bitset aloca
// ShardBits/64+1 palavras, então o último bit pode passar do range do shard).
func (s *ShardedBitset) filterShardOverflow(i int) {
	if s.sparse[i] == nil {
		return
	}
	ids := s.sparse[i].ids[:0]
	for _, id := range s.sparse[i].ids {
		if id < s.ShardBits {
			ids = append(ids, id)
		}
	}
	s.sparse[i].ids = ids
}

// Modes devolve a representação de cada shard ("dense" ou "sparse") — depois de
// garantir a decisão.
func (s *ShardedBitset) Modes() []string {
	s.Finalize()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, s.NumShards)
	for i, m := range s.modes {
		if m == modeSparse {
			out[i] = "sparse"
		} else {
			out[i] = "dense"
		}
	}
	return out
}

// countShard devolve |A ∩ B| para um shard, despachando pelo par de modos.
func (s *ShardedBitset) countShard(i int, o *ShardedBitset) int64 {
	sd := s.modes[i] == modeDense
	od := o.modes[i] == modeDense
	switch {
	case sd && od:
		return s.shards[i].AndPopcount(o.shards[i])
	case !sd && !od:
		return s.sparse[i].AndCount(o.sparse[i])
	case !sd && od:
		return sparseInDense(s.sparse[i], o.shards[i])
	default:
		return sparseInDense(o.sparse[i], s.shards[i])
	}
}

// sparseInDense conta quantos IDs do esparso existem no bitset.
func sparseInDense(sp *SparseSet, d *Bitset) int64 {
	var c int64
	for _, id := range sp.ids {
		if d.Test(id) {
			c++
		}
	}
	return c
}

// Has informa se o ID está no conjunto (funciona nos dois modos).
func (s *ShardedBitset) Has(id uint64) bool {
	sh := s.shardOf(id)
	if sh < 0 {
		return false
	}
	off := id - uint64(sh)*s.ShardBits

	s.mu.Lock()
	mode, sp := s.modes[sh], s.sparse[sh]
	s.mu.Unlock()

	if mode == modeSparse && sp != nil {
		return sp.Has(off)
	}
	return s.shards[sh].Test(off)
}

// Words expõe as palavras de um shard denso (para kernels vetorizados); um
// shard esparso devolve nil — quem consome palavras precisa checar o modo.
func (s *ShardedBitset) Words(sh int) []uint64 {
	if s.modes[sh] == modeSparse {
		return nil
	}
	return s.shards[sh].Words
}

// IntersectCountSerial calcula |A ∩ B| varrendo os shards em sequência.
func (s *ShardedBitset) IntersectCountSerial(o *ShardedBitset) int64 {
	s.Finalize()
	o.Finalize()
	var total int64
	for i := 0; i < s.NumShards; i++ {
		total += s.countShard(i, o)
	}
	return total
}

// IntersectCount calcula |A ∩ B| distribuindo os shards entre as CPUs.
func (s *ShardedBitset) IntersectCount(o *ShardedBitset) int64 {
	s.Finalize()
	o.Finalize()
	workers := runtime.GOMAXPROCS(0)
	if workers > s.NumShards {
		workers = s.NumShards
	}
	if workers <= 1 {
		return s.IntersectCountSerial(o)
	}
	var total int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	chunk := (s.NumShards + workers - 1) / workers
	for w := 0; w < workers; w++ {
		lo := w * chunk
		if lo >= s.NumShards {
			break
		}
		hi := lo + chunk
		if hi > s.NumShards {
			hi = s.NumShards
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			var c int64
			for i := lo; i < hi; i++ {
				c += s.countShard(i, o)
			}
			mu.Lock()
			total += c
			mu.Unlock()
		}(lo, hi)
	}
	wg.Wait()
	return total
}

// MemBytes devolve o total de bytes alocado (bitsets dos shards densos +
// listas dos shards esparsos).
func (s *ShardedBitset) MemBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b int64
	for i := 0; i < s.NumShards; i++ {
		if s.modes[i] == modeSparse && s.sparse[i] != nil {
			b += s.sparse[i].MemBytes()
			continue
		}
		if s.shards[i] != nil {
			b += int64(len(s.shards[i].Words) * 8)
		}
	}
	return b
}
