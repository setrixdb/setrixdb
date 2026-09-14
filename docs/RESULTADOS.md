# ADDB — Resultados consolidados (fonte do relatório final)

> Todos os testes e resultados, desde o início, num só lugar. Ambiente de medição:
> **Servidor de referência — 2 vCPU AMD EPYC 9J45 (Zen4, AVX-512), 3,8 GB RAM, Go 1.22.12 (+ gcc 11.4 cgo)**.
> Datas: 14/09/2026. Cada seção aponta o comando/arquivo que reproduz.

---

## 1. Hash posicional — colisão (o problema original)

A fórmula `Σ (UTF8(cᵢ)+i+1)·31ⁱ mod 2⁶⁴` **não é injetiva** (dígitos > base 31).

| Caso | Resultado |
|---|---|
| `"Oa"` vs `"0b"` | mesmo ID `3149` |
| `"AA"` vs U+085E | mesmo ID `2143` |
| 200.000 tokens alfanuméricos curtos | **78% de colisão** |

Arquivo: `internal/addb/hash_test.go` (`TestKnownCollision`, `TestCollisionRate`).

**Veredito:** o hash posicional, sozinho, **não serve** como ID único. Precisa de MPHF (ou hash + verificação).

---

## 2. MPHF (CHD) — ID único, 0 colisão

Implementação própria (sem lib de terceiros): `internal/mphf/`.

| Métrica | Valor |
|---|---|
| 50.000.000 chaves | **0 colisões** (por construção) |
| bits/chave (λ=0,90) | **13,68** (81,5 MiB) |
| build (50M) | 20,6 s (2,4M chaves/s), pico 1,35 GB |
| lookup (50M) | 113 ns/op (O(1)) |
| lookup (1M) | 13 ns/op |

bits/chave por load factor (n=1M): λ=0,90 → **12,57** · λ=0,95 → 15,07 · λ=0,99 → 17,49.

**Veredito:** resolve o "ID único"; cabe em `uint32` (50M < 2³²). Membership exata = MPHF + 1 comparação.

---

## 3. Camada de consulta (busca ordenada vs varredura)

`internal/addb/setops.go` + `cmd/setbench`.

| Estratégia | n=1M | n=10M |
|---|---|---|
| scan (O(n)) | 3.150 q/s | 300 q/s |
| sorted (O(log n)) | 8,3M q/s (**2.637×**) | 5,9M q/s (**19.644×**) |
| MPHF (O(1)) | 70,4M q/s (**22.346×**) | 66,4M q/s (**221.620×**) |

---

## 4. Head-to-head vs mercado — membership (n=1M)

`cmd/vsbench`.

| Estrutura | memória | lookup | exato? |
|---|---|---|---|
| `map[uint64]` (Go) | 22,3 B/chave | 133,3M ops/s | sim |
| **MPHF (CHD)** | **9,9 B/chave** | 100,4M ops/s | **sim** |
| Bloom (1% FP) | 1,2 B/chave | 23,6M ops/s | não |

**Veredito:** MPHF = paridade de velocidade com o `map`, **2,2× menos memória**, ID exato.

---

## 5. Head-to-head vs mercado — interseção (A = B = 1M)

`cmd/vsbench`.

| Estratégia | denso32 | aleat64 |
|---|---|---|
| sorted merge (ADDB) | 9,2 ms | 11,3 ms |
| Roaring (mercado) | 148 µs | 523 ms |
| hash join (map) | 91,6 ms | 94,9 ms |
| bitset AND (Go) | 29 µs | — |
| **bitset AND (AVX-512)** | **6 µs** | — |

**Veredito:** com IDs densos (via MPHF) + bitset, o AVX-512 bate o Roaring por **~24×**.

---

## 6. Kernel SIMD (AVX-512) + híbrido

`internal/simd/` (cgo). Instruções: `vpandq` + `vpopcntq`. **Dispatch em runtime** (AVX-512 se houver, senão escalar) — cobre AVX10.2.

- bitset AND AVX-512: **6 µs** (vs 148 µs Roaring) no denso32.
- **Extração dos elementos (`simd.ExtractSet`):** AVX-512 **1,58 ms** vs escalar **13,9 ms** (~8,8×) — 16K palavras, 1M bits setados.
- Portável: compila sem exigir AVX-512; escolhe a versão com `__builtin_cpu_supports`.

---

## 7. Universo ESPARSO (2²⁶ = 67M, A = B = 1M)

`cmd/sparsebench`.

| Estratégia | tempo | memória |
|---|---|---|
| **bitset AND (AVX-512)** | **303 µs** | 8,2 MB |
| bitset AND (Go) | 672 µs | 8,2 MB |
| Roaring64 | 9,26 ms | ~2,0 MB |
| sorted merge | 9,11 ms | — |

**Veredito:** o bitset+AVX-512 **também ganha no esparso** (desde que caiba em RAM). O crossover real é só quando o **universo não cabe em RAM** (2³² = 512 MB/set; 2³⁴ = 2 GB/set) — aí o Roaring (comprimido) vira.

---

## 8. Sharding (escala horizontal)

`internal/addb/shard.go` + `cmd/shardbench`. Universo **2³²**, 4M chaves, 2 CPUs.

| shards | mem total | intersec serial | intersec paralela |
|---|---|---|---|
| 4 | 536,9 MB | 148 ms | 28 ms |
| 16 | 536,9 MB | 65 ms | 46 ms |
| 64 | 536,9 MB | 58 ms | 26 ms |
| 256 | 536,9 MB | 52 ms | 28 ms |

**Veredito:** memória é **constante** (~537 MB = universo/8); o que escala é o **compute** (paraleliza por shard). Para universos maiores, a memória horizontal vem de **distribuir os ranges entre máquinas** + **roaring/híbrido por shard** (ver `docs/VISAO-ESCALABILIDADE.md`).

---

## 9. Geração por modelo local (Mac M5, qwen-code:9b)

Teste real: função `ExtractSet` (`internal/addb/extract.go`).

- Lógica **correta** ✅; **1 erro de tipo** (`uint64(i*64)+j`) → não compilava; corrigido 1 linha.
- `go test` PASS · `go vet` OK.
- ⚠️ Latência **~235 s** (provável Ollama sem Metal/GPU — a investigar).

---

## Resumo executivo

1. **ID único:** MPHF (CHD) → 0 colisão, ~13,7 bits/chave, lookup O(1).
2. **Membership:** paridade com `map`, 2,2× menos memória.
3. **Interseção:** bitset denso + AVX-512 → **líder** (24–30× vs Roaring), denso e esparso (enquanto couber).
4. **Escala:** sharding por range distribui o compute; memória horizontal = multi-máquina + híbrido bitset/roaring.
5. **Posicionamento mantido:** membership/interseção aritmética exata — **não** vira NoSQL nem vetorial.

_Reproduzível: `go test ./...` + `go run ./cmd/{setbench,vsbench,sparsebench,shardbench,mphfbench}`._
