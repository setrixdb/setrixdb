# ESPECIFICAÇÃO — ADDB (Arithmetic Database)

> **Prompt de Especificação Técnica: Motor de Busca e Banco de Dados Aritmético (ADDB)**
> Enviado pelo Tião. Transcrição fiel.

**Objetivo:** Implementar um motor de banco de dados **em memória**, **não-relacional**,
**não-vetorial** e **puramente aritmético** em **Go (Golang)**, projetado para
**execução paralela em aceleradores de hardware** (NPUs, SIMD/AVX-512) em chipsets de
**baixo consumo**.

---

## 1. Visão Geral da Arquitetura & Categoria

- **Categoria:** Banco de Dados **Aritmético-Determinístico em Memória**
  (*Deterministic Arithmetic In-Memory Database — ADDB*).
- **Conceito Chave:** **Transmutação simbólica imediata** de termos em **inteiros
  primitivos determinísticos** (`uint64`). Elimina o processamento de strings em tempo
  de execução, realizando buscas e cruzamento de sinônimos via **operações binárias e
  aritméticas** em **vetores contínuos de memória**.
- **Modelo de Execução:** Paralelismo massivo via **Goroutines** em Go, acoplado a
  instruções **SIMD/NPU** via arrays alinhados **zero-copy**.

## 2. Dimensionamento da Base de Dados (Footprint em RAM)

| Componente | Escala | Tamanho |
|---|---|---|
| Dicionário Global UTF-8 | ~50 milhões de termos (principais idiomas) | ~400 MB (IDs `uint64` contínuos) |
| Base de Sinônimos Relacional | ~50 milhões de conexões de sinonímia | ~460 MB (*Flat Arrays* + índices de offset `uint32`) |
| **Pegada Total** | — | **~1,0 GB de RAM** |

> Toda a linguagem humana e sinônimos contida em **RAM/cache**.

## 3. Algoritmo Fundamental: Mapeamento Posicional Determinístico

A função transforma qualquer string UTF-8 em um inteiro `uint64` único usando uma
**constante multiplicativa** e **deslocamento de bits** (*bitwise shift*), garantindo
**distinção entre anagramas** (ex.: `"casa"` vs `"saca"`) em tempo de execução **O(L)**,
onde `L` é o tamanho do termo.

```
ID = Σ ( UTF8(c_i) + i + 1 ) · B^i   (mod 2^64),   i = 0 … L-1
```

## 4. Algoritmo Principal e Busca Paralela (Código de Exemplo em Go)

```go
package main

import (
	"fmt"
	"sync"
)

// Constante base para o hashing posicional polinomial
const PrimeBase uint64 = 31

// ComputeDeterministicID calcula o ID numérico escalar de uma string UTF-8
func ComputeDeterministicID(term string) uint64 {
	var hash uint64 = 0
	var currentPower uint64 = 1

	for i, runeValue := range term {
		// Combina valor UTF-8 da letra com a posição (i+1)
		charPosValue := uint64(runeValue) + uint64(i+1)
		hash += charPosValue * currentPower
		currentPower *= PrimeBase
	}
	return hash
}

// FlatSynonymStorage armazena sinônimos em vetores contínuos de RAM sem usar Maps nativos
type FlatSynonymStorage struct {
	SynonymIDs []uint64 // Array contínuo contendo todos os IDs de sinônimos
	Offsets    []uint32 // Posição de início no array de sinônimos
	Lengths    []uint32 // Quantidade de sinônimos por termo
}

// ParallelSearchEngine orquestra a busca do termo e sinônimos simultaneamente
func ParallelSearchEngine(queryIDs []uint64, databaseShard []uint64) []uint64 {
	resultsChannel := make(chan uint64, len(queryIDs))
	var wg sync.WaitGroup

	// Dispara buscas paralelas simultâneas para o ID principal e todos os sinônimos
	for _, id := range queryIDs {
		wg.Add(1)
		go func(targetID uint64) {
			defer wg.Done()
			// Simulação de busca paralela no vetor de memória (mapeável para NPU/SIMD)
			for _, item := range databaseShard {
				if item == targetID {
					resultsChannel <- item
					return
				}
			}
		}(id)
	}

	wg.Wait()
	close(resultsChannel)

	matchedIDs := make([]uint64, 0)
	for res := range resultsChannel {
		matchedIDs = append(matchedIDs, res)
	}
	return matchedIDs
}

func main() {
	// 1. Mapeamento Determinístico
	idCasa := ComputeDeterministicID("casa")
	idMoradia := ComputeDeterministicID("moradia")
	idLar := ComputeDeterministicID("lar")

	fmt.Printf("ID Determinístico ('casa'): %d\n", idCasa)
	fmt.Printf("ID Determinístico ('moradia'): %d\n", idMoradia)
	fmt.Printf("ID Determinístico ('lar'): %d\n\n", idLar)

	// 2. Simulação de Lote de Busca (Termo + Sinônimos resolvidos em inteiros)
	searchBatch := []uint64{idCasa, idMoradia, idLar}

	// 3. Shard de Memória Contíguo em RAM
	ramDatabaseShard := []uint64{idCasa, idMoradia, idLar, 99999999, 88888888}

	// 4. Execução da Busca Paralela
	matches := ParallelSearchEngine(searchBatch, ramDatabaseShard)

	fmt.Printf("Busca paralela concluída. IDs encontrados em RAM/NPU: %v\n", matches)
}
```

## 5. Diretrizes para Integração com Hardware e LLMs

- **Comunicação com NPU/Acelerador:** Expor o slice `[]uint64` via `unsafe.Pointer`
  para os drivers **C/C++** da NPU, permitindo transferências **DMA (Direct Memory
  Access)** **sem alocação ou cópia** de memória em Go.
- **Topologia Distribuída:** Implementar roteamento por **Consistência Hash Ring**
  utilizando o **próprio ID `uint64`** para distribuir **pacotes binários
  ultra-compactos** via **UDP ou gRPC** entre nós do cluster.
- **Casos de Uso Primários:** Servir como **pré-filtro ultrarrápido** para pipelines
  de **RAG**, **indexação de memória de longo prazo** para **LLMs de borda (Edge AI)**
  e **deduplicação de tokens** com **consumo energético mínimo**.

### Próximos passos sugeridos pelo Tião

- [x] Exportar a especificação para um arquivo Markdown/README
- [x] Criar o teste de performance em Go para medir **ops/segundo**
