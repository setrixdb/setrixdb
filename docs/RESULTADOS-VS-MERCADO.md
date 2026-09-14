# Head-to-head — ADDB vs soluções de mercado

> Medido nesta VPS (AMD EPYC 9J45 / Zen4, 2 vCPU) em 2026-09-14, Go 1.22.12.
> Código: `cmd/vsbench`. Dependências **só do benchmark** (Roaring bitmap, Bloom).

## Membership (n = 1.000.000 chaves uint64)

| Estrutura | memória | lookup | exato? |
|---|---|---|---|
| `map[uint64]struct{}` (Go) | 22,3 B/chave | **105,9M ops/s** | sim |
| **MPHF (CHD) + 1 verificação** (ADDB) | **9,9 B/chave** | 96,1M ops/s | **sim** |
| Bloom filter (1% FP) | 1,2 B/chave | 22,9M ops/s | **não** (aprox.) |

**Leitura:** o MPHF é **competitivo em velocidade** (~paridade com o `map`) e usa **~2,2× menos memória**, com membership **exata** (e devolve um ID). Um Bloom gasta **~8× menos**, mas é **aproximado** e não dá ID.

## Interseção de dois conjuntos (A = B = 1M)

| Cenário | sorted merge (ADDB) | Roaring | hash join (map) |
|---|---|---|---|
| **denso32 (realista)** | 8,0 ms | **174 µs** ✅ | 81,6 ms |
| **aleat64 (pior caso)** | **9,5 ms** ✅ | 447 ms | 89,7 ms |

- **denso32** (IDs 0..2N, como os IDs do MPHF): **Roaring é ~46× mais rápido** que o nosso merge.
- **aleat64** (uint64 aleatórios): nosso merge **ganha** da Roaring64 (~47×) — mas é caso de nicho.

## Veredito (baseado em número)

1. **Membership: competitivo.** MPHF ≈ `map` em velocidade, com 2,2× menos memória e ID exato. Contra hash table, estamos **no páreo (não dominantes)**.
2. **Interseção: estamos ATRÁS.** Na carga realista (IDs densos), a **Roaring nos bate por ~46×** — porque usa **bitmaps comprimidos + SIMD**. Nosso merge é **escalar**.
3. **O caminho para ficar promissor de verdade:** (a) **kernel SIMD de interseção** (Lemire/AVX-512 — a CPU daqui **tem** AVX-512); (b) ou representar os shards como **bitmap**; (c) adotar o MPHF faz os IDs virarem **densos 32-bit**, o que ajuda **as duas** abordagens.

**Resposta curta à pergunta "é promissor?":** em **membership** sim; em **interseção**, ainda **não** — precisamos do SIMD pra alcançar/passar o estado da arte. É exatamente o próximo item do board.
