# ESPECIFICAÇÃO — ADDB (Arithmetic Database)

> Transcrição do prompt de especificação enviado pelo Tião (2026-09-14).
> **Nota:** na entrega original, as seções **1 (Visão Geral da Arquitetura)** e
> parte da **2** chegaram **truncadas** no canal. Este arquivo registra o que foi
> recebido; a versão reconstruída está em [`ARQUITETURA.md`](ARQUITETURA.md).

---

## Prompt de Especificação Técnica: Motor de Busca e Banco de Dados Aritmético (ADDB)

**Objetivo:** Implementar um motor de banco de dados **em memória**, **não-relacional**,
**não-vetorial** e **puramente aritmético** em **Go (Golang)**, projetado para
**execução paralela em aceleradores de hardware** (NPUs, SIMD/AVX-512) em chipsets de
**baixo consumo**.

### 1. Visão Geral da Arq…
*(seção recebida truncada)*

### 2–4. Trecho de referência (recebido)

```go
// 3. Shard de Memória Contíguo em RAM
ramDatabaseShard := []uint64{idCasa, idMoradia, idLar, 99999999, 88888888}

// 4. Execução da Busca Paralela
matches := ParallelSearchEngine(searchBatch, ramDatabaseShard)

fmt.Printf("Busca paralela concluída. IDs encontrados em RAM/NPU: %v\n", matches)
```

### 5. Diretrizes para Integração com Hardware e LLMs

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
