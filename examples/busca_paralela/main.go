// Copyright (c) 2026 SetrixDB
// SPDX-License-Identifier: Apache-2.0

// Command busca_paralela demonstra o SetrixDB ponta a ponta:
//
//	termo (string UTF-8) -> ID uint64 exato -> shard em RAM -> presença no conjunto.
//
// Nota: este exemplo é **didático**. Ele usa o kernel de varredura linear
// (addb.ParallelSearchEngine) para mostrar o laço branchless auto-vetorizável.
// Em produção a consulta é O(log n) ou O(n+m) — use setrixdb.Set / setrixdb.Intersect
// (busca binária + merge ordenado + bitset), que é o que o servidor usa.
package main

import (
	"fmt"

	"github.com/setrixdb/setrixdb"
	"github.com/setrixdb/setrixdb/internal/addb"
)

func main() {
	// 1. Termo (string UTF-8) -> ID uint64 exato.
	//
	// Use Keyer, não o hash posicional: o hash reduz a string a um uint64 mod 2^64
	// e pode dar o mesmo ID para termos distintos. O Keyer indexa o vocabulário com
	// MPHF (CHD) + conferência do termo, então 1 termo = 1 ID, sem colisão.
	keyer, err := setrixdb.BuildKeyer([]string{"casa", "moradia", "lar"})
	if err != nil {
		panic(err)
	}
	idCasa, _ := keyer.Lookup("casa")
	idMoradia, _ := keyer.Lookup("moradia")
	idLar, _ := keyer.Lookup("lar")

	fmt.Printf("ID exato ('casa'):    %d\n", idCasa)
	fmt.Printf("ID exato ('moradia'): %d\n", idMoradia)
	fmt.Printf("ID exato ('lar'):     %d\n\n", idLar)

	// 2. Base de sinônimos em flat arrays (vetores contínuos, sem maps nativos).
	syn := addb.NewSynonymStorage([]addb.SynonymEntry{
		{Term: idCasa, Synonyms: []uint64{idMoradia, idLar}},
	})

	// 3. Lote de busca = termo principal + sinônimos resolvidos em inteiros.
	searchBatch := append([]uint64{idCasa}, syn.Synonyms(idCasa)...)
	fmt.Printf("Lote de busca (termo + sinônimos): %v\n", searchBatch)

	// 4. Shard de memória contíguo em RAM.
	ramDatabaseShard := []uint64{idCasa, idMoradia, idLar, 99999999, 88888888}

	// 5. Presença no conjunto — caminho de produção (busca binária, O(log n)).
	set := setrixdb.NewSet(ramDatabaseShard...)
	fmt.Printf("Conjunto: %d IDs · 'casa' presente: %v\n", set.Len(), set.Has(idCasa))

	// 6. Demonstração didática do kernel de varredura linear (auto-vetorizável).
	matches := addb.ParallelSearchEngine(searchBatch, ramDatabaseShard)
	fmt.Printf("Kernel linear (didático) encontrou: %v\n", matches)

	// Ponteiro zero-copy para o driver da NPU (DMA).
	ptr := addb.UnsafePtr(ramDatabaseShard)
	fmt.Printf("Shard exposto para DMA em %p (%d IDs)\n", ptr, len(ramDatabaseShard))
}
