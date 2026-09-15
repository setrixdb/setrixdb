# SetrixDB — Arithmetic Database

_(antigo **ADDB** — Arithmetic Database)_

[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8.svg)](https://go.dev)
[![Status](https://img.shields.io/badge/status-proof--of--concept-orange.svg)](#)

> **Motor de conjuntos em memória, não-relacional, não-vetorial e puramente
> aritmético**, escrito em **Go (Golang)** e projetado para **execução paralela em
> aceleradores de hardware** (NPUs, SIMD/AVX-512) em chipsets de baixo consumo.
>
> **Não é "mais um banco":** é um **engine** que **coexiste** com o seu banco atual — ele guarda
> **conjuntos de IDs** (não payloads) e responde presença e interseção em microssegundos.

Projeto **pessoal** do Tião, **open source sob Apache-2.0**.

- **Apresentação / pitch:** [`docs/APRESENTACAO-PRODUTO.md`](docs/APRESENTACAO-PRODUTO.md)
- **Todos os resultados medidos:** [`docs/RESULTADOS.md`](docs/RESULTADOS.md)

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

> **Resultado medido (14/09):** trocando o hash posicional por um **MPHF (CHD)** com
> IDs únicos, 50 mi de termos → **0 colisão, 81,5 MiB, lookup O(1) em 113 ns**
> (~115.000× mais rápido que a varredura linear). Ver
> [`docs/RESULTADOS-MPHF.md`](docs/RESULTADOS-MPHF.md) e
> [`docs/REVISAO-ARQUITETURA.md`](docs/REVISAO-ARQUITETURA.md).

```
        ┌──────────────────────────────────────────────────────────────┐
        │                          SetrixDB Engine                          │
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

## 4. Mapeamento determinístico + busca paralela (exemplo)

```go
package main

import (
	"fmt"

	"github.com/setrixdb/setrixdb/internal/addb"
)

func main() {
	// 1. Transmutação simbólica: termo UTF-8 -> ID uint64 determinístico.
	idCasa := addb.ComputeDeterministicID("casa")
	idMoradia := addb.ComputeDeterministicID("moradia")
	idLar := addb.ComputeDeterministicID("lar")
	fmt.Printf("ID ('casa'): %d · ID ('moradia'): %d · ID ('lar'): %d\n", idCasa, idMoradia, idLar)

	// 2. Base de sinônimos em flat arrays (sem maps nativos).
	syn := addb.NewSynonymStorage([]addb.SynonymEntry{
		{Term: idCasa, Synonyms: []uint64{idMoradia, idLar}},
	})

	// 3. Lote de busca = termo + sinônimos, já em inteiros.
	searchBatch := append([]uint64{idCasa}, syn.Synonyms(idCasa)...)

	// 4. Shard de memória contíguo em RAM.
	ramDatabaseShard := []uint64{idCasa, idMoradia, idLar, 99999999, 88888888}

	// 5. Busca paralela.
	matches := addb.ParallelSearchEngine(searchBatch, ramDatabaseShard)
	fmt.Printf("Busca paralela concluída. IDs encontrados em RAM/NPU: %v\n", matches)
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
│   ├── hash.go                   ← ComputeDeterministicID (termo UTF-8 -> uint64)
│   ├── synonyms.go               ← FlatSynonymStorage (flat arrays + offsets)
│   ├── search.go                 ← kernel aritmético + busca paralela + DMA
│   └── ring.go                   ← consistent hash ring
├── internal/mphf/                ← MPHF (CHD) — ID único, 0 colisão
├── cmd/mphfbench/                ← mede bits/chave, colisões e ops/s do MPHF
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

## 8. Roadmap

- [x] Kernel SIMD nativo (AVX-512) via cgo/intrínsecos.
- [x] Anel de consistência com *rebalancing* incremental.
- [x] Protocolo binário entre nós (TCP zero-copy + conjuntos armazenados).
- [ ] Backend de driver de NPU (DMA + *pinning* de shard).
- [ ] Protocolo UDP compacto entre nós.
- [ ] Benchmarks de energia (J/busca) em SBC.
- [ ] Testes de escala em nuvem (cluster real multi-nó).

## 9. Contribuindo

Veja [`CONTRIBUTING.md`](CONTRIBUTING.md) e [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

## 10. Licença

Licenciado sob a **Apache License 2.0** — veja [`LICENSE`](LICENSE) e [`NOTICE`](NOTICE).
Copyright 2026 Thiago Silva.

---

_Especificação de trabalho do Tião · documentação mantida por Equipe ✨._
