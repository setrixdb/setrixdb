# SetrixDB — Resultados consolidados (fonte do relatório final)

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

## 2. MPHF (CHD v2) — ID único, 0 colisão

Implementação própria (sem lib de terceiros): `internal/mphf/`. **v2 corrige um defeito estrutural:** a v1 usava `M = n/λ` como nº de buckets **e** tamanho da tabela (por isso o deslocamento estourava em λ alto). A v2 separa **buckets (n/λ)** da **tabela (n·(1+ε))**.

| Métrica (n=50M) | v1 | v2 (λ=3,0 · ε=0,60) |
|---|---|---|
| colisões | 0 | **0** |
| bits/chave | 13,68 (81,5 MiB) | **4,03 (24,0 MiB)** |
| build | 20,6 s | 13,6 s |
| lookup | 113 ns | 118 ns |

Varredura λ×ε (n=1M, bits/chave):

| λ \ ε | 0,10 | 0,23 | 0,40 | 0,60 |
|---|---|---|---|---|
| 1,2 | 10,34 | 8,81 | 8,15 | 7,53 |
| 2,0 | 6,67 | 5,81 | 4,99 | 5,20 |
| 2,5 | 5,17 | 4,91 | 4,69 | 4,50 |
| 3,0 | 4,84 | 4,31 | 4,15 | **4,03** |

**Veredito:** **3,4× menos memória**, lookup na mesma faixa (~118 ns), build mais rápido. Aproxima o piso teórico (~1,44 bits/chave) e a fronteira prática (~2,6–3,6 bits/chave de RecSplit/PTHash). Cabe em `uint32` (50M < 2³²); membership exata = MPHF + 1 comparação.

> Nota: no `vsbench` o "9,9 B/chave" do MPHF somava o array de chaves de entrada (`n×8 B`) à estrutura — o número limpo é `h.Bytes()` (só a estrutura).

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
| **MPHF (CHD v2)** | **0,5 B/chave** (estrutura) | ~118 ns/lookup | **sim** |
| Bloom (1% FP) | 1,2 B/chave | 23,6M ops/s | não |

**Veredito:** MPHF = paridade de velocidade com o `map`, **2,2× menos memória**, ID exato.

---

## 5. Head-to-head vs mercado — interseção (A = B = 1M)

`cmd/vsbench`.

| Estratégia | denso32 | aleat64 |
|---|---|---|
| sorted merge (SetrixDB) | 9,2 ms | 11,3 ms |
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

1. **ID único:** MPHF (CHD v2) → 0 colisão, **~4 bits/chave** (3,4× menos que a v1), lookup O(1).
2. **Membership:** paridade com `map`, 2,2× menos memória.
3. **Interseção:** bitset denso + AVX-512 → **líder** (24–30× vs Roaring), denso e esparso (enquanto couber).
4. **Escala:** sharding por range distribui o compute; memória horizontal = multi-máquina + híbrido bitset/roaring.
5. **Distribuído:** nós por shard over-the-wire (TCP) — correto em loopback e entre máquinas reais; custo = rede.
6. **Posicionamento mantido:** membership/interseção aritmética exata — **não** vira NoSQL nem vetorial.

_Reproduzível: `go test ./...` + `go run ./cmd/{setbench,vsbench,sparsebench,shardbench,mphfbench}`._

---

## 10. Híbrido denso+esparso (a parede de memória)

`internal/addb/sparseset.go` (SparseSet, merge two-pointer) + `cmd/hybridbench`. Universo **2³⁶** (68,7 bi IDs), 4M chaves.

| Representação | memória (2³⁶, 4M) |
|---|---|
| bitset denso (universo/8) | **8,59 GB** (não cabe) |
| SparseSet (chaves×8) | 32 MB |
| Roaring64 (comprimido) | 10,05 MB |
| **híbrido** (95% hot 2²⁰ + 5% cauda) | **1,73 MB** |

Interseção (1M×1M): range **densa** → bitset AVX-512 **4 µs**; universo **esparso** → SparseSet merge **9,91 ms** vs Roaring64 **105,9 ms** (merge ~10× melhor que roaring64 em 64-bit esparso).

**Veredito:** o bitset denso é imbatível em velocidade, mas a memória é `universo/8`. O **híbrido resolve**: faixas quentes = bitset (AVX-512), cauda fria = esparso → memória ∝ **dados**, não ∝ universo.

---

## 11. Distribuído (over-the-wire)

