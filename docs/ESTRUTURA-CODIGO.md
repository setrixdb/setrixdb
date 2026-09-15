# Estrutura de código — SetrixDB

## Layout atual

```
setrixdb/
├── setrixdb.go              # API pública (pacote raiz): Set, NewSet, Intersect, Union, Filter, Index
├── cmd/
│   ├── setrixdb/            # CLI (version, build, info, has, intersect, bench)
│   ├── setrixdb-server/     # servidor HTTP/JSON + persistência de conjuntos (.sxset)
│   ├── setrixdb-capi/       # C ABI → libsetrixdb.so + libsetrixdb.h
│   ├── clusternode/         # nó de shard (TCP binário)
│   ├── clusterdemo/         # coordenador (demonstração/carga)
│   └── *bench               # setbench, vsbench, sparsebench, shardbench, hybridbench,
│                            # mphfbench, ringbench, ringclusterdemo
├── internal/
│   ├── mphf/                # keygen — Minimal Perfect Hash (CHD v2)
│   ├── addb/                # conjuntos: shard, setops, sparseset, ring, extract, hash, synonyms
│   ├── simd/                # kernels AVX-512 (cgo) + dispatch em runtime
│   └── cluster/             # nós + coordenador over-the-wire, sharded
├── deploy/k8s/              # manifesto de cluster (StatefulSet + Service)
└── docs/
```

> `internal/addb` mantém o nome **histórico** do projeto (ADDB → SetrixDB). É um pacote **interno**
> — não aparece na API pública — então o nome não afeta consumidores.

## Interfaces-chave

```go
// API pública (pacote raiz) — superfície estável para consumidores Go.
type Set interface { /* Has, Len, ... */ }
func NewSet(ids []uint64) Set
func Intersect(a, b Set) Set
```

## Evolução prevista

- **Backends plugáveis** de kernel (**scalar → SIMD → NPU**) atrás de uma interface, sem reescrever
  a consulta.
- **Contrato de memória p/ DMA**: buffers fora do GC (`mmap`/C.malloc) + `KeepAlive`.
- **Transporte**: o framing binário já existe; avaliar gRPC/UDP para casos específicos.

## Regras de engenharia

1. **ID não é único sem política de colisão explícita** (ver `docs/REVISAO-ARQUITETURA.md`).
2. **Nada de varredura linear `O(n)`** na consulta — use representação ordenada ou bitmap.
3. **Testes de verdade**: fuzz de colisão, golden tests de interseção, benchmark por backend
   (ops/s e ns/op).
