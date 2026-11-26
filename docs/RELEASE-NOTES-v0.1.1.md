# SetrixDB v0.1.1 — notas de release (preparadas)

> **Status deste documento:** texto **pronto para publicar**, mas a release **não foi
> publicada** e a tag `v0.1.1` **não foi criada** — depende de autorização (issue #20).
> Versão **alpha / pre-release**.

Segunda versão pública do SetrixDB. A `v0.1.0` provava as ideias em *benchmark*;
a `v0.1.1` coloca essas ideias **no caminho do produto**. Fecha a issue #16 (fatias 1–3),
com **24 commits até `aef4211`** desde a `v0.1.0` (15/09/2026) — contagem de
escopo do produto; correções de documentação posteriores não entram nela.

## Destaques

- **Kernel vetorizado no *hot path*** — `addb.Bitset.AndPopcount` e
  `addb.ExtractSet` passam a usar o kernel AVX-512 com *dispatch* em runtime
  (escalar no fallback `CGO_ENABLED=0`/arm64). Antes o kernel existia, mas só era
  exercitado por benchmark/teste.
- **Interseção escolhida por custo** (`densePlan`): representação densa + AND
  vetorizado quando os conjuntos ocupam pouco universo; *merge* em listas
  ordenadas quando são esparsos — mesmo resultado nos dois caminhos (teste de
  paridade), sem opção para o usuário configurar.
- **Híbrido denso/esparso por shard, decidido em runtime** — `ShardedBitset`
  escolhe a representação **por shard** pelo custo de memória (`Finalize`), e a
  interseção faz *dispatch por par de shards* (denso×denso, esparso×esparso,
  misto). Shard vazio custa zero.
- **Cluster robusto** — timeouts de escrita/leitura, reconexão e **peer morto sem
  pendurar**; goroutine órfã no `gather` corrigida (drena todas as respostas e
  devolve o primeiro erro); protocolo malformado encerra a conexão em vez de ler
  um stream desalinhado.
- **`Keyer`: ID exato por termo** — MPHF + conferência de *bucket*; o ID deixa de
  depender da posição no arquivo (`ComputeDeterministicID` fica **depreciado**
  para conjuntos persistidos, e segue disponível para IDs efêmeros/streaming).

## Medido (máquina com AVX-512 real, AMD EPYC 9J45, 2 vCPU)

| caso | antes | depois | ganho |
|---|---|---|---|
| `AndPopcount` 128 KB de palavras (escalar → kernel) | 8,9 µs | 1,45 µs | **6,1×** |
| `Intersect` (API pública, denso: 1 Mi de universo, 200 k IDs × 2) | 2,28 ms | 0,81 ms | **2,8×** |
| `Intersect` caminho denso isolado | 1,93 ms (merge) | 0,55 ms | **3,5×** |
| consulta em dados esparsos (100 IDs/shard, híbrido) | 5,4 µs | 1,9 µs | **2,8×** |
| memória de 800 IDs em 1 Mi de universo | 131.136 B | 6.400 B | **20,5× menos** |

Ressalva honesta: o runner do GitHub Actions **não tem AVX-512** — o CI prova o
caminho escalar/genérico; o tier vetorial é provado na máquina acima e por
`TestKernelTier`, que loga o kernel efetivo.

## Compatibilidade e migração

- **Sem quebra de API pública**: `Set.Add/Len/Has/IDs`, `Intersect`, `Union`,
  `Filter`, `Index` mantêm assinatura; `ShardedBitset.Has` passou a funcionar nos
  dois modos e `Words(sh)` devolve `nil` em shard esparso.
- **Depreciação**: `ComputeDeterministicID` para conjuntos **persistidos** — use
  `Keyer`. Arquivos `.sxset` gerados com IDs posicionais não são remapeados
  automaticamente; regenere-os com `Keyer` se precisar de estabilidade entre
  execuções.
- Escrita depois de `Finalize` (`Set`) volta o shard tocado para denso: o contrato
  antigo (escrever e consultar sem cerimônia) continua valendo.

## Dependências

- `github.com/bits-and-blooms/bloom/v3` **3.7.1 → 3.7.2** (#21), com o indireto
  `github.com/bits-and-blooms/bitset` **1.24.2 → 1.24.4**. Bump de manutenção
  (Dependabot), sem mudança de API pública nem de comportamento observável.

## Não incluído

- Publicação da release e tag `v0.1.1` (aguarda autorização — issue #20).
- Comparativo de cluster multi-nó fora do laboratório; números publicados de
  cluster seguem os do manifesto k8s de 3 nós (`deploy/`).

## Known issues

- CI sem AVX-512: o *tier* vetorial não é exercitado no GitHub Actions (só o
  escalar). Rodar `TestKernelTier` localmente para ver o kernel efetivo.
- `CGO_ENABLED=0` sempre usa o caminho escalar (comportamento esperado, não bug).
