# ADDB — Arithmetic Database

> **Motor de banco de dados em memória, não-relacional, não-vetorial e puramente
> aritmético**, escrito em **Go (Golang)** e projetado para **execução paralela em
> aceleradores de hardware** (NPUs, SIMD/AVX-512) em chipsets de baixo consumo.

Projeto **pessoal** do Tião. Repositório de documentação + prova de conceito.

---

## 1. Objetivo

Implementar um motor de banco de dados **em memória** que:

- **não é relacional** — não há tabelas, joins nem SQL;
- **não é vetorial** — não faz busca por similaridade / embeddings;
- é **puramente aritmético** — toda a "inteligência" da busca reduz-se a
  comparações e operações sobre inteiros (`uint64`), sem ponteiros indiretos,
  sem hashing e sem estruturas de ponteiros;

e que seja capaz de **executar em paralelo em aceleradores de hardware**
(NPUs, SIMD/AVX-512) em **chipsets de baixo consumo** (borda / Edge AI).

A unidade fundamental de dados é um **ID `uint64`**. O "banco" é um **shard de
memória contíguo** (`[]uint64`) que pode ser entregue ao hardware por
**zero-copy** (DMA).

## 2. Visão geral da arquitetura

```
        ┌──────────────────────────────────────────────────────────────┐
        │                          ADDB Engine                          │
        │                                                              │
   batch│  ┌──────────────┐   ┌──────────────┐        ┌──────────────┐  │
  ──────┼─▶│  Partition   │──▶│ Parallel     │───────▶│  Merge /     │  │
 (uint64)│  │  (por worker)│   │ Search Kernel│        │  Dedup       │──┼──▶ matches
        │  └──────────────┘   └──────┬───────┘        └──────────────┘  │
        │                            │ comparação puramente aritmética │
        │                            ▼                                 │
        │                 ┌──────────────────────┐                     │
        │                 │  Shard de RAM        │                     │
        │                 │  contíguo []uint64   │◀── DMA / unsafe.Ptr │
        │                 └──────────────────────┘                     │
        └──────────────────────────────────────────────────────────────┘
```

### Camadas

| Camada | Papel | Artefato |
|---|---|---|
| **Shard** | Bloco contíguo de IDs em RAM; base do zero-copy | `internal/addb/search.go` |
| **Kernel aritmético** | Comparação pura `uint64` (branchless, vetorizável) | `internal/addb/search.go` |
| **Paralelizador** | Fatia o *batch* de consultas entre workers | `internal/addb/search.go` |
| **Exposição DMA** | `unsafe.Pointer` para drivers C/C++ da NPU | `internal/addb/search.go` |
| **Roteamento** | Consistent Hash Ring por `uint64` entre nós | `internal/addb/ring.go` |

### Princípios de projeto

1. **Zero-copy por padrão.** Nada de alocação/cópia no caminho quente; o shard
   é um slice contíguo repassado direto ao acelerador.
2. **Aritmética pura.** O casamento é uma igualdade de inteiros — auto-vetorizável
   pelo compilador e mapeável para lanes SIMD.
3. **Paralelismo por dados.** Escala por *batching*, não por threads caras.
4. **Energia mínima.** Menos movimentação de memória ⇒ menos joules por busca.

## 3. Modelo de dados

- **ID:** `uint64` (chave e valor — o dado *é* o número).
- **Shard:** `[]uint64` contíguo, imutável durante a busca.
- **Consulta (batch):** `[]uint64` — vários IDs buscados numa única chamada.

## 4. Busca paralela (exemplo)

```go
package main

import (
	"fmt"

	"github.com/tgosoul2019/addb/internal/addb"
)

func main() {
	// 1-2. IDs de exemplo (domínio do problema)
	const (
		idCasa    uint64 = 1001
		idMoradia uint64 = 1002
		idLar     uint64 = 1003
	)

	// 3. Shard de memória contíguo em RAM
	ramDatabaseShard := []uint64{idCasa, idMoradia, idLar, 99999999, 88888888}

	// 4. Execução da busca paralela
	searchBatch := []uint64{idCasa, 88888888, idLar, 12345678}
	matches := addb.ParallelSearchEngine(searchBatch, ramDatabaseShard)

	fmt.Printf("Busca paralela concluída. IDs encontrados em RAM/NPU: %v\n", matches)
	// → IDs encontrados em RAM/NPU: [1001 88888888 1003]
}
```

Exemplo completo e executável: [`examples/busca_paralela/main.go`](examples/busca_paralela/main.go).

## 5. Diretrizes para integração com hardware e LLMs

### Comunicação com NPU / acelerador

Expor o slice `[]uint64` via `unsafe.Pointer` para os drivers **C/C++** da NPU,
permitindo transferências **DMA (Direct Memory Access)** **sem alocação nem cópia**
de memória em Go.

```go
ptr := addb.UnsafePtr(shard) // *C.uint64_t pronto para o driver da NPU
```

> O chamador **deve** garantir que o shard continue vivo durante a transferência e
> que o slice não seja realocado. O shard deve ser *pinado* quando o driver exigir.

### Topologia distribuída

Implementar roteamento por **Consistent Hash Ring** usando o **próprio ID
`uint64`** para distribuir **pacotes binários ultra-compactos** via **UDP ou gRPC**
entre os nós do cluster.

- `AddNode(n)` / `RemoveNode(n)` recomputam o anel sem re-hash total.
- `Route(id)` devolve o nó dono da chave (posição por *hash* no anel).

### Casos de uso primários

- **Pré-filtro ultrarrápido para pipelines de RAG** — cortar candidatos antes de
  gastar compute caro.
- **Indexação de memória de longo prazo de LLMs de borda (Edge AI)** — IDs de
  fatos/tokens em RAM, busca por igualdade em microssegundos.
- **Deduplicação de tokens** com **consumo energético mínimo**.

## 6. Estrutura do repositório

```
addb/
├── README.md                     ← este documento
├── go.mod
├── docs/
│   ├── ESPECIFICACAO.md          ← prompt de especificação original
│   └── ARQUITETURA.md            ← detalhamento técnico
├── internal/addb/
│   ├── search.go                 ← kernel aritmético + busca paralela + DMA
│   └── ring.go                   ← consistent hash ring
├── examples/busca_paralela/main.go
├── cmd/benchmark/main.go         ← mede ops/segundo (executável)
└── bench/search_bench_test.go    ← benchmark `go test -bench`
```

## 7. Teste de performance (ops/segundo)

Existem duas formas de medir:

```bash
# 1) Executável (imprime ops/s na hora)
go run ./cmd/benchmark

# com parâmetros explícitos (shard 65.536 IDs, batch de 512, 1.000 buscas)
go run ./cmd/benchmark -shard 65536 -batch 512 -iters 1000

# 2) Benchmark idiomático do Go
go test -bench=. -benchmem ./bench/
```

O benchmark mede **buscas por segundo** (`buscas/s`) e **consultas por segundo**
(`consultas/s`) sobre um shard sintético.

## 8. Roadmap (rascunho)

- [ ] Kernel SIMD nativo (AVX-512) via cgo/intrínsecos.
- [ ] Backend de driver de NPU (DMA + *pinning* de shard).
- [ ] Anel de consistência com *rebalancing* incremental.
- [ ] Protocolo binário UDP compacto entre nós.
- [ ] Benchmarks de energia (J/busca) em SBC.

---

_Especificação de trabalho do Tião · documentação mantida por Equipe ✨._
