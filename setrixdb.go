// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

// Package setrixdb é a API pública do SetrixDB — o motor de conjuntos em memória,
// não-relacional, não-vetorial e puramente aritmético.
//
// Ele trabalha com **IDs `uint64`**: você guarda conjuntos de IDs e faz presença e
// interseção de forma exata, em memória, sem payload. O SetrixDB **coexiste** com o
// seu banco atual — o dado fica onde você confia, o SetrixDB responde as perguntas
// de conjunto.
//
// Exemplo:
//
//	import "github.com/setrixdb/setrixdb"
//
//	a := setrixdb.NewSet(1, 2, 3, 4)
//	b := setrixdb.NewSet(3, 4, 5, 6)
//	fmt.Println(setrixdb.Intersect(a, b).Len()) // 2  (3 e 4)
//
// Este pacote é a **camada embarcável**. Para overhead de rede / cluster, veja o
// comando `cmd/clusterdemo` e `cmd/clusternode`.
package setrixdb

import (
	"sort"

	"github.com/setrixdb/setrixdb/internal/addb"
	"github.com/setrixdb/setrixdb/internal/mphf"
)

// Set é um conjunto de IDs `uint64`. A representação é esparsa; na primeira
// consulta o conjunto é ordenado e deduplicado (idempotente).
type Set struct {
	ids    []uint64
	sorted bool
}

// NewSet cria um conjunto a partir de IDs avulsos.
func NewSet(ids ...uint64) *Set {
	s := &Set{}
	s.Add(ids...)
	return s
}

// Add insere um ou mais IDs no conjunto.
func (s *Set) Add(ids ...uint64) {
	if len(ids) == 0 {
		return
	}
	s.ids = append(s.ids, ids...)
	s.sorted = false
}

// Len devolve o número de IDs **únicos** do conjunto.
func (s *Set) Len() int {
	s.finalize()
	return len(s.ids)
}

// Has informa se o ID pertence ao conjunto (busca binária, O(log n)).
func (s *Set) Has(id uint64) bool {
	s.finalize()
	return addb.ContainsSorted(s.ids, id)
}

// IDs devolve uma cópia ordenada e sem duplicatas dos IDs do conjunto.
func (s *Set) IDs() []uint64 {
	s.finalize()
	out := make([]uint64, len(s.ids))
	copy(out, s.ids)
	return out
}

func (s *Set) finalize() {
	if s.sorted {
		return
	}
	sort.Slice(s.ids, func(i, j int) bool { return s.ids[i] < s.ids[j] })
	w := 0
	for i := range s.ids {
		if w == 0 || s.ids[i] != s.ids[w-1] {
			s.ids[w] = s.ids[i]
			w++
		}
	}
	s.ids = s.ids[:w]
	s.sorted = true
}

// Intersect devolve um **novo** conjunto com os IDs presentes em **todos** os
// conjuntos dados (merge linear, exato). Sem conjuntos, devolve conjunto vazio.
func Intersect(sets ...*Set) *Set {
	if len(sets) == 0 {
		return &Set{sorted: true}
	}
	raw := make([][]uint64, len(sets))
	for i, s := range sets {
		raw[i] = s.IDs()
	}
	return &Set{ids: addb.IntersectMany(raw...), sorted: true}
}

// Union devolve um **novo** conjunto com a união dos IDs de todos os conjuntos.
func Union(sets ...*Set) *Set {
	out := &Set{}
	for _, s := range sets {
		out.ids = append(out.ids, s.IDs()...)
	}
	out.finalize()
	return out
}

// Filter devolve os IDs do conjunto `base` que também estão em `mask` — atalho
// legível para Intersect(base, mask).
func Filter(base, mask *Set) *Set {
	return Intersect(base, mask)
}

// ---------- ID único exato (MPHF) ----------

// Index é um mapeamento **perfeito** de um conjunto de chaves para IDs densos
// (0..n-1), sem colisões entre as chaves indexadas. É a base para representações
// densas (bitset).
//
// Aviso: um MPHF **não guarda as chaves** — sozinho, ele pode acusar falsos
// positivos raros. Para pertencimento **exato**, use Set.Has (que compara o ID).
type Index struct {
	c *mphf.CHD
}

// BuildIndex constrói o índice perfeito para as chaves dadas (0 colisão).
func BuildIndex(keys []uint64) (*Index, error) {
	c, err := mphf.Build(keys, 3.0, 0xADDB)
	if err != nil {
		return nil, err
	}
	return &Index{c: c}, nil
}

// Lookup devolve o ID denso da chave e se ela foi encontrada (O(1)).
// Pode haver falso-positivo raro para chaves ausentes (ver aviso no tipo Index).
func (i *Index) Lookup(key uint64) (uint32, bool) {
	return i.c.Lookup(key)
}

// Len devolve o número de chaves indexadas.
func (i *Index) Len() int { return int(i.c.N) }

// BitsPerKey devolve o custo médio de bits por chave.
func (i *Index) BitsPerKey() float64 { return i.c.BitsPerKey() }

// Bytes devolve o tamanho da estrutura em bytes.
func (i *Index) Bytes() uint64 { return i.c.Bytes() }
