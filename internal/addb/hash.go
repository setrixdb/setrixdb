// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

// PrimeBase é a constante base do hashing posicional polinomial.
const PrimeBase uint64 = 31

// ComputeDeterministicID calcula o ID numérico escalar (uint64) de uma string
// UTF-8 pelo mapeamento posicional determinístico do SetrixDB:
//
//	ID = Σ_{i=0}^{L-1} ( UTF8(c_i) + i + 1 ) · B^i   (mod 2^64),  B = PrimeBase
//
// Garante a distinção entre anagramas (ex.: "casa" vs "saca") em tempo O(L),
// eliminando o processamento de strings em tempo de execução
// (transmutação simbólica: termo -> inteiro primitivo).
//
// Observação: em Go, `range` sobre string itera por RUNE, mas o índice `i` é o
// deslocamento em BYTES. A fórmula acima usa esse índice posicional do byte.
func ComputeDeterministicID(term string) uint64 {
	var hash uint64 = 0
	var currentPower uint64 = 1

	for i, runeValue := range term {
		// Combina valor UTF-8 da letra com a posição (i+1).
		charPosValue := uint64(runeValue) + uint64(i+1)
		hash += charPosValue * currentPower
		currentPower *= PrimeBase
	}
	return hash
}
