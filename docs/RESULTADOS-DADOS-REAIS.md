# Resultados com DADOS REAIS — varejo e palavras

> Teste de ponta a ponta com um **dataset público real** de e-commerce/retail, usando a **CLI**
> (`setrixdb build` + `setrixdb intersect`) e verificação independente do resultado.

## 1. Dataset

- **Fonte:** [Online Retail II](https://archive.ics.uci.edu/dataset/502/online+retail+ii) — UCI Machine Learning Repository.
- **Natureza:** transações reais de um varejista **online do Reino Unido**, **01/12/2009 → 09/12/2011**.
- **Volume:** **1.067.371 linhas** de venda (2 planilhas), ~5.3 mil SKUs distintos.
- Formato original: `online_retail_II.xlsx` (45,6 MB). Colunas: *Invoice, StockCode, Description,
  Quantity, InvoiceDate, Price, Customer ID, Country*.

## 2. O que fizemos

Cada **linha de venda** recebeu um **ID `uint64`** (1..1.067.371) e derivamos **conjuntos** por faceta:

| Conjunto | Critério | Tamanho |
|---|---|---|
| `all` | todas as linhas | 1.067.371 |
| `uk` | país = United Kingdom | 981.330 |
| `q4_2011` | out–dez/2011 | 170.979 |
| `price5` | preço unitário ≥ 5 | 186.889 |
| `dec2011` | dezembro/2011 | 25.526 |
| `cust12347` | cliente 12347 | 253 |

## 3. A consulta (filtro facetado = interseção)

> **"Linhas do Reino Unido **E** no 4º trimestre de 2011 **E** com preço ≥ 5."**

```bash
setrixdb build -o uk.sxset      -input uk.txt
setrixdb build -o q4_2011.sxset -input q4_2011.txt
setrixdb build -o price5.sxset  -input price5.txt

setrixdb intersect uk.sxset q4_2011.sxset price5.sxset --bench 500
```

**Resultado:**

| Métrica | Valor |
|---|---|
| Conjuntos | 981.330 · 170.979 · 186.889 IDs |
| Resultado | **22.701** linhas |
| 1ª execução | **923 µs** |
| Melhor de 500 | **823 µs** |

## 4. Verificação independente (verdade)

O resultado foi conferido **por fora** do SetrixDB, com `sort` + `comm` (interseção clássica do Unix):

```bash
LC_ALL=C comm -12 <(sort uk.txt) <(sort q4_2011.txt) \
  | LC_ALL=C comm -12 - <(sort price5.txt) | wc -l
# => 22701   (idêntico ao SetrixDB)
```

✔️ **Exato.** O SetrixDB devolveu exatamente a mesma interseção, em **sub-milissegundo**.

## 5. Leitura honesta

- É **dado real** (não sintético), de um varejista de verdade — valida ingestão, facetas e interseção.
- A cardinalidade aqui é de **~1M linhas**; escalas de 10M–1B são o próximo passo (dataset de catálogo
  maior) — e o motor já demonstrou esse patamar nos benchmarks sintéticos (`docs/RESULTADOS.md`).

_Reprodutível: baixe o dataset, derive as listas de IDs, rode `setrixdb build`/`intersect`._

---

## 6. Base de PALAVRAS reais — títulos da Wikipédia (19,3 milhões de termos)

Segunda rodada, agora com uma **base de palavras/termos reais em escala**:

- **Fonte:** dump público `enwiki-latest-all-titles-in-ns0` (títulos reais da Wikipédia, incluindo redirects).
- **Volume:** **19.264.252 termos** (títulos). Observação: o dump usa `_` no lugar de espaço
  (ex.: `United_States`), então "palavra composta/frase" aparece como termo com underscore.
- Conjuntos derivados:

| Conjunto | Critério | Tamanho |
|---|---|---|
| `has_space` | termo de **múltiplas palavras** (`_`) | 16.926.181 |
| `len20` | comprimento ≥ 20 | 8.514.223 |
| `start_s` | começa com **s/S** | 1.616.610 |
| `united` | prefixo `United` | 38.636 |

**Consultas (interseção):**

| Consulta | Resultado | Tempo (melhor de 200) |
|---|---|---|
| multi-palavra **E** começa com `s` | **1.408.399** | **9,5 ms** |
| multi-palavra **E** começa com `United` | **38.602** | **8,0 ms** |

**Verificação independente** (`comm`/`sort`): **1.408.399** e **38.602** — **idênticos**. ✔️

> Isto é exatamente o caso de uso de **texto**: cada termo/frase (inclusive composto com espaço) é uma
> chave; consultas combinadas são interseção de conjuntos. Ver `docs/TEXTO-E-FRASES.md`.

---

## 7. Escala 10M+ — MovieLens 25M (25 milhões de avaliações reais)

Dataset público **MovieLens 25M** (GroupLens): **25.000.095 avaliações** de 162 mil usuários sobre
62 mil filmes. Cada avaliação virou um ID `uint64` (índice da linha) e as facetas foram derivadas do
catálogo (gênero, década do filme, nota). Ambiente: servidor de referência 2 vCPU Zen4/AVX-512, Go 1.22.

| Conjunto (faceta) | membros |
|---|---|
| nota ≥ 4,0 | **12.452.811** |
| Drama | **10.962.833** |
| Comédia | 8.926.230 |
| anos 2000 | 6.884.974 |

Ingestão (CLI): ~1,5 s por conjunto de ~11M IDs.

### Consultas facetadas (dados reais)

| Consulta | Resultado | Verificação independente (`sort`/`comm`) |
|---|---|---|
| Drama **E** anos 2000 **E** nota ≥ 4 | **1.634.027** | ✅ idêntico |
| Drama **E** nota ≥ 4 | **6.096.563** | ✅ idêntico |
| Comédia **E** nota ≥ 4 **E** anos 2000 | **965.677** | ✅ idêntico |

### Representação importa (mesmo universo de 25M IDs)

| Caminho | memória/conjunto | latência (A∩B) |
|---|---|---|
| Merge de listas ordenadas (CLI) | 87,7 MB | 80,4 ms |
| **Bitset denso (AVX-512)** | **2 MB** | **227 µs** (~350× mais rápido) |

Com universo **denso** (25M IDs) e conjuntos grandes, o **bitset** é ao mesmo tempo **mais rápido** e
**muito menor** que a lista ordenada: a memória do bitset é `universo/8` (constante no universo),
enquanto a lista guarda 8 bytes por membro. É o caso de uso onde o motor brilha — e onde a escolha de
representação decide o resultado.
