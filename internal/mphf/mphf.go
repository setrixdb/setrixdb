// Package mphf implementa um Minimal Perfect Hash Function (MPHF) no estilo
// CHD (Czech–Havas–Majewski, 1997) escrito do zero — sem dependência de
// bibliotecas de terceiros (zero licença pendurada no repo).
//
// Propriedade do MPHF: dado um CONJUNTO FECHADO de N chaves, produz um mapa
// chave -> ID em [0, N) SEM COLISÃO. É o instrumento certo para o dicionário
// do ADDB (~50 mi de termos conhecidos).
//
// ATENÇÃO (semântica): o MPHF mapeia a chave para um ID, mas NÃO decide
// membership. Uma chave de fora cai num slot ocupado com probabilidade ≈ load
// factor. Para membership exata, o dicionário confere o termo armazenado no
// índice devolvido (uma comparação).
package mphf

import (
	"errors"
	"math/bits"
	"sort"
)

// CHD é um perfect hash construído sobre um conjunto fixo de chaves.
type CHD struct {
	N     uint32   // número de chaves
	M     uint32   // número de buckets/slots
	gpack []uint64 // deslocamentos (displacement) empacotados com gw bits cada
	gw    uint8    // largura em bits de cada deslocamento
	maxD  uint32   // maior deslocamento usado
	words []uint64 // bitset de slots ocupados
	l1    []uint32 // rank nível 1 (bloco de 512 bits)
	l2    []uint16 // rank nível 2 (palavra de 64 bits)
	seed1 uint64
	seed2 uint64
}

const maxDisplacement = 1<<16 - 1

// mix é o finalizador splitmix64 (avalanche forte e bijetivo).
func mix(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

func (c *CHD) h1(k uint64) uint64 { return mix(k^c.seed1) % uint64(c.M) }
func (c *CHD) h2(k uint64) uint64 { return mix(k^c.seed2) % uint64(c.M) }

// ErrBuild indica falha de construção mesmo após retries.
var ErrBuild = errors.New("mphf: construção falhou (reduza o load factor)")

// Build constrói o MPHF para o conjunto de chaves. lambda é o load factor
// alvo (chaves/slots); recomendado ~0.95.
func Build(keys []uint64, lambda float64, seed uint64) (*CHD, error) {
	n := len(keys)
	if n == 0 {
		return &CHD{N: 0, M: 0}, nil
	}
	M := uint32(float64(n)/lambda) + 1
	c := &CHD{N: uint32(n), M: M, seed1: seed}

	// 1. contagem por bucket (h1 não depende do sal de colocação).
	counts := make([]int32, M)
	for _, k := range keys {
		counts[c.h1(k)]++
	}
	starts := make([]int32, M)
	var s int32
	for j := 0; j < int(M); j++ {
		starts[j] = s
		s += counts[j]
	}
	keyAt := make([]uint32, n) // CSR: chaves agrupadas por bucket
	for j := 0; j < int(M); j++ {
		counts[j] = starts[j] // reusa como cursor
	}
	for i, k := range keys {
		j := c.h1(k)
		keyAt[counts[j]] = uint32(i)
		counts[j]++
	}

	// 2. buckets por tamanho decrescente.
	order := make([]int32, 0, M)
	for j := 0; j < int(M); j++ {
		if counts[j] > starts[j] {
			order = append(order, int32(j))
		}
	}
	sort.Slice(order, func(a, b int) bool {
		ja, jb := order[a], order[b]
		return (counts[ja] - starts[ja]) > (counts[jb] - starts[jb])
	})

	// 3. colocação com retries de sal (bucket com colisão de h2 é
	//    irrecuperável por deslocamento — retry resolve; ~padrão CHD).
	G := make([]uint16, M)
	c.words = make([]uint64, (uint64(M)+63)/64)
	const maxAttempts = 256
	for attempt := 0; attempt < maxAttempts; attempt++ {
		c.seed2 = mix(seed + 1 + uint64(attempt)*0x9E3779B97F4A7C15)
		for i := range G {
			G[i] = 0
		}
		for i := range c.words {
			c.words[i] = 0
		}
		if c.placeAll(keys, keyAt, starts, counts, order, G) {
			c.packG(G)
			c.buildRank()
			return c, nil
		}
	}
	return nil, ErrBuild
}

// placeAll tenta posicionar todos os buckets com o sal atual. true = sucesso.
func (c *CHD) placeAll(keys []uint64, keyAt []uint32, starts, ends []int32, order []int32, G []uint16) bool {
	M := uint64(c.M)
	for _, jb := range order {
		j := int(jb)
		lo, hi := starts[j], ends[j]
		ok := false
		for d := uint64(0); d < maxDisplacement && !ok; d++ {
			good := true
			for a := lo; a < hi && good; a++ {
				posA := (c.h2(keys[keyAt[a]]) + d) % M
				if bitSet(c.words, posA) {
					good = false
					break
				}
				for b := lo; b < a; b++ {
					if (c.h2(keys[keyAt[b]])+d)%M == posA {
						good = false
						break
					}
				}
			}
			if good {
				for a := lo; a < hi; a++ {
					setBit(c.words, (c.h2(keys[keyAt[a]])+d)%M)
				}
				G[j] = uint16(d)
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// packG empacota os deslocamentos na largura mínima de bits e descarta G.
func (c *CHD) packG(G []uint16) {
	var maxD uint16
	for _, d := range G {
		if d > maxD {
			maxD = d
		}
	}
	c.maxD = uint32(maxD)
	w := bits.Len16(maxD)
	if w == 0 {
		w = 1
	}
	c.gw = uint8(w)
	nwords := (int(c.M)*w+63)/64 + 1
	c.gpack = make([]uint64, nwords)
	for j := 0; j < int(c.M); j++ {
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

// Lookup devolve o ID (0..N-1) da chave. ok=false se a chave não pertence ao conjunto.
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

func (c *CHD) buildRank() {
	nw := len(c.words)
	nb := (nw + 7) / 8
	c.l1 = make([]uint32, nb+1)
	c.l2 = make([]uint16, nw+1)
	var total uint32
	for b := 0; b < nb; b++ {
		c.l1[b] = total
		var within uint32
		for w := 0; w < 8; w++ {
			idx := b*8 + w
			if idx >= nw {
				break
			}
			c.l2[idx] = uint16(within)
			within += uint32(bits.OnesCount64(c.words[idx]))
		}
		total += within
	}
	c.l1[nb] = total
}

func (c *CHD) rank(pos uint32) uint32 {
	w := pos >> 6
	off := pos & 63
	b := w >> 3
	r := c.l1[b] + uint32(c.l2[w])
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
	t += uint64(len(c.l2)) * 16
	t += uint64(len(c.l1)) * 32
	return float64(t) / float64(c.N)
}

// Bytes devolve o tamanho total da estrutura em bytes.
func (c *CHD) Bytes() uint64 {
	return uint64(len(c.gpack))*8 + uint64(len(c.words))*8 + uint64(len(c.l2))*2 + uint64(len(c.l1))*4
}

// MaxD devolve o maior deslocamento usado e a largura em bits por bucket.
func (c *CHD) MaxD() uint32 { return c.maxD }

// GWidth devolve a largura em bits usada por deslocamento na estrutura empacotada.
func (c *CHD) GWidth() uint8 { return c.gw }

func bitSet(w []uint64, i uint64) bool { return w[i>>6]&(1<<(i&63)) != 0 }
func setBit(w []uint64, i uint64)      { w[i>>6] |= 1 << (i & 63) }
