# Estrutura de código proposta — SetrixDB

> Antes de deployar, separar as camadas e **deixar os backends plugáveis** (scalar → SIMD
> → NPU). O POC atual está tudo em `internal/addb`; abaixo o alvo.

## Layout

```
addb/
├── cmd/
│   ├── addbd/                 # daemon (servidor de consultas + ring)
│   └── benchmark/             # mede ops/s (existe)
├── internal/
│   ├── keygen/                # termo -> ID (plugável) + política de colisão
│   │   ├── keygen.go          # interface KeyGen
│   │   ├── poly31.go          # hash posicional atual (com guarda de colisão)
│   │   └── xxhash.go          # alternativa robusta
│   ├── dict/                  # vocabulário: termo <-> ID
│   │   └── flat.go            # CSR: IDs contíguos + offsets
│   ├── store/                 # representações de shard
│   │   ├── shard.go           # Shard (uint64)
│   │   ├── sorted.go          # array ordenado (binária / merge SIMD)
│   │   └── bitmap.go          # Roaring / EWAH por faixa de ID
│   ├── query/                 # operações de conjunto (a "consulta")
│   │   └── setops.go          # Contains / Intersect / Union / Difference
│   ├── kernel/                # backends de execução (o "acelerador")
│   │   ├── kernel.go          # interface Kernel
│   │   ├── scalar.go          # puro Go (branchless)
│   │   ├── simd_amd64.s/.go   # AVX-512 via asm/cgo
│   │   └── npu/               # driver C/C++ + DMA (mmap/pin)
│   ├── synonyms/              # FlatSynonymStorage (CSR de sinonímia) — existe
│   ├── ring/                  # consistent hash ring — existe
│   └── transport/             # framing binário UDP/gRPC
├── pkg/addb/                  # API pública estável (opcional)
├── testdata/                  # corpora reais para teste de colisão
└── docs/
```

## Interfaces-chave (definir antes de codar)

```go
// KeyGen: transmutação termo -> ID. Determinístico; a colisão é tratada acima.
type KeyGen interface {
    ID(term string) uint64
    // Verify resolve colisão: o ID sozinho pode não bastar.
    ID128(term string) [2]uint64 // opcional, se exigir exatidão sem verificação
}

// Store: representação do shard (ordenado, bitmap, ...).
type Store interface {
    Kind() string                     // "sorted" | "bitmap" | "flat"
    Contains(id uint64) bool
    Intersect(query []uint64) []uint64
}

// Kernel: backend de execução (scalar | simd | npu). Mesmo contrato, ganhos diferentes.
type Kernel interface {
    Name() string
    Intersect(store Store, query []uint64) []uint64
}
```

## Regras de engenharia (para não repetir os erros)

1. **Nunca tratar o ID como único sem uma política de colisão explícita.** (ver
   `docs/REVISAO-ARQUITETURA.md`, item 1).
2. **Nada de varredura linear O(n) na consulta.** Membership/intersecção em representação
   **ordenada** ou **bitmap**.
3. **Contrato de memória** para DMA: buffers fora do GC (`mmap`/C.malloc) + `KeepAlive`.
4. **Interfaces antes de otimizar.** O POC pode ser scalar; SIMD/NPU entram atrás de
   `Kernel` sem reescrever a consulta.
5. **Testes de verdade:** property-based/fuzz para colisão (gerar N termos, contar
   colisões), golden tests de interseção, benchmarks por backend (ops/s e ns/op).

## Ordem de implementação sugerida

1. `keygen` + política de colisão + teste de colisão sobre corpus real (5–50 mi termos).
2. `store/sorted` + `query/setops` (interseção por merge) + kernel scalar.
3. Backend SIMD (AVX-512) do kernel de interseção + benchmark comparativo.
4. `transport` + `ring` (cluster) com protocolo binário compacto.
5. Backend NPU (DMA) — o último, e só se o SIMD já não bastar.
