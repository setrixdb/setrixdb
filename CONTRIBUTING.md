# Contribuindo com o SetrixDB

Obrigado pelo interesse! Este projeto é **open source (Apache-2.0)** e contribuições são bem-vindas.

## Como contribuir

1. Abra uma **issue** descrevendo o problema/ideia antes de grandes mudanças.
2. Faça um **fork** e crie um branch com nome descritivo (`feat/kernel-avx10`, `fix/mphf-collision`).
3. Escreva código **com testes** e rode a suíte completa antes do PR:

   ```bash
   go vet ./...
   go test ./...
   ```

4. Abra o **Pull Request** explicando o *o quê* e o *porquê*, com evidência (benchmarks, saída de teste).

## Diretrizes

- **Verdade acima de tudo:** não reporte sucesso sem teste real; número de benchmark precisa ser reproduzível.
- **Sem dependências de terceiros no núcleo** — o motor é escrito do zero (MPHF, bitset, ring, cluster).
- **Siga o estilo Go** (`gofmt`) e mantenha o código vetorizável/sem ponteiros indiretos no caminho quente.
- Commits pequenos e com mensagem clara.

## Licença das contribuições

Ao enviar um PR você concorda em licenciar sua contribuição sob a **Apache-2.0** e declara ter o direito de fazê-lo.
