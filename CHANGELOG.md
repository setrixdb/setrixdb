# Changelog

Todas as mudanças relevantes deste projeto são documentadas neste arquivo.

O formato segue o [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/)
e este projeto adere ao [Versionamento Semântico](https://semver.org/lang/pt-BR/).

## [Não lançado] — v1.1 (em andamento)

Integração no caminho do produto do que a v0.1.0 entregou apenas em benchmarks
(issue #16). Primeira fatia: **kernel SIMD no hot path**.

### Changed

- **`addb.Bitset.AndPopcount` passa a usar o kernel vetorizado** (AVX-512 com
  dispatch em runtime; escalar no fallback `CGO_ENABLED=0`/arm64) em vez do laço
  escalar em Go. É o caminho quente de `ShardedBitset.IntersectCount`.
- **`addb.ExtractSet` passa a usar o kernel de extração vetorizada** — antes a
  versão vetorizada era exercitada só por teste.
- **`setrixdb.Intersect` escolhe o caminho por custo** (`densePlan`): representação
  densa + AND vetorizado quando os conjuntos ocupam pouco universo; merge em
  listas ordenadas quando são esparsos. Mesmo resultado nos dois caminhos
  (teste de paridade), sem opção para o usuário configurar.

### Added

- `simd.And` (AND palavra a palavra) e `simd.Popcount`, ambos com dispatch
  AVX-512/escalar em runtime.
- `addb.Bitset.Intersect` / `addb.Bitset.Extract` e `addb.IntersectIDs`
  (interseção densa de N conjuntos).
- Benchmarks reproduzíveis em `bench/simd_path_bench_test.go` (merge x denso,
  kernel x escalar, API pública antes x depois).

### Fixed

- **`simd.ExtractSet` alocava 64 posições por palavra** (máximo teórico) e
  gastava mais na alocação que na extração: com isso o caminho denso chegava a
  ser mais lento que o merge. Agora dimensiona pelo popcount real.

### Medido

Máquina com AVX-512 real (AMD EPYC 9J45, 2 vCPU, container `golang:1.25`),
`go test -run XXX -bench ... -benchtime 300ms -count 3 ./bench/`:

| benchmark | antes | depois | ganho |
|---|---|---|---|
| `AndPopcount` 128 KB de palavras (escalar → kernel) | 8,9 µs | 1,45 µs | **6,1×** |
| `Intersect` API pública, dados densos (1 Mi de universo, 200 k IDs × 2) | 2,28 ms | 0,81 ms | **2,8×** |
| `Intersect` caminho denso isolado | 1,93 ms (merge) | 0,55 ms | **3,5×** |
| `Intersect` API pública, dados esparsos | 9,8 µs | 9,8 µs | igual (merge preservado) |

Ressalva registrada: o runner do GitHub Actions **não tem AVX-512**, então o CI
prova o caminho escalar/genérico; o tier vetorial é provado nesta máquina (e por
`TestKernelTier`, que loga o kernel efetivo).

## [0.1.0]

Primeira versão pública (**alpha / pre-release**). Ver as *release notes* para escopo, itens
não incluídos e *known issues*.

### Added

- **Engine / representações de índice**
  - MPHF (CHD, v2) próprio, para IDs únicos e densos — 0 colisão, ~4 bits/chave, lookup O(1).
  - Bitset denso (`internal/addb/bitset.go`).
  - SparseSet com merge *two-pointer* (`internal/addb/sparseset.go`), para universos esparsos.
  - Representação híbrida (faixas densas quentes + cauda esparsa) — memória proporcional aos dados, não ao universo.
- **Kernel SIMD** — AVX-512 (`vpandq` + `vpopcntq`) via cgo, com **fallback escalar portátil** (`internal/simd/`).
- **Sharding** — particionamento do espaço de IDs em ranges contíguos + interseção paralela por shard (`internal/addb/shard.go`).
- **Cluster** — nós por shard *over-the-wire* (TCP binário, zero-copy, sem dependências de terceiros) e modo **conjuntos armazenados** (`internal/cluster/`).
- **Consistent hash ring** — *placement* de nós/shard com rebalanceamento incremental (`internal/addb/ring.go`).
- **CLI** (`cmd/setrixdb`) — subcomandos `build`, `info`, `has`, `intersect`, `bench`, `id`.
- **API Go pública** — pacote raiz `setrixdb`: `NewSet`, `Set.Add/Len/Has/IDs`, `Intersect`, `Union`, `Filter`, `Index` (MPHF denso).
- **Servidor HTTP/JSON** (`cmd/setrixdb-server`) — rotas `/health`, `/sets`, `PUT|GET|DELETE /sets/{name}`, `GET /sets/{name}/has`, `POST /intersect`, `POST /union`; aceita JSON (`{"ids":[...]}`) ou texto puro (um ID por linha).
- **C ABI (FFI)** (`cmd/setrixdb-capi`) — gera `libsetrixdb.so` + `libsetrixdb.h` com a interface `sx_*`; exemplos prontos em **C** e **Python** (`examples/capi/`).
- **Persistência de conjuntos `.sxset`** — formato binário portátil, little-endian (`persist.go`).
- **Governança e licença** — `LICENSE` (Apache-2.0), `NOTICE`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `AUTHORS.md`, `.mailmap` e cabeçalhos **SPDX** em todo o código-fonte.

### Changed

- Projeto renomeado de **ADDB (Arithmetic Database)** para **SetrixDB**.
- Camada de identificação: do *hash* posicional (não-injetivo e sujeito a colisão) para **MPHF (CHD)**, para conjuntos densos. O `ComputeDeterministicID` permanece disponível para IDs efêmeros/streaming.
- MPHF evoluído para **v2**, separando *buckets* (`n/λ`) da tabela (`n·(1+ε)`) — ~3,4× menos memória que a v1, no mesmo patamar de lookup.

### Fixed

- **Portabilidade do kernel SIMD:** compila sem exigir AVX-512 e faz **dispatch em runtime** (`__builtin_cpu_supports`) — usa AVX-512 quando disponível e cai para o escalar caso contrário (cobre CPUs sem AVX-512 e AVX10.2).
- **Validação na leitura de `.sxset`:** checagem de *magic*/versão do formato e da ordem crescente dos IDs; arquivos inválidos são rejeitados em vez de aceitos silenciosamente.
