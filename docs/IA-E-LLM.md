# SetrixDB para IA e LLMs
### Onde ele entra — e onde **não** entra

> Honestidade primeiro: o SetrixDB **não é** um banco vetorial, **não** faz similaridade e **não**
> "entende" texto. Ele é a camada **exata** que trabalha **ao lado** da busca por similaridade.

---

## 1. O encaixe

LLMs trabalham por **similaridade** (aproximação). Mas quase toda decisão ao redor deles é
**exata**: *este documento o usuário pode ver? este trecho já veio antes? quais itens satisfazem
todos os filtros?* — e isso é **álgebra de conjuntos**.

**O SetrixDB é o par exato da busca vetorial:** o modelo decide por *parecido*; o SetrixDB decide
por *certo*.

## 2. Casos de uso reais

1. **RAG com permissões (a maior dor):** antes de montar o contexto, filtrar
   `docs_permitidos ∩ candidatos`. Sem isso, RAG é um vetor clássico de **vazamento de dados**.
   1 interseção exata resolve.
2. **Deduplicação de contexto:** remover trechos repetidos que inflam (e confundem) o prompt.
3. **Filtros de metadados exatos:** data, idioma, categoria, tenant — todos como conjuntos,
   combinados com **E** / **OU**.
4. **Geração de candidatos / feature store:** entregar candidatos em microssegundos para o
   re-ranking ou para o modelo.
5. **Guardrails determinísticos:** listas exatas de bloqueio/permissão que **não** dependem da
   "boa vontade" do modelo.

## 3. "Economia de tokens" — o que é verdade

**Não é uma feature mágica; é consequência.** Token economizado vem de **recuperar menos e melhor**:

- **Contexto menor:** filtrar antes de montar o prompt → menos trechos → **menos tokens de entrada**.
- **Sem repetição:** dedup evita pagar duas vezes pelo mesmo conteúdo.
- **Menos ruído:** cortar o irrelevante reduz tokens "desperdiçados" e melhora a resposta.

> **Ordem de grandeza:** cortar o contexto pela metade tende a cortar ~50% dos tokens de *input* —
> mas isso **depende do pipeline**, então tratamos como potencial, não como garantia.

## 4. O que **não** prometemos

- Não substitui embeddings nem o banco vetorial.
- Não faz busca semântica nem geração.
- Não "pensa" — ele **conta conjuntos**, exato e rápido.

## 5. A frase

**"O modelo pensa por similaridade. O SetrixDB decide por certeza."**
