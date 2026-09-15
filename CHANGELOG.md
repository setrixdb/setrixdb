# Changelog

Todas as mudanças relevantes deste projeto são documentadas neste arquivo.

O formato segue o [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/)
e este projeto adere ao [Versionamento Semântico](https://semver.org/lang/pt-BR/).

## [0.1.0]

Primeira versão pública (**alpha / pre-release**). Ver
`notes/RELEASE-NOTES-v0.1.0.md` para escopo, itens não inclusos e *known issues*.

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
