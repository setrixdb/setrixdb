// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package setrixdb

import (
	"fmt"

	"github.com/setrixdb/setrixdb/internal/addb"
	"github.com/setrixdb/setrixdb/internal/mphf"
)

// keyerMaxProbe é o limite da cadeia de desambiguação usada quando dois termos
// distintos caem no mesmo valor do hash posicional.
const keyerMaxProbe = 16

// keyerSmallMax é o limite para usar mapa direto em vez de MPHF. A construção do
// CHD tem custo com picos (o build re-semeia até achar deslocamentos válidos) e
// para vocabulário minúsculo isso apareceria como "travar" no CLI; o mapa direto
// é O(1) e dá o mesmo contrato (IDs densos e exatos).
const keyerSmallMax = 4096

// keyerSalt é o passo do gerador de candidatos (constante de Weyl, 2^64/φ).
const keyerSalt uint64 = 0x9E3779B97F4A7C15

// Keyer mapeia **termos (string) → IDs (uint64) densos e sem colisão** entre os
// termos indexados.
//
// Como funciona: o hash posicional (addb.ComputeDeterministicID) sozinho **não é
// injetivo** — ele reduz uma string a um uint64 mod 2^64 e colide (medido: 78% em
// 200 mil tokens curtos). O Keyer resolve isso em duas camadas:
//
//  1. **MPHF (CHD)** sobre as chaves dos termos indexados → IDs em [0, n), sem
//     colisão entre termos distintos;
//  2. **conferência do termo** — o MPHF não guarda as chaves, então o termo é
//     comparado após o lookup. Isso elimina o falso-positivo do MPHF e devolve
//     `false` de forma exata para termos que não estão no índice.
//
// Termos distintos que caem no mesmo valor do hash posicional são desambiguados
// por uma cadeia determinística de candidatos (mesma ordem na construção e no
// lookup), então a injetividade não depende da sorte do hash.
//
// Vocabulário pequeno (até keyerSmallMax termos) usa mapa direto em vez do CHD —
// mesmo contrato (IDs densos 0..n-1, exatos), sem o custo de construção do MPHF.
//
// O Keyer é imutável depois de construído e seguro para leitura concorrente.
// Uso típico: indexar o vocabulário uma vez (catálogo de produtos, dicionário) e
// consultar em O(1) com ~13 bits por termo.
type Keyer struct {
	c        *mphf.CHD
	small    map[string]uint64 // caminho de vocabulário pequeno (sem MPHF)
	terms    []string
	maxProbe int
	colisoes int
}

// BuildKeyer constrói o índice perfeito dos termos dados.
//
// Termos repetidos são indexados uma única vez (o primeiro vence) — o mapeamento
// termo → ID é sempre 1:1. Devolve erro se a desambiguação estourar o limite de
// candidatos (na prática, só com vocabulário patológico).
func BuildKeyer(terms []string) (*Keyer, error) {
	k := &Keyer{}

	// Deduplica preservando a ordem de entrada.
	vistos := make(map[string]struct{}, len(terms))
	unicos := make([]string, 0, len(terms))
	for _, t := range terms {
		if _, dup := vistos[t]; dup {
			continue
		}
		vistos[t] = struct{}{}
		unicos = append(unicos, t)
	}

	n := len(unicos)
	if n == 0 {
		return k, nil
	}

	if n <= keyerSmallMax {
		k.small = make(map[string]uint64, n)
		k.terms = unicos
		for i, t := range unicos {
			k.small[t] = uint64(i)
		}
		return k, nil
	}

	taken := make(map[uint64]struct{}, n)
	keys := make([]uint64, n)

	for i, t := range unicos {
		base := addb.ComputeDeterministicID(t)
		probe := 0
		key := base
		for {
			if _, dup := taken[key]; !dup {
				break
			}
			probe++
			if probe > keyerMaxProbe {
				return nil, fmt.Errorf("setrixdb: desambiguação do termo %q estourou %d candidatos", t, keyerMaxProbe)
			}
			key = candidato(base, probe)
		}
		taken[key] = struct{}{}
		keys[i] = key
		if probe > k.maxProbe {
			k.maxProbe = probe
		}
		if probe > 0 {
			k.colisoes++
		}
	}

	c, err := mphf.Build(keys, 3.0, 0xADDB)
	if err != nil {
		return nil, fmt.Errorf("setrixdb: MPHF sobre os termos: %w", err)
	}
	k.c = c

	// O MPHF devolve IDs densos em [0, n) mas NÃO na ordem de inserção: o termo
	// de cada ID é montado consultando o índice (é ele que define o número).
	termosPorID := make([]string, n)
	for i, key := range keys {
		id, ok := c.Lookup(key)
		if !ok {
			return nil, fmt.Errorf("setrixdb: chave do termo %q sem ID no MPHF", unicos[i])
		}
		termosPorID[id] = unicos[i]
	}
	for id, termo := range termosPorID {
		if termo == "" {
			return nil, fmt.Errorf("setrixdb: ID %d ficou sem termo no índice", id)
		}
	}
	k.terms = termosPorID
	return k, nil
}