`internal/cluster/` (TCP binário, zero-copy, sem deps) + `cmd/clusterdemo` + `cmd/clusternode`. Nós servem um **shard** (fatia de palavras do bitset global) por TCP; o coordenador faz **broadcast paralelo** da consulta e soma |A_shard ∩ B|.

| Cenário | Resultado |
|---|---|
| Correção (local vs distribuído) | **idêntico** em todos os testes |
| 4 nós locais (loopback), universo 2²⁶ | local 282 µs · distribuído **3,3 ms/consulta** (307 q/s) |
| **servidor ↔ nó remoto** (link lento), universo 2²⁴, 2 shards | correto; **275 ms/consulta** (rede-bound: link ~2 MB/s) |

**Veredito:** o SetrixDB escala **horizontalmente** — memória e compute distribuídos entre máquinas. O gargalo era a **rede** (transmitir a consulta a cada query); a solução implementada são **conjuntos ARMAZENADOS**: a interseção `A ∩ B` de dois sets já armazenados só carrega o **nome** na rede (cada nó faz o `AND` local), sem transmitir dados.

| cenário | armazenado (só nomes) | ad-hoc (transmite a consulta) |
|---|---|---|
| 4 nós locais (2²⁶) | **559 µs** | 3,29 ms (**6×**) |
| **servidor ↔ nó remoto** (rede, 2²⁴) | **19,4 ms** | 364 ms (**19×**) |

Em link lento (link ~2 MB/s) o ganho explode — é o modo escalável do cluster.

---

## 12. Hash ring — topologia dinâmica do cluster

`internal/addb/ring.go` (consistent hash ring, membrosia dinâmica) + `cmd/ringbench`. 1M IDs, 2000 pontos virtuais/nó.

| nós | add→remap (SetrixDB) | balance máx | módulo (add→remap) |
|---|---|---|---|
| 4 | 18% (ideal 20%) | 23% (ideal 25%) | 80% |
| 8 | 8% (ideal 11%) | 15% (ideal 12%) | 89% |
| 16 | 7% (ideal 6%) | 9% (ideal 6%) | 94% |
| 64 | 2% (ideal 1,5%) | 3% (ideal 1,6%) | 99% |

**Veredito:** entrar/sair um nó remapeia só ~1/(N+1) dos IDs (vs ~tudo no módulo ingênuo) — a base pra **rebalancear o cluster sem re-shuffle total**. É a peça de topologia que casa com o plano de clusterizar o banco em instâncias (cluster real).

**Integração ring+cluster** (`internal/cluster/sharded.go` + `cmd/ringclusterdemo`): cada shard é atribuído a um nó **pelo anel**. Verificado: resultado **idêntico ao local** (12 shards / 3 nós) e, entrando +1 nó, **2/12 shards mudam de dono** (17%, ideal 25%) — no módulo mudariam ~todos.

---

## 13. Cluster Kubernetes (3 nós) — validação em nuvem

`deploy/k8s/` + `cmd/clusternode` / `cmd/clusterdemo`. 3 nós de shard como pods (1 por máquina,
anti-affinity), coordenador em nó separado. Kubernetes v1.36, nós de 4 vCPU / 8 GB. Comunicação
**TCP :19100 em estrela** (coordenador → nó; os nós não falam entre si). Data: 15/09/2026.

| universo | |A∩B| local | distribuído | lat. armazenado | lat. ad-hoc | ganho | load |
|---|---|---|---|---|---|---|---|
| 2²⁴ (262k palavras) | 77.668 | 77.668 ✅ | 1,2 ms | 21,2 ms | 18× | 82 ms |
| 2²⁶ (1,05M palavras) | 311.376 | 311.376 ✅ | 3,2 ms | 69,1 ms | 22× | 246 ms |
| 2²⁸ (4,2M palavras) | 1.246.437 | 1.246.437 ✅ | 10,4 ms | 258,5 ms | 25× | 1,28 s |

Correção **idêntica ao local em todos os modos e escalas**; o modo **armazenado** é **18–25× mais
rápido** que o ad-hoc — e o ganho **cresce com a escala**.

**Limite (coordenador com 1 GiB):** o teste **passa em 2²⁸** e é **OOMKilled em 2²⁹/2³⁰** — a
**corretude resiste até 2³⁰** (4.982.242 = local). O que quebra primeiro é a **memória do
coordenador**, que detém o conjunto **global** para fatiar (~10·W palavras + `GOGC=100` ≈ 2× do
*live*). Melhoria prevista: fatiar por *seed*/streaming (ver `docs/VISAO-ESCALABILIDADE.md`).

Reproduzível: `deploy/k8s/README.md`.
