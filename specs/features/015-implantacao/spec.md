# Spec: Implantação (contêiner, instalação offline, HTTPS por proxy, Windows nativo)

## Metadados
- **ID:** 015
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 007
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
Levar o servidor para além da máquina local com segurança: contêiner, instalação sem rede, TLS por proxy reverso e os extras do Windows nativo.

## Escopo

### Dentro do escopo
- Implantação em contêiner e health check (`docs/deploy.md`).
- Instalação offline (`docs/airgapped-install.md`).
- HTTPS por proxy reverso e redes de proxy confiáveis (`docs/https-via-proxy.md`).
- Windows nativo e extras (serviço, caminhos, antivírus) (`docs/windows.md`, `docs/windows-native-extras.md`).
- Rotinas de backup externo e espelho git remoto (`docs/backup.md`).
- Comandos `init`, `generate-auth-token` e `upgrade` (`docs/cli.md` §9).

### Fora do escopo (explicitamente)
- macOS (fora do escopo inicial, `.github/memory/tech.md`).

## Fontes (docs)
- `docs/deploy.md`, `docs/airgapped-install.md`, `docs/https-via-proxy.md`, `docs/windows.md`, `docs/windows-native-extras.md`, `docs/backup.md`, `docs/cli.md`.

## Dependências
- 007 (autenticação para exposição em rede), 002.

## Comportamento Esperado
> A preencher na revisão contra os docs listados em "Fontes". Cenários Gherkin (sucesso, falha e borda) saem dos cenários de aceitação dos próprios docs, quando existirem.

## Critérios de Sucesso
- [ ] A preencher na revisão: cada regra numérica e cada cenário de aceitação dos docs com teste correspondente
- [ ] Testes de contrato das invariantes tocadas, com teste multi-sessão quando a constituição exigir
- [ ] `scripts/check.sh` e o lint passam sem apontamentos

## Checklist de Prontidão
- [ ] Propósito claro e sem ambiguidade
- [ ] Escopo delimitado (dentro/fora)
- [ ] Cenários cobrem happy path e alternativos
- [ ] Edge cases identificados com comportamento esperado
- [ ] Constraints explícitas
- [ ] Critérios de sucesso mensuráveis
- [ ] Abordagem escolhida justificada
