# Revisão de Arquitetura — SetrixDB

> Parecer honesto de **engenharia de software** e **engenharia de IA** sobre a spec do SetrixDB,
> pedido pelo Tião ("me responda honestamente"). Nada aqui é para agradar: é para não
> deployarmos uma arquitetura que não fecha.

## Veredito rápido

A **intuição central é boa** (representar termos como inteiros e trabalhar em arrays
contíguos), mas **a arquitetura, como especificada, não fecha** para os casos de uso
propostos. Há **dois problemas críticos** (ID não-injetivo e busca O(n) por consulta) que
invertem completamente a promessa de performance. A boa notícia: **mudando a camada de
consulta e a de representação**, a ideia vira algo sólido e até elegante.

---

## O que está genuinamente certo

1. **Transmutar termo → inteiro é padrão na indústria.** Interning de strings, hashing e
   tokenizadores fazem exatamente isso. Instinto correto.
2. **Flat arrays + offsets (`Offsets`/`Lengths`) é a estrutura certa.** Isso é **CSR**
   (*Compressed Sparse Row*) — a mesma base de índices invertidos, listas de adjacência de
   grafos e postings lists. **Esse é o melhor pedaço da spec.**
3. **Zero-copy para acelerador** é a direção certa para borda (DMA ao invés de copiar).
4. **Paralelismo de dados** (particionar o lote) é a forma correta de escalar.

---

## Problemas — por severidade

### 🔴 1. O ID **não é injetivo** (corrige o próprio conceito)

A fórmula `Σ (UTF8(cᵢ) + i + 1)·31^i mod 2⁶⁴` **não é uma bijeção**. Como os códigos de
caractere (≥ 65 em ASCII) são **maiores que a base 31**, os "dígitos" estouram a base e a
representação **deixa de ser canônica** — logo, strings diferentes colidem.

**Colisões reais (verificadas):**

```
ComputeDeterministicID("Oa")  = 3149
ComputeDeterministicID("0b")  = 3149   // "Oa" e "0b" → MESMO ID

ComputeDeterministicID("AA")     = 2143
ComputeDeterministicID("\u085e") = 2143  // mesma coisa
```

Isso **não é raro/birthday** — é estrutural: `a₁ − a₂ = 31·(b₂ − b₁)`. Um dicionário com
50 mi de termos **vai** colidir, e colisão num dicionário = **duas palavras viram a mesma
chave** = corrupção silenciosa.

> Para 50 mi de IDs *aleatórios* em 2⁶⁴, a chance de colisão por aniversário é ~0,007%.
> O problema aqui não é o espaço — **é a função não ser injetiva**.
>
> **Medido neste repo** (`TestCollisionRate`): corpus de **200.000 tokens
> alfanuméricos curtos** → **78% de colisão** (156.177 colisões). Não é teoria.

**Correção obrigatória (escolher uma):**
- **(a)** Deixar de vender como "ID único" e tratar como **hash**: guardar o termo (ou um
  hash 128 bits) junto e **verificar na colisão**. Exato, custo baixo.
- **(b)** Usar `(cᵢ mod (B−1))` para manter os dígitos < base → representação canônica
  (mas ainda limitada pelo comprimento que cabe em 64 bits).
- **(c)** Usar um hash não-cripto decente (**xxHash64 / FNV-1a 64**) e assumir o mesmo
  contrato "hash + verificação".

### 🔴 2. A busca é **O(n) por consulta** — o oposto da promessa

`ParallelSearchEngine` varre **o shard inteiro para cada ID do lote**. Com o alvo de
**50 mi de termos (~400 MB)**:

- uma consulta = ler 400 MB; 50 mi de consultas = **2×10¹⁶ bytes**;
- mesmo a ~20 GB/s de banda, dá **~20 ms por consulta**;
- SIMD melhora a **constante** (~8–16×), **não a complexidade** — e o gargalo aqui é
  **banda de memória**, não CPU. NPU não resolve banda.

Comparação honesta com o "concorrente" que a spec ignora:

