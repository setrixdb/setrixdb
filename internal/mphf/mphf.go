// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

// Package mphf implementa um Minimal Perfect Hash Function (MPHF) no estilo
// CHD (Czech–Havas–Majewski) escrito do zero — sem dependência de terceiros.
//
// CHD v2: separa o NÚMERO DE BUCKETS (n/λ) do TAMANHO DA TABELA (n·(1+ε)).
// A v1 confundia os dois (M = n/λ para ambos), o que estourava a largura do
// deslocamento em load factor alto. Agora o deslocamento é por bucket sobre
// uma tabela global com folga ε → deslocamentos pequenos → bem menos bits.
//
// Propriedade: mapeia N chaves → ID em [0, N) SEM colisão. NÃO decide
// membership (a verificação do termo fica no dicionário).
package mphf

import (
	"errors"
	"math/bits"
	"sort"
)

// CHD é um perfect hash construído sobre um conjunto fixo de chaves.
type CHD struct {
	N     uint32   // número de chaves
	M     uint32   // tamanho da tabela (slots) = N*(1+ε)
	NB    uint32   // número de buckets = N/λ
	gpack []uint64 // deslocamentos (displacement) empacotados, gw bits cada (NB entradas)
	gw    uint8    // largura em bits de cada deslocamento
	maxD  uint32   // maior deslocamento usado
	words []uint64 // bitset de slots ocupados (M bits)
	l1    []uint32 // rank nível 1 (bloco de 512 bits)
	seed1 uint64
	seed2 uint64
}

const maxDisplacement = 1<<24 - 1
const defaultEpsilon = 0.23

