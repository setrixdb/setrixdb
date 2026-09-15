# Arquitetura — SetrixDB

Motor de conjuntos **aritmético** e **embarcável**: guarda **conjuntos de IDs (`uint64`)** e
responde operações de conjunto (interseção, união, diferença) com **exatidão** — **coexistindo**
com o banco atual (não o substitui). Não é um banco de propósito geral.

**Pipeline:** `termo/texto → keygen (ID) → conjunto → operação (kernel) → IDs`.

## 1. Keygen — MPHF (CHD v2)

Cada termo UTF-8 vira um ID `uint64` **determinístico** por **Minimal Perfect Hash (CHD v2)**:
**0 colisões**, ~**4,03 bits/chave**, lookup `O(1)` (~118 ns). Código: `internal/mphf`.

> Nota histórica: o POC usava um *hash posicional* modular; a taxa de colisão medida foi alta
> (78%) e ele foi **substituído pelo MPHF**. Ver `docs/RESULTADOS.md`.

## 2. Representações de conjunto

`internal/addb`:

- **Bitset denso** — `universo/8` bytes; interseção pelo kernel SIMD (mais rápido).
- **SparseSet** — lista ordenada; ideal quando o universo é esparso.
- **Híbrido** — faixas quentes em bitset + cauda esparsa → memória ∝ **dados**, não ∝ universo.
- **ShardedBitset** — fatia por faixa de ID (paraleliza o *compute*).

## 3. Kernels SIMD

`internal/simd`: kernels em C (cgo) com **AVX-512** (`vpandq` + `vpopcntq`) e **dispatch em
runtime** (`__builtin_cpu_supports`), com **fallback escalar portável** (cobre CPUs sem AVX-512;
compatível com AVX10.2). Inclui AND+popcount e **extração vetorizada** dos elementos.

## 4. Operações

`internal/addb/setops.go`: `Contains` / `Intersect` / `Union` / `Difference`, com laços
**branchless** (sem `if` no caminho quente) — amigáveis ao auto-vectorizer.

## 5. Escala

- **Vertical:** paralelismo por shard (goroutines limitadas a `GOMAXPROCS`).
- **Horizontal (cluster):** `internal/cluster` — nós servem **shards** por **TCP binário**; o
  coordenador faz broadcast paralelo e soma o resultado. Dois modos:
  - **armazenado** — só os **nomes** dos conjuntos trafegam (zero dados por consulta);
  - **ad-hoc** — transmite a fatia da consulta a cada consulta.
- **Topologia dinâmica:** *consistent hash ring* (`internal/addb/ring.go`) — entrar/sair um nó
  remapeia ~`1/(N+1)` dos IDs.

## 6. Interfaces

- **Pacote Go** (raiz `github.com/setrixdb/setrixdb`): `Set` / `NewSet` / `Intersect` / `Union` /
  `Filter` / `Index`.
- **CLI** `cmd/setrixdb` (`version`, `build`, `info`, `has`, `intersect`, `bench`).
- **Servidor HTTP** `cmd/setrixdb-server` (`/sets`, `/intersect`, `/union`, …) com persistência
  de conjuntos (`.sxset`).
- **C ABI** `cmd/setrixdb-capi` (`libsetrixdb.so` + `libsetrixdb.h`).

## 7. Limites e riscos

- Igualdade **pura**: não responde a *range queries* nem a similaridade — é um **pré-filtro
  exato**, não um banco de propósito geral.
- O bitset denso custa `universo/8` bytes; o **híbrido/esparso** resolve quando o universo é grande.
- O keygen precisa ser **determinístico e estável** entre nós (mesmo mapeamento ID ↔ termo).

## 8. Números

Ver `docs/RESULTADOS.md` (13 seções). Resumo: MPHF ~4 bits/chave · interseção densa AVX-512
~4 µs (24× Roaring) · híbrido 1,7 MB vs 8,6 GB · cluster correto com **18–25×** no modo
armazenado e ponto de quebra documentado.
