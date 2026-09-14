# Visão de Escalabilidade — ADDB

> Alinhamento de direção (14/09). Objetivo: **crescer e escalar** SEM virar NoSQL nem vetorial.

## 1. Posicionamento (o que NÃO somos)

- ❌ **Não é NoSQL** — sem key-value, documento, grafo ou relacional.
- ❌ **Não é vetorial** — sem embeddings, sem busca por similaridade.
- ✅ **É um índice aritmético-determinístico em memória**: membership **exata** + operações de
  conjunto (interseção/união) sobre IDs `uint64`.
- Papel: **pré-filtro / indexador de 1º estágio** — fica **na frente** de RAG, LLM de borda e
  deduplicação. O banco/DB (se houver) é outro sistema; o ADDB só responde "existe? quais batem?".

## 2. A unidade de escala: o Shard de IDs

O espaço de chaves é `uint64` (ou `uint32` denso vindo do MPHF). Escalar = **particionar o espaço
de IDs em ranges contíguos** e distribuir os ranges entre nós.

- Cada nó guarda **um range** como **bitset denso** (a estrutura que deu o ganho SIMD).
- **Contíguo de propósito** (e não hash-ring): mantém o bitset **denso** por nó — o hash-ring
  (do `ring.go`) fica para **placement de nós/items**, não para fatiar IDs.

## 3. Operações distribuídas

| Operação | Como escala |
|---|---|
| `membership(id)` | rota pelo range → **1 nó** responde (O(1) local) |
| `intersect(querySet)` | **broadcast** do conjunto → cada nó devolve seu subset → **merge** (soma de popcounts / união de listas) |

A interseção é **embarrassingly parallel**: latência = max(shards), throughput = soma.

## 4. Hierarquia de memória (escala o universo)

- Range **denso que cabe em RAM**: `bitset` + **AVX-512** (liderança medida).
- Range **gigante/esparso**: `roaring` (comprimido) por faixa — o crossover é quando o bitset
  não cabe mais em RAM (ex.: universo 2³²+).
- `MPHF` (com `G` comprimido) para o **dicionário termo → ID**.

## 5. Protocolo entre nós

- **Binário compacto**: ID `uint64` + payload mínimo.
- Transporte **UDP** (latência) ou **gRPC** (confiabilidade) — decidir pela carga.
- Interseção distribuída = um pacote com o `querySet` (bitset/sorted) → respostas em `uint64`.

## 6. Roadmap (grande e escalável, sem sair da tese)

1. **Shard local** — particionar em N shards no mesmo processo + interseção distribuída; medir.
2. **Over-the-wire** — gRPC/UDP entre nós (ring de placement) + protocolo binário.
3. **SIMD/NPU por shard** — o kernel vetorizado que já temos, replicado por nó.
4. **Híbrido por densidade** — bitset denso × roaring esparso, escolhido por range.

> Guarda-corpo: cada feature nova passa pelo filtro "isso ainda é membership/interseção
> aritmética exata?" Se vira KV, query range geral, ou similaridade → **não entra**.
