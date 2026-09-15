# Texto, palavras e frases — como o SetrixDB lida

> Pergunta central: **o motor lida com frases, ou só com palavras?**
> Resposta curta: o motor é **agnóstico** — ele guarda **IDs `uint64`**. Quem lida com texto é a
> **camada de dicionário** que mapeia *string → ID*. E aí frases, palavras e compostos com espaço
> são todos a **mesma coisa**: uma chave.

---

## 1. O motor não sabe o que é uma "palavra"

O SetrixDB armazena e cruza **conjuntos de inteiros**. Para ele, não existe "palavra" nem "frase":
existe **chave**. Logo:

| Entrada de texto | Como entra | Suportado? |
|---|---|---|
| `pizza` (palavra) | 1 chave | ✅ |
| `são paulo` (frase/composição com espaço) | **1 chave** (a string inteira) | ✅ |
| `e-commerce` (hífen) | 1 chave | ✅ |
| `New York City` | 1 chave | ✅ |
| documento inteiro | N chaves (ver §3) | ✅ |

**Não há tratamento especial para o espaço.** `"são paulo"` é só uma string mais longa que
`"pizza"`. O ID vem da **camada de dicionário**.

## 2. A camada de dicionário (`string → uint64`)

O motor oferece/prevê três estratégias — cada uma com um trade-off:

| Estratégia | Colisão | Memória | Uso |
|---|---|---|---|
| **Hash determinístico** (`ComputeDeterministicID`) | possível | **zero** (não guarda nada) | streaming, IDs efêmeros |
| **MPHF + verificação** | **0** (exato) | ∝ vocabulário | quando precisa de exatidão |
| **Dicionário externo** (a sua tabela) | 0 | ∝ vocabulário | integração com o sistema do cliente |

## 3. Três níveis de uso

1. **Busca por termo/frase exata** (`has`): a frase inteira é **uma** chave. "*Este termo/frase está
   no meu vocabulário?*" — 1 lookup.
2. **Combinação de filtros** (`intersect`): cada atributo é um conjunto de IDs; "cor **E** tamanho
   **E** marca" = interseção. *(Aqui o texto já foi mapeado a IDs na ingestão.)*
3. **Busca de frase DENTRO de texto** (full-text): quebre o texto em **shingles/n-gramas** de
   palavras (ex.: janelas de 3 palavras) e indexe cada janela como uma chave. Uma **frase** é então a
   **interseção** das janelas (e a ordem vem do próprio n-grama). É assim que se faz *phrase match*
   com um motor de conjuntos.

## 4. Normalização é obrigatória (e hoje é sua)

⚠️ **O motor é exato** — e "exato" significa que `"São Paulo"`, `"sao paulo"` e `"São  Paulo"`
(dois espaços) geram **IDs diferentes**. Veja com o próprio CLI:

```bash
$ setrixdb id "são paulo" "sao paulo" "São Paulo"
  "são paulo"   -> 106532816968652
  "sao paulo"   -> 105651496225965
  "São Paulo"   -> 106532787415948
```

Por isso, **antes de gerar o ID**, normalize a string:

- **case-fold** (`strings.ToLower` / Unicode-aware);
- **Unicode NFC** (acentos compostos vs pré-compostos);
- **trim** + **colapso de espaços** (`strings.Fields` + rejoin por 1 espaço);
- opcional: remover pontuação leve, tratar sinônimos (temos `FlatSynonymStorage`).

> A normalização é uma **decisão de produto** (não do motor): cada caso de uso tem regras próprias.

## 5. Resumo

- **Frases? Sim** — a frase inteira é uma chave, igual a qualquer palavra.
- **Palavras compostas com espaço? Sim** — é só uma string com espaço.
- **Espaço não é especial.** O que importa é a **normalização** e a **estratégia de dicionário**.
- **Busca de frase em texto** = indexar n-gramas (janelas de palavras) e intersectar.
