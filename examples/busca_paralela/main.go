// Command busca_paralela demonstra o ADDB ponta a ponta:
//
//	termo (string UTF-8) -> ID uint64 determinístico -> shard em RAM -> busca paralela.
package main

import (
	"fmt"

	"github.com/setrixdb/setrixdb/internal/addb"
)

func main() {
	// 1. Mapeamento posicional determinístico (string UTF-8 -> uint64).
	idCasa := addb.ComputeDeterministicID("casa")
	idMoradia := addb.ComputeDeterministicID("moradia")
	idLar := addb.ComputeDeterministicID("lar")

	fmt.Printf("ID Determinístico ('casa'):    %d\n", idCasa)
	fmt.Printf("ID Determinístico ('moradia'): %d\n", idMoradia)
	fmt.Printf("ID Determinístico ('lar'):     %d\n\n", idLar)

	// 2. Base de sinônimos em flat arrays (vetores contínuos, sem maps nativos).
	syn := addb.NewSynonymStorage([]addb.SynonymEntry{
		{Term: idCasa, Synonyms: []uint64{idMoradia, idLar}},
	})

	// 3. Lote de busca = termo principal + sinônimos resolvidos em inteiros.
	searchBatch := append([]uint64{idCasa}, syn.Synonyms(idCasa)...)
	fmt.Printf("Lote de busca (termo + sinônimos): %v\n", searchBatch)

	// 4. Shard de memória contíguo em RAM.
	ramDatabaseShard := []uint64{idCasa, idMoradia, idLar, 99999999, 88888888}

	// 5. Execução da busca paralela.
	matches := addb.ParallelSearchEngine(searchBatch, ramDatabaseShard)
	fmt.Printf("Busca paralela concluída. IDs encontrados em RAM/NPU: %v\n", matches)

	// Ponteiro zero-copy para o driver da NPU (DMA).
	ptr := addb.UnsafePtr(ramDatabaseShard)
	fmt.Printf("Shard exposto para DMA em %p (%d IDs)\n", ptr, len(ramDatabaseShard))
}