| Operação | SetrixDB (scan) | `map[uint64]` / hash | Sorted + binária | Bitmap (roaring) |
|---|---|---|---|---|
| Membership | **O(n)** ❌ | O(1) ✅ | O(log n) ✅ | O(1)–O(n/64) ✅ |
| Interseção de conjuntos | O(n·m) ❌ | O(min) ✅ | **O(n+m) SIMD** ✅ | **O(n/64) rápido** ✅ |

**Correção obrigatória:** trocar varredura por **interseção de arrays ordenados com
merge SIMD** (galloping intersection) **ou Roaring/EWAH bitmaps**. É exatamente aí que
SIMD/NPU passa a render — e a *energia por op* despenca.

### 🟠 3. "Dicionário de 50 mi" sem as palavras (lacuna conceitual)

A spec conta **400 MB de IDs `uint64`** — mas `uint64` não guarda a palavra. Sem as
strings você tem só um **conjunto de hashes**: não dá para imprimir o termo, nem
desambiguar colisão, nem fazer stemming/morfologia. Se guardar as strings, o footprint
**estoura** o 1 GB com folga. Ou seja: **ou falta a informação, ou falta o orçamento.**

Reenquadrar: o que existe é um **grafo de sinonímia + conjunto de vocabulário**, não
"toda a linguagem humana". Nomear assim evita promessa que não se cumpre.

### 🟠 4. Zero-copy / DMA: o `unsafe.Pointer` sozinho não basta

DMA para NPU normalmente exige **memória pinada / fisicamente contígua** (page-locked,
hugepages, mapeamento via `mmap` + IOMMU). O heap do Go **não é pinável** e o GC pode
mover/liberar. Para "zero-copy" de verdade: alocar os buffers de treino **fora** do GC
(`mmap`/C.malloc), usar `runtime.KeepAlive` e documentar o contrato. Sem isso é *copy*,
não *zero-copy*.

### 🟡 5. Os casos de uso (IA) já têm solução melhor

| Caso de uso da spec | Ferramenta correta | SetrixDB |
|---|---|---|
| Pré-filtro de RAG | **Índice invertido / BM25** (postings = CSR!) | scan O(n) ❌ |
| Memória de longo prazo p/ LLM de borda | **Hash set / KV** | scan O(n) ❌ |
| Deduplicação de tokens | **Bloom/Cuckoo filter** (aprox.) ou hash set | scan O(n) ❌ |

A ironia: o **formato de armazenamento** que você projetou (`FlatSynonymStorage`) **é** o
formato de um índice invertido. O que falta é a **camada de consulta** de um índice
invertido (interseção de postings), não varredura. Você construiu a parte certa e a
consulta errada.

### 🟡 6. Bug sutil: "posição" é offset de **byte**

`for i, r := range term` em Go dá **offset em bytes**, não índice de rune. Em UTF-8
multibyte (`"café"`, CJK) as posições pulam. Para a fórmula "posicional" fazer sentido,
use índice de **rune** (contador) em vez do byte offset do `range`.

---

## Onde isso **pode** vencer (e como eu estruturaria)

O diferencial real de SetrixDB não é "banco", é **interseção de conjuntos em borda**:

1. **Representação:** arrays `uint64` **ordenados** por shard, ou **bitmaps Roaring**
   particionados por faixa de ID — ambos cache/banda-friendly e **SIMD-nativos**.
2. **Kernel:** `Intersect(shard, query)` com merge vetorizado (`vpcmpeqq`/`vpconflictd`)
   ou popcount de bitmaps. Plugável: **scalar → SIMD → NPU**.
3. **Roteamento:** consistent hash ring (mantém) + protocolo binário compacto.
4. **Correção:** hash + **verificação de colisão** (ou ID 128-bit).

Assim você mantém a tese ("aritmético, determinístico, baixa energia em borda") **sem** as
duas falhas fatais.

## SLOs que faltam na spec (sem eles, não há como "deployar")

- **Latência** p50/p99 alvo por consulta e por shard.
- **QPS** alvo e **tamanho máx. de shard** (que caiba em cache? em RAM?).
- **Energia** alvo (µJ ou J por operação) no alvo de borda.
- **Taxa de colisão** aceitável e **precisão exigida** (exata vs aproximada → decide
  hash set vs Bloom).
- **Semântica da consulta**: membership? interseção? união? ranking? (hoje só membership).
