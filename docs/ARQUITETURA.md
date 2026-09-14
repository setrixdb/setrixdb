# Arquitetura — ADDB

Detalhamento técnico do **Arithmetic Database**. Reconstruído a partir do prompt do
Tião (as seções iniciais originais chegaram truncadas — ver `ESPECIFICACAO.md`).

## 1. Visão geral

ADDB abandona os pilares tradicionais de um banco (relações, índices B-tree,
hashing, vetores) e aposta em **uma única primitiva**: a **igualdade aritmética de
inteiros de 64 bits**. Tudo o mais — paralelismo, distribuição, integração com
hardware — deriva dela.

- **Unidade atômica:** `uint64`.
- **Armazenamento:** *shard* contíguo (`[]uint64`) — cache-friendly e DMA-friendly.
- **Busca:** casamento por igualdade, *branchless*, vetorizável.
- **Escala:** paralelismo de dados (particionamento do *batch*) + distribuição por
  hash ring.

## 2. Por que "puramente aritmético"?

| Abordagem tradicional | ADDB |
|---|---|
| B-tree / hash index | varredura aritmética vetorizada |
| Ponteiros entre nós | IDs `uint64` contíguos |
| Busca vetorial (similaridade) | igualdade exata |
| Alocações no caminho quente | zero-copy / zero-alloc |

O ganho: o compilador (e o hardware) consegue transformar o laço interno em
**instruções SIMD**, processando várias comparações por ciclo.

## 3. Kernel de busca

```go
func Contains(shard []uint64, q uint64) bool {
	var found uint64
	for _, v := range shard {
		// Igualdade branchless: x==0 -> 1, senão 0.
		x := v ^ q
		eq := 1 - ((x | (^x + 1)) >> 63)
		found |= eq
	}
	return found == 1
}
```

Sem `if` dentro do laço quente ⇒ sem *branch misprediction*; o *auto-vectorizer*
do Go/GC converte isso em operações de lane.

## 4. Paralelismo

`ParallelSearchEngine(batch, shard)`:

1. Fatia o **batch** de consultas em `GOMAXPROCS(0)` pedaços;
2. Cada worker varre o shard inteiro para o seu pedaço;
3. Os resultados locais são **mergeados preservando a ordem** do batch, sem duplicatas.

O gargalo é memória, não CPU — por isso a ênfase em contiguidade e zero-copy.

## 5. Zero-copy / DMA para NPU

`UnsafePtr(shard)` devolve o endereço da *backing array* para drivers C/C++.
Combinado com *pinning* de memória, o mesmo buffer vai e volta da NPU sem cópia.

> ⚠️ Regras de segurança: o shard precisa permanecer vivo e não realocado durante a
> transferência; concorrência com GC sobre slices longos exige *pinning* explícito.

## 6. Consistent Hash Ring

`ring.go` implementa um anel de hashes para rotear IDs entre nós:

- `AddNode` / `RemoveNode` movem apenas os segmentos afetados (*minimal churn*);
- `Route(id)` mapeia o ID para o nó responsável;
- Pacotes de roteamento são **binários** (ID `uint64` + payload), enviáveis por UDP/gRPC.

## 7. Limites e riscos

- Busca por igualdade **pura** não responde a *range queries* nem a similaridade —
  é um **pré-filtro**, não um banco de propósito geral.
- Varredura linear só compensa com shards que caibam em RAM e/ou com SIMD.
- Para grandes cardinalidades, considerar **shards ordenados + busca binária** ou
  **bitmaps** como evolução (mantendo a aritmética pura).
