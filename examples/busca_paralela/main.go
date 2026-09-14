// Command busca_paralela demonstra o ADDB: um shard de IDs em RAM e uma busca
// paralela aritmética sobre um batch de consultas.
package main

import (
	"fmt"

	"github.com/tgosoul2019/addb/internal/addb"
)

func main() {
	// 1-2. IDs de exemplo (domínio do problema).
	const (
		idCasa    uint64 = 1001
		idMoradia uint64 = 1002
		idLar     uint64 = 1003
	)

	// 3. Shard de memória contíguo em RAM.
	ramDatabaseShard := []uint64{idCasa, idMoradia, idLar, 99999999, 88888888}

	// Batch de consultas.
	searchBatch := []uint64{idCasa, 88888888, idLar, 12345678}

	// 4. Execução da busca paralela.
	matches := addb.ParallelSearchEngine(searchBatch, ramDatabaseShard)

	fmt.Printf("Busca paralela concluída. IDs encontrados em RAM/NPU: %v\n", matches)

	// Ponteiro de zero-copy para o driver da NPU (DMA).
	ptr := addb.UnsafePtr(ramDatabaseShard)
	fmt.Printf("Shard exposto para DMA em %p (%d IDs)\n", ptr, len(ramDatabaseShard))
}
