# Arquitetura — SetrixDB

Detalhamento técnico do motor de conjuntos (Arithmetic Set Engine) conforme a especificação de origem do projeto.

## 1. Visão geral

SetrixDB abandona os pilares tradicionais de um banco (relações, índices B-tree,
hashing de strings em runtime, vetores) e aposta em **uma única primitiva**:
a **aritmética determinística sobre inteiros de 64 bits**.

- **Unidade atômica:** `uint64` (o dado *é* o número).
- **Transmutação simbólica:** qualquer termo UTF-8 vira um ID `uint64`
  **determinístico**, em tempo `O(L)` — sem processar strings na busca.
- **Armazenamento:** *shards* contíguos (`[]uint64`) — cache-friendly e DMA-friendly.
- **Busca:** casamento por igualdade, *branchless*, vetorizável.
- **Escala:** paralelismo de dados (goroutines) + distribuição por hash ring.

## 2. Dimensionamento (footprint ~1 GB de RAM)

| Componente | Escala | Tamanho |
|---|---|---|
| Dicionário Global UTF-8 | ~50 mi de termos | ~400 MB (IDs `uint64` contínuos) |
| Base de Sinônimos | ~50 mi de conexões | ~460 MB (*flat arrays* + offsets `uint32`) |
| **Total** | — | **~1,0 GB** |

## 3. Mapeamento posicional determinístico

`ComputeDeterministicID` (`internal/addb/hash.go`):

```
ID = Σ_{i=0}^{L-1} ( UTF8(c_i) + i + 1 ) · B^i   (mod 2^64),  B = 31
```

- **Base multiplicativa** `B = 31` e posição `i` entram na conta ⇒ **anagramas
  diferem** (`"casa"` ≠ `"saca"`).
- Somatório em `uint64` ⇒ *wrap-around* mod 2^64, sem custo.
- Custo `O(L)`, puramente aritmético (sem alocação, sem mapa de strings).

> Nota: em Go, `range` sobre `string` itera por *rune*, mas o índice `i` é o
> deslocamento em **bytes**. A fórmula usa esse índice posicional.

## 4. Base de sinônimos em flat arrays

`FlatSynonymStorage` (`internal/addb/synonyms.go`) — **sem maps nativos**:

```
SynonymIDs: [ s0 s1 s2 | s3 s4 | … ]   // todos os sinônimos concatenados
Offsets:    [ 0        3      …    ]   // início da fatia de cada termo
Lengths:    [ 3        2      …    ]   // quantidade de sinônimos
Terms:      [ t0       t1     …    ]   // termo dono da fatia
```

A fatia de um termo `k` é `SynonymIDs[Offsets[k] : Offsets[k]+Lengths[k]]`.
Busca do termo por varredura em `Terms` (em produção: ordenar + busca binária).

## 5. Kernel de busca

```go
func Contains(shard []uint64, q uint64) bool {
	var found uint64
	for _, v := range shard {
		x := v ^ q
		eq := 1 - ((x | (^x + 1)) >> 63) // 1 se v==q, 0 caso contrário
		found |= eq
	}
	return found == 1
}
```

Sem `if` no laço quente ⇒ sem *branch misprediction*; forma amigável ao
*auto-vectorizer* (AVX-512: `vpcmpeqq` + `kortest`).

## 6. Paralelismo

`ParallelSearchEngine(queryIDs, databaseShard)` recebe o termo principal **e os
sinônimos já resolvidos em inteiros** e dispara a busca concorrente.

A especificação original usa **uma goroutine por ID** com `chan uint64`. A
implementação neste repo **endurece** isso para um **pool de workers** limitado a
`GOMAXPROCS(0)` — mesmo resultado (ordem preservada, sem duplicatas), porém sem
risco de explosão de goroutines em lotes grandes. Ver `internal/addb/search.go`.

## 7. Zero-copy / DMA para NPU

`UnsafePtr(shard)` devolve o endereço da *backing array* para drivers C/C++.
Combinado com *pinning*, o mesmo buffer vai e volta da NPU sem cópia.

> ⚠️ O shard precisa permanecer vivo e não realocado durante a transferência.

## 8. Consistent Hash Ring

`ring.go` roteia IDs `uint64` entre nós sem re-hash total (`AddNode`/`RemoveNode`
movem apenas os segmentos afetados; `Route(id)` devolve o nó dono). Pacotes
binários compactos, enviáveis por **UDP/gRPC**.

## 9. Limites e riscos

- Igualdade **pura** não responde a *range queries* nem a similaridade — é um
  **pré-filtro**, não um banco de propósito geral.
- Varredura linear só compensa com shards em RAM e/ou com SIMD.
- Risco de **colisão de hash**: o mapa posicional é determinístico, mas não é
  *injetivo* por construção (mod 2^64). Em produção, validar a taxa de colisão no
  dicionário real (50 mi de termos) e, se preciso, acrescentar um *salt*/chave.
