# ADDB — Marketing & Nome Comercial
### Do nome técnico (`ADDB`) ao produto de mercado

> **✅ DECISÃO (15/09/2026 — Tião):** marca = **Setrix**, produto = **SetrixDB** (um "x").
> Domínios: **`setrixdb.com`** + **`setrixdb.io`**. Org GitHub: **`setrixdb`**.
> Licença: a definir (**sugestão: Apache-2.0**). INPI: "setrix"/"setrixdb" → **0 registros**.

> "ADDB / Arithmetic Database" é o **nome de projeto** (interno). Para vender a ideia,
> precisamos de uma **marca**, um **posicionamento** e uma **mensagem**.

---

## 1. Posicionamento (a frase que define o produto)

> **Para** equipes que precisam cruzar e filtrar bilhões de IDs em tempo real,
> **o `<NOME>`** é um **banco de dados de conjuntos aritméticos** que responde
> interseção e membership **em microssegundos, de forma exata e vetorizada**.
> **Diferente de** bancos relacionais, NoSQL e vetoriais, ele **não guarda documentos nem
> vetores** — ele trata **conjuntos de inteiros** como cidadão de primeira classe e roda
> em **qualquer hardware** (x86, ARM, NPU).

**Categoria a cunhar:** *Arithmetic / Set Database* · *SIMD-native data engine*.

---

## 2. Taglines (escolher 1)

- **"The arithmetic database."** — simples, define categoria.
- **"Sets in microseconds."** — benefício direto.
- **"Exact. In-memory. Vectorized."** — três atributos.
- **"O banco que a borda estava esperando."** — emocional/edge (PT-BR).
- **"Bilhões de IDs. Microssegundos. Zero cópia."** — PT-BR, técnico-vendedor.

---

## 3. Elevator pitch (30 s)

> Bancos de dados modernos foram feitos para **linhas, documentos ou vetores** — não para
> **conjuntos**. Mas quase tudo que a IA na borda faz hoje é exatamente isso: cruzar listas
> de IDs. O `<NOME>` é um banco **em memória, exato e puramente aritmético**: converte tudo
> em IDs, representa como bitsets vetorizados e responde interseções e presença em
> **microssegundos** — **24 a 30× mais rápido** que o estado da arte, com **memória
> proporcional aos dados** e rodando em **qualquer chip, de NPU a servidor**.

---

## 4. Mensagens-chave (o que repetir sempre)

1. **Não é mais um banco** — é uma **categoria nova**: *arithmetic/set database*.
2. **Exato, não aproximado** — o oposto dos vetoriais.
3. **Feito para o hardware de hoje** — SIMD/NPU, não disco.
4. **Memória ∝ dados** — escala sem explodir RAM.
5. **Escala horizontal nativa** — sharding + ring + conjuntos armazenados.
6. **Zero dependências** — núcleo escrito do zero; encaixa em qualquer stack.

## 5. Objeções & respostas

| Objeção | Resposta |
|---|---|
| "Já existe Roaring Bitmap." | Roaring é CPU escalar e comprimido; nós usamos **AVX-512/NPU** e batemos ele em **24–30×** onde cabe em RAM. |
| "Isso é um banco?" | É um **engine de conjuntos** com camada de armazenamento/consulta — pode ser embarcado ou serviço. |
| "E o caso gigante que não cabe?" | **Híbrido** (quente denso + cauda esparsa) + sharding multi-máquina. |
| "Serve para meu caso?" | Se sua carga é **cruzamento/filtragem/presença sobre IDs**, sim — e rápido. |

---

## 6. Curto prazo de nomes (curto, moderno, pronunciável)

| Nome | Racional | `dominio.com` | `dominio.io` |
|---|---|---|---|
| **Arithmo** | direto de *arithmetic*; curto, marca fácil | ocupado | ocupado |
| **Quantara** | *quantity* + sufixo moderno; som AI/scale | ocupado | ocupado |
| **Numera** | *numeral/numerar*; sério, europeu | ocupado | ocupado |
| **Algora** | *algorithm* + ágora (mercado, gr.); forte | ocupado | ocupado |
| **Axion** | partícula + axioma; moderno, tech | ocupado | ocupado |
| **Setrix** | *set* + sufixo tech; curto | ocupado | livre aparente |
| ~~Addb~~ | interno; sem apelo de marca | — | aparentemente livre |

### ✅ Escolhido: SetrixDB
- **Marca:** Setrix · **Produto/engine:** SetrixDB (relação *Postgres → PostgreSQL*).
- **Domínios:** `setrixdb.com` (oficial) + `setrixdb.io` (dev) — ambos **livres**.
- **Org GitHub:** `setrixdb` (livre) — projeto nasce **open source**.
- **Tagline:** *"SetrixDB — the arithmetic database."*
- **Licença:** pendente de decisão (ver recomendação Apache-2.0 abaixo).

**Observação honesta:** os `.com`/`.io` da maioria já estão ocupados (verifiquei DNS).
Caminhos: variantes (`arithmo.dev`, `numera.db`, `getalgora.com`), sufixos **`.ai` `.dev` `.db`**,
ou cunhar algo mais único. **Se você gostar de um, eu checo disponibilidade real + registro.**

### Direções de identidade visual
- **Símbolo:** bit/`{ }`/interseção de dois conjuntos (∩) trabalhado geometricamente.
- **Paleta:** preto + um acento elétrico (verde-limão ou ciano) — "hardware/silício".
- **Tom de voz:** técnico, direto, sem hype. "Microssegundos", não "revolucionário".

---

## 7. Próximos passos de marketing

1. **Escolher o nome** (decisão do Tião) → registro de domínio + marca.
2. **One-pager** (1 página) + **deck** (8–10 slides) derivados de `APRESENTACAO-PRODUTO.md`.
3. **Repositório/site de produto** com o pitch e os números reproduzíveis.
4. **Prova pública**: benchmark reprodutível + artefatos (PDF, diagrama) para o primeiro contato.
