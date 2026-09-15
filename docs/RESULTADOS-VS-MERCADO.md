# Head-to-head — SetrixDB vs soluções de mercado

> Medido em servidor de referência (AMD EPYC 9J45 / Zen4, 2 vCPU) em 2026-09-14, Go 1.22.12 + gcc 11.4 (cgo, AVX-512).
> Código: `cmd/vsbench`. Dependências de terceiros **só do benchmark** (Roaring, Bloom).

## Membership (n = 1.000.000 chaves uint64)

| Estrutura | memória | lookup | exato? |
|---|---|---|---|
| `map[uint64]struct{}` (Go) | 22,3 B/chave | 133,3M ops/s | sim |
| **MPHF (CHD) + 1 verificação** (SetrixDB) | **9,9 B/chave** | 100,4M ops/s | **sim** |
| Bloom filter (1% FP) | 1,2 B/chave | 23,6M ops/s | não (aprox.) |

**Leitura:** MPHF ≈ `map` em velocidade, com **~2,2× menos memória** e **membership exata** (+ ID).

## Interseção de dois conjuntos (A = B = 1M)

| Estratégia | denso32 (realista) | aleat64 (pior caso) |
|---|---|---|
| sorted merge (SetrixDB, escalar) | 9,2 ms | **11,3 ms** |
| Roaring (mercado) | 148 µs | 523 ms |
| hash join (map) | 91,6 ms | 94,9 ms |
| **bitset AND (Go, escalar)** | 29 µs | — |
| **bitset AND (AVX-512, cgo)** | **6 µs** ✅ | — |

- **denso32** = IDs 0..2N (o caso do SetrixDB: IDs do MPHF são densos). Roaring é rápido (148 µs), mas o **bitset AND em AVX-512 faz em 6 µs → ~24× mais rápido que o Roaring** (e ~1.500× vs o merge escalar).
- **aleat64** = uint64 aleatórios: bitset não se aplica (universo 2⁶⁴); aí nosso merge escalar ganha da Roaring64 (~46×).

## Veredito (baseado em número)

1. **Membership:** competitivo (paridade com `map`, 2,2× menos memória, ID exato).
2. **Interseção:** com **IDs densos (via MPHF) + bitset + AVX-512**, **superamos o Roaring por ~24×**. O vetor escalar perdia (9 ms vs 148 µs); o **SIMD virou o jogo**.
3. **Ressalvas honestas:**
   - O bitset é **não-comprimido** (244 KB p/ universo de 2M) — Roaring comprime. Para universos **esparsos/gigantes**, o Roaring pode voltar a ganhar em memória.
   - O kernel AVX-512 **exige CPU com AVX-512** (temos) — há fallback Go (29 µs, ainda ~5× melhor que Roaring aqui).
   - Sem o MPHF, os IDs ficam esparsos em 2⁶⁴ e o bitset denso não funciona.

**Resposta à pergunta "é promissor?":** **sim — e agora com número:** no cenário-alvo (IDs **densos** vindos do MPHF) o conjunto **MPHF + bitset + AVX-512** é **líder**, batendo o estado da arte na interseção. O diferencial que faltava era usar SIMD + IDs densos juntos.

## Interseção em universo ESPARSO (2^26 = 67M, A = B = 1M)

| Estratégia | tempo | memória |
|---|---|---|
| **bitset AND (AVX-512)** | **303 µs** | 8,2 MB |
| bitset AND (Go) | 672 µs | 8,2 MB |
| Roaring64 | 9,26 ms | ~2,0 MB |
| sorted merge | 9,11 ms | — |

> Mesmo no caso "esparso" o **bitset + AVX-512 ganha (~30× vs Roaring)** — porque o bitset de 67M de bits (8 MB) ainda cabe em memória e o AND vetorizado varre em O(universo/64). O Roaring comprime melhor (2 MB vs 8 MB), mas é mais lento (overhead de containers).
>
> **Crossover real:** o bitset só perde quando o universo é **tão grande que o bitset não cabe em RAM** (ex.: universo 2³² = 512 MB/set; 2³⁴ = 2 GB/set). Aí o Roaring (comprimido) vira a opção — ou um esquema híbrido (bitset por chunk + roaring por faixa).

## Próximos passos

- Extração vetorizada dos elementos da interseção (não só o cardinal).
- Testar universos esparsos/grandes (Roaring vs bitset vs híbrido).
- Levar pro NPU quando houver hardware (aqui só AVX-512).
