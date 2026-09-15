// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

package addb

// ExtractSet devolve, em ordem crescente, todas as posições de bit setadas (1)
// do bitset `words`. Gerado pelo modelo local qwen-code:9b do Mac (teste real).
func ExtractSet(words []uint64) []uint64 {
	var positions []uint64
	for i, word := range words {
		if word == 0 {
			continue
		}
		for j := uint(0); j < 64; j++ {
			if (word & (uint64(1) << j)) != 0 {
				positions = append(positions, uint64(i*64)+uint64(j))
			}
		}
	}
	return positions
}
