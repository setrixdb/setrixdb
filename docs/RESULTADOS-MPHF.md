# Resultados medidos — MPHF (CHD) para o dicionário do SetrixDB

> Números reais, medidos em servidor de referência em **2026-09-14**. Hardware: **2 vCPU** (AMD EPYC
> 9J45), Go **1.22.12**. Código: `internal/mphf/` (CHD escrito do zero) + `cmd/mphfbench`.
> Nada aqui é estimativa — é `go run`/`go test` executado.

## O que foi provado

O MPHF (CHD) construído sobre **50.000.000 de chaves**:

- **0 colisões** (por construção) — o requisito que o hash posicional da spec **não** cumpre (78% de colisão em tokens curtos).
- **13,68 bits/chave** = **81,5 MiB** para 50M (a spec previa ~400 MB só para os IDs).
- **build** em 20,6 s (2,4M chaves/s), pico de RAM **1,35 GB**.
- **lookup O(1)**: 113 ns/op (8,8M/s em 1 thread; 16,4M/s em 2 threads).

## Bits/chave por load factor (n = 1.000.000)

| λ | max displacement | largura/bucket | **bits/chave** |
|---|---|---|---|
| 0.90 | 833 | 10 bits | **12,57** |
| 0.95 | 4.383 | 13 bits | 15,07 |
| 0.99 | 33.426 | 16 bits | 17,49 |

> λ=0,90 venceu (deslocamentos menores ⇒ menos bits por bucket). Escolhido.

## Escala (λ = 0,90, empacotado)

| n | build | bits/chave | estrutura | lookup 1 thread |
|---|---|---|---|---|
| 1e6 | 0,1 s | 12,57 | 1,5 MiB | 13,0 ns/op (76,8M/s) |
| 2e6 | 0,4 s | 12,57 | 3,0 MiB | 13,0 ns/op (76,7M/s) |
| 5e7 | 20,6 s | 13,68 | 81,5 MiB | 113,0 ns/op (8,8M/s) |

> Efeito de cache é real e honesto: o lookup sai de ~13 ns (estrutura em cache) para
> ~113 ns a 50M (estrutura de 81 MiB não cabe em cache). Continua **O(1)**.

## Comparação direta (50 mi de termos)

| Estrutura | tamanho | colisão | lookup |
|---|---|---|---|
| **MPHF (CHD, este repo)** | **81,5 MiB** (13,68 b/k) | **0** | 113 ns (O(1)) |
| `[]uint64` cru (IDs da spec) | 400 MB (64 b/k) | 0 | — (precisa buscar) |
| `map[uint64]uint32` | ~1,56 GB (31,2 B/k) | 0 | 64,9 ns |
| hash posicional da spec | 400 MB | **78%** ❌ | — |
| **SCAN atual (`addb.Contains`)** | 0 (usa o shard) | 0 | **12.919.000 ns** (O(n)) |

**Ganho do MPHF vs scan linear: ~115.000×** por consulta (113 ns vs 12,9 ms).

## Veredito (baseado em número)

1. **O MPHF resolve o problema do ID único** que a spec tinha: 0 colisão, ID de 32 bits (50M < 2³²), e **18× menos memória** que `map` / **4,7× menos** que o array cru de `uint64`.
2. **MPHF não é filtro de membership.** Cerca de 90–95% das chaves de fora caem num slot ocupado (≈ load factor). Para membership exata, o dicionário **confere o termo** no índice devolvido — 1 comparação. Isso é o desenho correto (dicionário = MPHF + termos).
3. **A consulta do SetrixDB não deve ser varredura.** Mesmo com MPHF no ID, a busca por *shard* tem de ser por representação ordenada/bitmap — o scan segue sendo O(n).
4. **Custo/benefício do G:** a maior parte dos bits é o array de deslocamento (11 bits/bucket). Implementações de produção (BDZ/PTHash) chegam a 2–3 bits/chave comprimindo/evitando esse array inteiro — **próximo alvo de otimização**, se valer.

## Reproduzir

```bash
go test ./internal/mphf/                              # prova 0 colisão
go run ./cmd/mphfbench -n 1000000 -lambda 0.90        # 12,57 bits/chave
go run ./cmd/mphfbench -n 50000000 -lambda 0.90 -map=false   # 50M
```