// candidato devolve o i-ésimo candidato de hash para um termo (i ≥ 1).
// Função pura: a mesma cadeia é reproduzida na construção e no lookup.
func candidato(base uint64, i int) uint64 {
	return mix64(base + uint64(i)*keyerSalt)
}

// mix64 é o finalizador (avalanche) usado nos candidatos de desambiguação.
func mix64(x uint64) uint64 {
	x ^= x >> 33
	x *= 0xff51afd7ed558ccd
	x ^= x >> 33
	x *= 0xc4ceb9fe1a85ec53
	x ^= x >> 33
	return x
}

// Len devolve o número de termos indexados.
func (k *Keyer) Len() int { return len(k.terms) }

// Colisoes devolve quantos termos precisaram de desambiguação na construção
// (dois termos distintos com o mesmo hash posicional). Serve de medida da
// qualidade do hash de origem: zero é o caso comum em vocabulário longo.
func (k *Keyer) Colisoes() int { return k.colisoes }

// BitsPerKey devolve o custo médio do índice em bits por termo (sem contar os
// termos guardados para conferência).
func (k *Keyer) BitsPerKey() float64 {
	if k.c == nil {
		return 0
	}
	return k.c.BitsPerKey()
}

// Bytes devolve o tamanho do índice perfeito em bytes (sem os termos).
func (k *Keyer) Bytes() uint64 {
	if k.c == nil {
		return 0
	}
	return k.c.Bytes()
}

// Lookup devolve o ID denso (0..Len()-1) do termo e se ele está indexado.
//
// Exato: um termo ausente devolve `false` (o MPHF sozinho poderia acusar
// falso-positivo; a conferência do termo elimina isso).
func (k *Keyer) Lookup(term string) (uint64, bool) {
	if k.small != nil {
		id, ok := k.small[term]
		return id, ok
	}
	if k.c == nil {
		return 0, false
	}
	base := addb.ComputeDeterministicID(term)
	for probe := 0; probe <= k.maxProbe; probe++ {
		key := base
		if probe > 0 {
			key = candidato(base, probe)
		}
		id, ok := k.c.Lookup(key)
		if !ok {
			continue
		}
		if int(id) < len(k.terms) && k.terms[id] == term {
			return uint64(id), true
		}
	}
	return 0, false
}

// Term devolve o termo do ID (inverso do Lookup). `false` se o ID não existe.
func (k *Keyer) Term(id uint64) (string, bool) {
	if id >= uint64(len(k.terms)) {
		return "", false
	}
	return k.terms[id], true
}

// IDSet monta o Set dos termos indexados presentes na lista dada. Termos
// ausentes do índice são ignorados (e reportados em `faltando`).
func (k *Keyer) IDSet(terms ...string) (set *Set, faltando []string) {
	ids := make([]uint64, 0, len(terms))
	for _, t := range terms {
		if id, ok := k.Lookup(t); ok {
			ids = append(ids, id)
		} else {
			faltando = append(faltando, t)
		}
	}
	return NewSet(ids...), faltando
}
