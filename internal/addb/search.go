// Package addb implementa o núcleo do SetrixDB — Arithmetic Database.
//
// Princípios: banco em memória, não-relacional, não-vetorial, puramente
// aritmético. A primitiva central é a igualdade entre inteiros uint64,
// organizada em shards contíguos para permitir paralelismo de dados,
// auto-vetorização (SIMD/AVX-512) e transferência zero-copy para NPUs.
package addb

import (
	"runtime"
	"sync"
	"unsafe"
)

// Shard é um bloco contíguo de IDs em RAM. É a unidade de armazenamento do SetrixDB.
type Shard struct {
	IDs []uint64
}

// NewShard copia ids para um bloco contíguo novo.
func NewShard(ids []uint64) Shard {
	buf := make([]uint64, len(ids))
	copy(buf, ids)
	return Shard{IDs: buf}
}

// Contains executa o kernel aritmético: varredura branchless de igualdade.
// O laço interno é escrito de forma a ser auto-vetorizável (SIMD).
func Contains(shard []uint64, q uint64) bool {
	var found uint64
	for _, v := range shard {
		// Igualdade branchless: x==0 -> 1, senão 0.
		// (x | -x) tem o bit alto ligado sse x != 0.
		x := v ^ q
		eq := 1 - ((x | (^x + 1)) >> 63) // 1 se v==q, 0 caso contrário
		found |= eq
	}
	return found == 1
}

// ParallelSearchEngine busca em paralelo todos os IDs de batch presentes no shard.
//
// O batch de consultas é particionado entre GOMAXPROCS(0) workers; cada worker
// varre o shard inteiro para o seu pedaço. O resultado preserva a ordem do batch
// e não contém duplicatas.
func ParallelSearchEngine(batch, shard []uint64) []uint64 {
	if len(batch) == 0 || len(shard) == 0 {
		return nil
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > len(batch) {
		workers = len(batch)
	}

	chunk := (len(batch) + workers - 1) / workers
	results := make([][]uint64, workers)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		lo := w * chunk
		if lo >= len(batch) {
			break
		}
		hi := lo + chunk
		if hi > len(batch) {
			hi = len(batch)
		}

		wg.Add(1)
		go func(w, lo, hi int) {
			defer wg.Done()
			part := batch[lo:hi]
			local := make([]uint64, 0, len(part))
			for _, q := range part {
				if Contains(shard, q) {
					local = append(local, q)
				}
			}
			results[w] = local
		}(w, lo, hi)
	}
	wg.Wait()

	return mergeUnique(results)
}

// mergeUnique concatena os resultados parciais preservando a ordem e removendo
// duplicatas (cada ID aparece no máximo uma vez).
func mergeUnique(parts [][]uint64) []uint64 {
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	out := make([]uint64, 0, total)

	seen := make(map[uint64]struct{}, total)
	for _, p := range parts {
		for _, id := range p {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

// UnsafePtr expõe o endereço da backing array de um shard para transferência
// DMA zero-copy a drivers C/C++ de NPU/acelerador.
//
// CONTRATO: o chamador deve garantir que o slice (ou o Shard que o contém)
// permaneça vivo e não seja realocado durante toda a transferência. Para
// transferências assíncronas, a memória deve ser pinada explicitamente.
func UnsafePtr(shard []uint64) unsafe.Pointer {
	if len(shard) == 0 {
		return nil
	}
	return unsafe.Pointer(&shard[0])
}
