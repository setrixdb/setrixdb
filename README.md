# SetrixDB — the arithmetic set engine

_(formerly **ADDB** — Arithmetic Database)_

[![CI](https://github.com/setrixdb/setrixdb/actions/workflows/ci.yml/badge.svg)](https://github.com/setrixdb/setrixdb/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/setrixdb/setrixdb)](https://goreportcard.com/report/github.com/setrixdb/setrixdb)
[![Go Reference](https://pkg.go.dev/badge/github.com/setrixdb/setrixdb.svg)](https://pkg.go.dev/github.com/setrixdb/setrixdb)
[![Release](https://img.shields.io/github/v/release/setrixdb/setrixdb?include_prereleases&sort=semver)](https://github.com/setrixdb/setrixdb/releases)
[![Stars](https://img.shields.io/github/stars/setrixdb/setrixdb?style=flat)](https://github.com/setrixdb/setrixdb/stargazers)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

> **An in-memory, non-relational, non-vector, purely arithmetic set engine**, written in
> **Go (Golang)** and designed for **parallel execution on hardware accelerators**
> (NPUs, SIMD/AVX-512) on **low-power chipsets**.
>
> **It is not "yet another database":** it is an **engine** that **coexists** with your current
> database — it stores **sets of IDs** (not payloads) and answers membership and intersection in
> microseconds.

**Open source (Apache-2.0).**

- **Product pitch:** [`docs/APRESENTACAO-PRODUTO.md`](docs/APRESENTACAO-PRODUTO.md)
- **All measured results:** [`docs/RESULTADOS.md`](docs/RESULTADOS.md)
- **Test with real data (retail):** [`docs/RESULTADOS-DADOS-REAIS.md`](docs/RESULTADOS-DADOS-REAIS.md)

---

## 1. Goal

Build an **in-memory** database engine that:

- **is not relational** — no tables, joins or SQL;
- **is not vector** — no similarity search / embeddings;
- is **purely arithmetic** — all of the search "intelligence" reduces to
  comparisons and operations over integers (`uint64`), with no pointer
  indirection, no hashing and no pointer-based structures;

and that can **run in parallel on hardware accelerators**
(NPUs, SIMD/AVX-512) on **low-power chipsets** (edge / Edge AI).

The fundamental data unit is a **`uint64` ID**. The "database" is a **contiguous
memory shard** (`[]uint64`) that can be handed to hardware via
**zero-copy** (DMA).

## 2. Architecture overview

> **Measured result (Sep 14):** replacing the positional hash with an **MPHF (CHD)** that
> yields unique IDs, 50M terms → **0 collisions, 81.5 MiB, O(1) lookup in 113 ns**
> (~115,000× faster than a linear scan). See
> [`docs/RESULTADOS-MPHF.md`](docs/RESULTADOS-MPHF.md) and
> [`docs/REVISAO-ARQUITETURA.md`](docs/REVISAO-ARQUITETURA.md).

```
        ┌──────────────────────────────────────────────────────────────┐
        │                          SetrixDB Engine                     │
        │                                                              │
   batch│  ┌──────────────┐   ┌──────────────┐        ┌──────────────┐  │
  ──────┼─▶│  Partition   │──▶│ Parallel     │───────▶│  Merge /     │  │
 (uint64)│  │  (per worker)│   │ Search Kernel│        │  Dedup       │──┼──▶ matches
        │  └──────────────┘   └──────┬───────┘        └──────────────┘  │
        │                            │ purely arithmetic comparison    │
        │                            ▼                                 │
        │                 ┌──────────────────────┐                     │
        │                 │  Contiguous RAM shard│                     │
        │                 │  []uint64            │◀── DMA / unsafe.Ptr │
        │                 └──────────────────────┘                     │
        └──────────────────────────────────────────────────────────────┘
```

### Layers

| Layer | Role | Artifact |
|---|---|---|
| **Shard** | Contiguous block of IDs in RAM; the zero-copy foundation | `internal/addb/search.go` |
| **Arithmetic kernel** | Pure `uint64` comparison (branchless, vectorizable) | `internal/addb/search.go` |
| **Parallelizer** | Splits the query *batch* across workers | `internal/addb/search.go` |
| **DMA exposure** | `unsafe.Pointer` for C/C++ NPU drivers | `internal/addb/search.go` |
| **Routing** | Consistent Hash Ring by `uint64` across nodes | `internal/addb/ring.go` |

### Design principles

1. **Zero-copy by default.** No allocation/copy on the hot path; the shard
   is a contiguous slice handed straight to the accelerator.
2. **Pure arithmetic.** A match is an integer equality — auto-vectorizable
   by the compiler and mappable to SIMD lanes.
3. **Data parallelism.** Scales through *batching*, not expensive threads.
4. **Minimal energy.** Less memory movement ⇒ fewer joules per search.

## 3. Data model

- **ID:** `uint64` (key and value — the data *is* the number).
- **Shard:** contiguous `[]uint64`, immutable during a search.
- **Query (batch):** `[]uint64` — several IDs looked up in a single call.

## 4. Deterministic mapping + parallel search (example)

```go
package main

import (
	"fmt"

	"github.com/setrixdb/setrixdb/internal/addb"
)

func main() {
	// 1. Symbolic transmutation: UTF-8 term -> deterministic uint64 ID.
	idCasa := addb.ComputeDeterministicID("casa")
	idMoradia := addb.ComputeDeterministicID("moradia")
	idLar := addb.ComputeDeterministicID("lar")
	fmt.Printf("ID ('casa'): %d · ID ('moradia'): %d · ID ('lar'): %d\n", idCasa, idMoradia, idLar)

	// 2. Synonym base in flat arrays (no native maps).
	syn := addb.NewSynonymStorage([]addb.SynonymEntry{
		{Term: idCasa, Synonyms: []uint64{idMoradia, idLar}},
	})

	// 3. Search batch = term + synonyms, already as integers.
	searchBatch := append([]uint64{idCasa}, syn.Synonyms(idCasa)...)

	// 4. Contiguous memory shard in RAM.
	ramDatabaseShard := []uint64{idCasa, idMoradia, idLar, 99999999, 88888888}

	// 5. Parallel search.
	matches := addb.ParallelSearchEngine(searchBatch, ramDatabaseShard)
	fmt.Printf("Parallel search done. IDs found in RAM/NPU: %v\n", matches)
}
```

Full, runnable example: [`examples/busca_paralela/main.go`](examples/busca_paralela/main.go).

## 5. Guidelines for hardware and LLM integration

### Talking to an NPU / accelerator

Expose the `[]uint64` slice via `unsafe.Pointer` to the NPU's **C/C++** drivers,
enabling **DMA (Direct Memory Access)** transfers **with no allocation and no copy**
of Go memory.

```go
ptr := addb.UnsafePtr(shard) // *C.uint64_t ready for the NPU driver
```

> The caller **must** keep the shard alive during the transfer and must not let the
> slice be reallocated. The shard must be *pinned* when the driver requires it.

### Distributed topology

Implement routing via a **Consistent Hash Ring** using the **`uint64` ID itself**
to distribute **ultra-compact binary packets** over **UDP or gRPC**
between cluster nodes.

- `AddNode(n)` / `RemoveNode(n)` recompute the ring without a full re-hash.
- `Route(id)` returns the node that owns the key (position by *hash* on the ring).

### Primary use cases

- **Ultra-fast pre-filter for RAG pipelines** — cut candidates before spending
  expensive compute.
- **Long-term memory indexing for edge LLMs (Edge AI)** — IDs of facts/tokens in
  RAM, equality search in microseconds.
- **Token deduplication** with **minimal energy consumption**.

## 6. Repository structure

```
.
├── README.md                     ← this document
├── go.mod
├── setrixdb.go                   ← public Go API (package setrixdb)
├── docs/
│   ├── ESPECIFICACAO.md          ← original specification
│   └── ARQUITETURA.md            ← technical deep dive
├── internal/
│   ├── addb/                     ← deterministic ID, synonym storage, kernel, ring
│   ├── mphf/                     ← MPHF (CHD) — unique IDs, 0 collisions
│   ├── simd/                     ← AVX-512 kernel (cgo) + portable scalar fallback
│   └── cluster/                  ← sharding, cluster, binary protocol
├── cmd/                          ← CLI, HTTP server, C ABI, benchmarks
│   ├── setrixdb/                 ← CLI (build/info/has/intersect/bench)
│   ├── setrixdb-server/          ← HTTP/JSON server
│   └── setrixdb-capi/            ← C ABI (c-shared)
├── examples/                     ← runnable examples (busca_paralela, capi)
└── bench/                        ← `go test -bench` benchmarks
```

## 7. Performance test (ops/second)

There are two ways to measure:

```bash
# 1) Executable (prints ops/s immediately)
go run ./cmd/benchmark

# with explicit parameters (shard of 65,536 IDs, batch of 512, 1,000 searches)
go run ./cmd/benchmark -shard 65536 -batch 512 -iters 1000

# 2) Idiomatic Go benchmark
go test -bench=. -benchmem ./bench/
```

The benchmark measures **searches per second** (`searches/s`) and **queries per second**
(`queries/s`) over a synthetic shard.

## 8. Roadmap

- [x] Native SIMD kernel (AVX-512) via cgo/intrinsics.
- [x] Consistent ring with incremental *rebalancing*.
- [x] Binary protocol between nodes (zero-copy TCP + stored sets).
- [ ] NPU driver backend (DMA + shard *pinning*).
- [ ] Compact UDP protocol between nodes.
- [ ] Energy benchmarks (J/search) on SBCs.
- [ ] At-scale tests in the cloud (real multi-node cluster).

## 9. CLI

```bash
go build -o setrixdb ./cmd/setrixdb

# build a set from IDs (one per line, or CSV)
./setrixdb build -o catalog.sxset -input ids.txt

# info / membership
./setrixdb info catalog.sxset
./setrixdb has  catalog.sxset 12345 67890

# intersection of N sets (the "AND" of a faceted query)
./setrixdb intersect color_red.sxset size_m.sxset brand_x.sxset in_stock.sxset --bench 500
```

Real example (5 million products): intersection of **4 sets** (color, size, brand, stock)
→ **4.1 ms** on the first run. The `.sxset` format is binary and portable.

## 10. Embeddable API (Go)

```go
import "github.com/setrixdb/setrixdb"

red  := setrixdb.NewSet(1, 2, 3, 4, 5)
sizeM := setrixdb.NewSet(3, 4, 6)

res := setrixdb.Intersect(red, sizeM) // {3, 4}
res.Len()   // 2
res.Has(3)  // true
```

Also available: `Index` (MPHF — dense ID), `Union`, `Filter`. The public surface is the **root
package** `setrixdb`; packages under `internal/` are **not** importable by design.

## 11. Remote API (HTTP/JSON server)

```bash
go run ./cmd/setrixdb-server -addr :8080

curl -X PUT localhost:8080/sets/a -d '{"ids":[1,2,3]}'
curl -X PUT localhost:8080/sets/b -d '{"ids":[3,4,5]}'
curl -X POST localhost:8080/intersect -d '{"sets":["a","b"]}'
# {"count":1,"sets":["a","b"]}
```

Routes: `/health`, `/sets`, `PUT|GET|DELETE /sets/{name}`, `GET /sets/{name}/has?id=`, `POST /intersect`,
`POST /union`. Accepts JSON (`{"ids":[...]}`) or plain text (one ID per line).

**Set persistence:** start with `-data ./data` and each set is written to `./data/<name>.sxset`
(reloaded on boot). We persist **sets of IDs** — not payloads — keeping SetrixDB as an
**engine/index**.

## 12. C ABI (FFI) — embed in C/C++/Rust/Python

```bash
CGO_ENABLED=1 go build -buildmode=c-shared -o libsetrixdb.so ./cmd/setrixdb-capi
# produces libsetrixdb.so + libsetrixdb.h
```

Interface (handle = integer; arrays returned via `malloc`, free with `sx_free`):

```c
char*      sx_version();
long long  sx_set_new();
int        sx_set_add_many(long long h, uint64_t* ids, long long n);
long long  sx_set_len(long long h);
int        sx_set_has(long long h, uint64_t id);
long long  sx_intersect_many(long long a, long long b);
int        sx_intersect_ids(long long a, long long b, uint64_t** out, long long* n);
int        sx_set_free(long long h);
void       sx_free(void* p);
```

Ready-made examples in [`examples/capi/`](examples/capi) — tested from **C** (gcc) and **Python** (ctypes).

## 13. Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) and [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

## 14. License

Licensed under the **Apache License 2.0** — see [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).

Project contact: **contato@setrixdb.com**.
Copyright 2026 SetrixDB.
