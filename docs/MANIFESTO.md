# SetrixDB — o banco que pensa em conjuntos
### Manifesto / visão de produto

> **Verdade primeiro:** todo número abaixo foi **medido** (14/09/2026, VPS 2 vCPU AMD Zen4 com AVX-512,
> Go 1.22). O que é visão está marcado como visão. Nada aqui é promessa disfarçada de resultado.

---

## 1. A frase

**Não guardamos documentos. Não guardamos vetores. Guardamos *conjuntos* — e os cruzamos em microssegundos.**

## 2. O mundo que está chegando

A próxima década não será decidida por quem *guarda* mais dados, e sim por quem **decide** mais rápido
sobre eles. Um sistema de recomendação que responde antes do olho piscar. Um diagnóstico que cruza
milhões de sinais clínicos em tempo real, no próprio hospital. Uma fraude barrada na borda da rede,
em hardware de bolso. Um telescópio que filtra bilhões de eventos por segundo.

Esse futuro tem uma característica em comum: **quase tudo é álgebra de conjuntos** — *este ID está
nesta lista? quais IDs aparecem nas duas?* — e quase tudo precisa acontecer **exato**, **rápido** e
em **hardware modesto**.

E aqui está o incômodo: os bancos de dados que dominam o mercado foram desenhados para **outra era**.
Bancos relacionais pensam em linhas e disco. Colunares, em analytics em lote. NoSQL, em documentos
por chave. Vetoriais, em **aproximação** — acham "parecido", não *igual*. Nenhum deles trata o
**conjunto de inteiros** como cidadão de primeira classe, nem conversa com o que o silício moderno
tem de melhor: unidades SIMD largas e NPUs.

**Essa lacuna é o SetrixDB.**

## 3. O que nós construímos (de verdade)

O SetrixDB é um motor de banco de dados **em memória, não-relacional, não-vetorial e puramente
aritmético**. A "inteligência" da busca não é um índice exótico nem um modelo estatístico — é
**aritmética de inteiros**, vetorizada.

- **ID único exato:** um *perfect hash* próprio (CHD v2) — **0 colisão**, **~4 bits/chave**,
  lookup em **~118 ns** (n = 50 milhões).
- **Interseção densa:** **6 µs** com AVX-512 — **~24× mais rápido** que o Roaring Bitmap (148 µs),
  o padrão da indústria.
- **Interseção esparsa:** **303 µs** contra **9,26 ms** do Roaring64 — **~30×**.
- **Memória proporcional aos *dados*, não ao universo:** o modo híbrido resolve um universo de
  2³⁶ IDs com **1,73 MB**, onde o bitset denso exigiria **8,59 GB**.
- **Escala horizontal real:** shards distribuídos por *consistent hash ring*, consultas em
  *broadcast* zero-copy; em link lento, o modo de **conjuntos armazenados** foi **19× mais rápido**.
- **Testado de ponta a ponta:** roda igual em loopback e entre máquinas reais via VPN.
- **Sem dependências de terceiros no núcleo.** MPHF, bitset, anel e cluster escritos do zero.

Isso não é "mais um banco". É uma **categoria nova**: *Arithmetic / Set Database*.

## 4. Por que isso é diferente (e por que agora)

Escolhemos três apostas que, juntas, ninguém estava fazendo:

1. **Exatidão em vez de aproximação.** Os bancos vetoriais dominam a "busca por IA", mas devolvem
   *parecidos*, não *iguais*. Quando a resposta precisa ser **exata** — elegibilidade, deduplicação,
   segurança, conformidade —, a aproximação é um risco, não uma feature.
2. **O hardware como protagonista.** Trocamos ponteiros e *hashing* por vetores de inteiros que a
   CPU/NPU engole de uma vez. O mesmo desenho que ignora disco abraça **AVX-512 e AVX10.2** hoje,
   e NPUs amanhã.
3. **Memória que acompanha os dados.** Ao separar o "quente" (denso) do "frio" (esparso), o consumo
   deixa de ser `universo ÷ 8` e passa a ser proporcional ao que você realmente tem.

## 5. Onde isso muda o jogo

- **E-commerce e busca:** filtros facetados, deduplicação de catálogo, geração de candidatos para
  recomendação — **em microssegundos**, sem queimar um cluster inteiro.
- **Pesquisa médica e saúde:** seleção de **coortes** (pacientes que satisfazem *N* critérios ao mesmo
  tempo), cruzamento de listas de genomas/variantes (k-mers), e triagem exata — rodando **no hospital**,
  não só na nuvem.
- **Segurança e antifraude:** listas de bloqueio, correlação de sinais e políticas de acesso — exato,
  instantâneo, em qualquer volume.
- **CIÊNCIA e FÍSICA:** filtragem de bilhões de eventos/medições (astro, sensores, aceleradores) onde
  cada consulta é uma interseção.
- **Infra e redes (a nuvem):** tabelas de rotas, VPCs e políticas L4 — exatamente o tipo de dado que
  sustenta plataformas como a **plataforma de nuvem**, onde faremos os testes de escala.
- **IA na borda (edge):** *feature stores*, gating e recuperação **exata** de candidatos para modelos —
  o par exato dos bancos vetoriais: onde "parecido" não basta, a resposta precisa ser *certa*.
- **Observabilidade e Telecom:** séries por ID, correlação de telemetria, matching de tráfego.

## 6. O convite

O SetrixDB nasce **open source (Apache-2.0)** — acreditamos que **adoção vem antes de império**: o
código é aberto, os benchmarks são reproduzíveis, e a estrada passa por **acelerar em NPU, protocolo
entre nós e escala real em nuvem**.

Se você acredita que a próxima onda da computação não é sobre *armazenar mais*, mas sobre **decidir
mais rápido** — o SetrixDB foi feito para você.

**SetrixDB — the arithmetic database.**
*Conjuntos. Em microssegundos. Em qualquer chip.*
