# SetrixDB — o motor de conjuntos que faltava
### Visão de produto · manifesto

> **Verdade primeiro:** todo número abaixo foi **medido** (14/09/2026, VPS 2 vCPU AMD Zen4 com AVX-512,
> Go 1.22). O que é visão está marcado como visão. Nada aqui é promessa disfarçada de resultado.

---

## 1. A frase

**Não competimos com o seu banco. Somos o motor de conjuntos que trabalha *ao lado dele* —
exato, em microssegundos, em qualquer chip.**

## 2. O mundo que está chegando

A próxima década não será decidida por quem *guarda* mais dados, e sim por quem **decide** mais rápido
sobre eles. Um sistema de recomendação que responde antes do olho piscar. Um diagnóstico que cruza
milhões de sinais clínicos dentro do próprio hospital. Uma fraude barrada na borda da rede, em
hardware de bolso. Um telescópio que filtra bilhões de eventos por segundo.

Esse futuro tem um traço comum: **quase tudo é álgebra de conjuntos** — *este ID está nesta lista?
quais IDs aparecem nas duas?* — e quase tudo precisa acontecer **exato**, **rápido** e em **hardware
modesto**.

## 3. Um novo tipo de motor (não é "mais um banco")

Relacionais, colunares, NoSQL e vetoriais são **excelentes no que fazem** — e vão continuar sendo.
Cada um resolve um problema real: transações, analytics, escala de documentos, busca por
similaridade. **O SetrixDB não vem substituir nenhum deles.**

Ele preenche um espaço que todos deixaram em aberto: **operações de conjunto exatas e vetorizadas**.
Não é um banco de dados de propósito geral — é um **motor de conjuntos embarcável**. Ele **coexiste**:
o seu dado continua onde você já confia; o SetrixDB entra ao lado e responde as perguntas de
conjunto — *presença, interseção, filtragem* — em microssegundos.

E aqui está a escolha de design que garante a velocidade: o SetrixDB trabalha com **IDs** (`uint64`)
— ele **armazena conjuntos de IDs**, não os documentos em si. Um conjunto é um `uint64`, duas
operações são um `AND`, e o resultado é exato. **Tudo é aritmética.**

**Visão de categoria:** *Arithmetic / Set Database* — uma nova camada na sua stack, não um substituto.

## 4. O que nós construímos (de verdade)

Um motor **em memória, não-relacional, não-vetorial e puramente aritmético**:

- **ID único exato** — *perfect hash* próprio (CHD v2): **0 colisão**, **~4 bits/chave**,
  lookup em **~118 ns** (n = 50 milhões).
- **Interseção densa:** **6 µs** com AVX-512 — **~24× mais rápido que a implementação de bitmap de
  referência da indústria** (148 µs), sem desmerecer o trabalho dela.
- **Interseção esparsa:** **303 µs** contra **9,26 ms** (~30×).
- **Memória proporcional aos *dados*, não ao universo:** o modo híbrido resolve um universo de
  2³⁶ IDs com **1,73 MB**, onde o bitset denso pediria 8,59 GB.
- **Escala horizontal:** shards distribuídos por *consistent hash ring*; em link lento, o modo de
  **conjuntos armazenados** foi **19× mais rápido**.
- **Testado de ponta a ponta** em loopback e entre máquinas reais.
- **Sem dependências de terceiros no núcleo.**

## 5. Onde isso muda o jogo

- **E-commerce e busca:** filtros facetados, deduplicação de catálogo, geração de candidatos para
  recomendação — em microssegundos, ao lado do seu banco atual.
- **Pesquisa médica e saúde:** seleção de **coortes** (pacientes que satisfazem *N* critérios ao mesmo
  tempo), cruzamento de listas de variantes, triagem exata — rodando **no hospital**, não só na nuvem.
- **Segurança e antifraude:** listas de bloqueio, correlação de sinais, políticas de acesso.
- **Ciência e física:** filtragem de bilhões de eventos/medições onde cada consulta é uma interseção.
- **Infra e redes:** tabelas de rotas, VPCs e políticas L4 — o tipo de dado que sustenta plataformas
  de nuvem, onde faremos os testes de escala.
- **IA na borda (edge):** *feature stores*, gating e recuperação **exata** de candidatos para modelos —
  o par exato dos bancos vetoriais: onde "parecido" não basta, a resposta precisa ser *certa*.
- **Observabilidade e telecom:** séries por ID, correlação de telemetria, matching de tráfego.

> **Estratégia:** começar por **uma dor** e dominá-la — um primeiro caso real, um número citável,
> um primeiro usuário. Depois, expandir.

## 6. O convite

O SetrixDB nasce **open source (Apache-2.0)** — acreditamos que **adoção vem antes de império**: o
código é aberto, os benchmarks são reproduzíveis, e a estrada passa por **acelerar em NPU, protocolo
entre nós e escala real em nuvem**.

Se você acredita que a próxima onda da computação não é sobre *armazenar mais*, mas sobre **decidir
mais rápido** — o SetrixDB foi feito para ser o seu motor de conjuntos.

**SetrixDB — the arithmetic set engine.**
*Conjuntos. Em microssegundos. Em qualquer chip. Ao lado do seu banco.*
