package addb

import (
	"runtime"
	"sync"
)

// ShardedBitset particiona o espaço de IDs (uint64) em `NumShards` ranges
// CONTÍGUOS, cada um representado como um Bitset denso. É a unidade de
// escala horizontal do SetrixDB: membership roteia O(1) para o shard certo;
// a interseção é broadcast por shard (embarrassingly parallel).
//
// Dois ShardedBitset só são compatíveis para interseção se tiverem o MESMO
// NumShards (mesmo ShardBits).
type ShardedBitset struct {
	NumShards int
	ShardBits uint64
	shards    []*Bitset
}

// NewShardedBitset cria um espaço de `universe` IDs fatiado em `numShards` ranges.
func NewShardedBitset(universe uint64, numShards int) *ShardedBitset {
	sb := &ShardedBitset{NumShards: numShards}
	sb.ShardBits = (universe + uint64(numShards) - 1) / uint64(numShards)
	sb.shards = make([]*Bitset, numShards)
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
func (s *ShardedBitset) Set(id uint64) {
	sh := s.shardOf(id)
	if sh < 0 {
		return
	}
	s.shards[sh].Set(id - uint64(sh)*s.ShardBits)
}

// Has informa se o ID está no conjunto.
func (s *ShardedBitset) Has(id uint64) bool {
	sh := s.shardOf(id)
	if sh < 0 {
		return false
	}
	return s.shards[sh].Test(id - uint64(sh)*s.ShardBits)
}

// Words expõe as palavras de um shard (para kernels vetorizados).
func (s *ShardedBitset) Words(sh int) []uint64 { return s.shards[sh].Words }

// IntersectCountSerial calcula |A ∩ B| varrendo os shards em sequência.
func (s *ShardedBitset) IntersectCountSerial(o *ShardedBitset) int64 {
	var total int64
	for i := 0; i < s.NumShards; i++ {
		total += s.shards[i].AndPopcount(o.shards[i])
	}
	return total
}

// IntersectCount calcula |A ∩ B| distribuindo os shards entre as CPUs.
func (s *ShardedBitset) IntersectCount(o *ShardedBitset) int64 {
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
				c += s.shards[i].AndPopcount(o.shards[i])
			}
			mu.Lock()
			total += c
			mu.Unlock()
		}(lo, hi)
	}
	wg.Wait()
	return total
}

// MemBytes devolve o total de bytes alocado pelos bitsets dos shards.
func (s *ShardedBitset) MemBytes() int64 {
	var b int64
	for _, sh := range s.shards {
		b += int64(len(sh.Words) * 8)
	}
	return b
}