// mix é o finalizador splitmix64 (avalanche forte e bijetivo).
func mix(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

func (c *CHD) h1(k uint64) uint64 { return mix(k^c.seed1) % uint64(c.NB) }
func (c *CHD) h2(k uint64) uint64 { return mix(k^c.seed2) % uint64(c.M) }

// ErrBuild indica falha de construção mesmo após retries.
var ErrBuild = errors.New("mphf: construção falhou (reduza λ ou aumente ε)")

// Build constrói o MPHF. λ é o fator de carga dos BUCKETS (chaves/bucket);
// ε (folga da tabela) usa o default. Recomendado: λ ≈ 2.0.
func Build(keys []uint64, lambda float64, seed uint64) (*CHD, error) {
	return BuildOpts(keys, lambda, defaultEpsilon, seed)
}

// BuildOpts constrói o MPHF com controle explícito de λ e ε.
func BuildOpts(keys []uint64, lambda, epsilon float64, seed uint64) (*CHD, error) {
	n := len(keys)
	if n == 0 {
		return &CHD{N: 0, M: 0}, nil
	}
	NB := uint32(float64(n)/lambda) + 1
	M := uint32(float64(n)*(1+epsilon)) + 1
	if M <= NB {
		M = NB + 1
	}
	c := &CHD{N: uint32(n), M: M, NB: NB, seed1: seed}

	// CSR: chaves agrupadas por bucket (h1).
	counts := make([]int32, NB)
	for _, k := range keys {
		counts[c.h1(k)]++
	}
	starts := make([]int32, NB)
	var s int32
	for j := 0; j < int(NB); j++ {
		starts[j] = s
		s += counts[j]
	}
	keyAt := make([]uint32, n)
	for j := 0; j < int(NB); j++ {
		counts[j] = starts[j]
	}
	for i, k := range keys {
		j := c.h1(k)
		keyAt[counts[j]] = uint32(i)
		counts[j]++
	}

	// buckets por tamanho decrescente (maiores primeiro → tabela ainda vazia).
	order := make([]int32, 0, NB)
	for j := 0; j < int(NB); j++ {
		if counts[j] > starts[j] {
			order = append(order, int32(j))
		}
	}
	sort.Slice(order, func(a, b int) bool {
		return (counts[order[a]] - starts[order[a]]) > (counts[order[b]] - starts[order[b]])
	})

	G := make([]uint32, NB)
	c.words = make([]uint64, (uint64(M)+63)/64)
	const maxAttempts = 1024
	for attempt := 0; attempt < maxAttempts; attempt++ {
		c.seed2 = mix(seed + 1 + uint64(attempt)*0x9E3779B97F4A7C15)
		h2vals := make([]uint64, n)
		for i, k := range keys {
			h2vals[i] = c.h2(k)
		}
		for i := range G {
			G[i] = 0
		}
		for i := range c.words {
			c.words[i] = 0
		}
		if c.placeAll(keyAt, starts, counts, order, G, h2vals) {
			c.packG(G)
			c.buildRank()
			return c, nil
		}
	}
	return nil, ErrBuild
}

// placeAll posiciona os buckets (maiores primeiro). Para cada bucket, acha o
// deslocamento d tal que todos os slots caem livres E distintos entre si.
func (c *CHD) placeAll(keyAt []uint32, starts, ends []int32, order []int32, G []uint32, h2vals []uint64) bool {
	M := uint64(c.M)
	for _, jb := range order {
		j := int(jb)
		lo, hi := starts[j], ends[j]
		ok := false
		for d := uint64(0); d < maxDisplacement && !ok; d++ {
			good := true
			for a := lo; a < hi && good; a++ {
				posA := (h2vals[keyAt[a]] + d) % M
				if bitSet(c.words, posA) {
					good = false
					break
				}
				for b := lo; b < a; b++ {
					if (h2vals[keyAt[b]]+d)%M == posA {
						good = false
						break
					}
				}
			}
			if good {
				for a := lo; a < hi; a++ {
					setBit(c.words, (h2vals[keyAt[a]]+d)%M)
				}
				G[j] = uint32(d)
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// packG empacota os deslocamentos na largura mínima de bits.
func (c *CHD) packG(G []uint32) {
	var maxD uint32
	for _, d := range G {
		if d > maxD {
			maxD = d
		}
	}
	c.maxD = maxD
	w := bits.Len32(maxD)
	if w == 0 {
		w = 1
	}
	c.gw = uint8(w)
	nwords := (int(c.NB)*w+63)/64 + 1
	c.gpack = make([]uint64, nwords)
	for j := 0; j < int(c.NB); j++ {
		v := uint64(G[j])
		bp := uint64(j) * uint64(w)
		wi := bp >> 6
		off := bp & 63
		c.gpack[wi] |= v << off
		if off+uint64(w) > 64 {
			c.gpack[wi+1] |= v >> (64 - off)
		}
	}
}

// gval lê o deslocamento do bucket j.
func (c *CHD) gval(j uint64) uint64 {
	bp := j * uint64(c.gw)
	wi := bp >> 6
	off := bp & 63
	v := c.gpack[wi] >> off
	if off+uint64(c.gw) > 64 {
		v |= c.gpack[wi+1] << (64 - off)
	}
	return v & ((uint64(1) << c.gw) - 1)
}

// Lookup devolve o ID (0..N-1) da chave. ok=false se o slot está vazio.
func (c *CHD) Lookup(k uint64) (uint32, bool) {
	if c.M == 0 {
		return 0, false
	}
	j := c.h1(k)
	pos := (c.h2(k) + c.gval(j)) % uint64(c.M)
	if !bitSet(c.words, pos) {
		return 0, false
	}
	return c.rank(uint32(pos)), true
}

// buildRank monta o rank de 1 nível (bloco de 512 bits = 8 palavras).
func (c *CHD) buildRank() {
	nw := len(c.words)
	nb := (nw + 7) / 8
	c.l1 = make([]uint32, nb+1)
	var total uint32
	for b := 0; b < nb; b++ {
		c.l1[b] = total
		for w := b * 8; w < (b+1)*8 && w < nw; w++ {
			total += uint32(bits.OnesCount64(c.words[w]))
		}
	}
	c.l1[nb] = total
}

// rank devolve o número de bits setados antes de pos (inclusive) → ID.
func (c *CHD) rank(pos uint32) uint32 {
	w := pos >> 6
	off := pos & 63
	b := w >> 3
	r := c.l1[b]
	for i := b << 3; i < w; i++ {
		r += uint32(bits.OnesCount64(c.words[i]))
	}
	if off > 0 {
		r += uint32(bits.OnesCount64(c.words[w] & ((uint64(1) << off) - 1)))
	}
	return r
}

// BitsPerKey devolve o tamanho da estrutura em bits por chave.
func (c *CHD) BitsPerKey() float64 {
	if c.N == 0 {
		return 0
	}
	t := uint64(len(c.gpack)) * 64
	t += uint64(len(c.words)) * 64
	t += uint64(len(c.l1)) * 32
	return float64(t) / float64(c.N)
}

// Bytes devolve o tamanho total da estrutura em bytes.
func (c *CHD) Bytes() uint64 {
	return uint64(len(c.gpack))*8 + uint64(len(c.words))*8 + uint64(len(c.l1))*4
}

// MaxD devolve o maior deslocamento usado.
func (c *CHD) MaxD() uint32 { return c.maxD }

// GWidth devolve a largura em bits por deslocamento.
func (c *CHD) GWidth() uint8 { return c.gw }

func bitSet(w []uint64, i uint64) bool { return w[i>>6]&(1<<(i&63)) != 0 }
func setBit(w []uint64, i uint64)      { w[i>>6] |= 1 << (i & 63) }
