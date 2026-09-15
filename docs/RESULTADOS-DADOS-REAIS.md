# Resultados com DADOS REAIS — varejo (Online Retail II)

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
