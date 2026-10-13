# Spec: Formato OKF e migrações de estrutura da wiki

## Metadados
- **ID:** 006
- **Status:** draft (esqueleto gerado a partir dos docs em 2026-10-13; revisar contra os docs antes de aprovar)
- **Autor:** phm-aguiar
- **Data:** 2026-10-13
- **Feature Anterior:** 002
- **Roadmap:** `specs/features/roadmap.md`
- **Aprovado:** false

## Propósito
A wiki precisa seguir o Open Knowledge Format (versão 0.2), exportar um pacote OKF e migrar lojas criadas por versões anteriores para o layout atual, sem tocar no esquema do banco.

## Escopo

### Dentro do escopo
- Conformidade OKF das páginas e do pacote: versão do formato, estrutura, mapeamento de tipo e metadados, validação (`docs/okf.md`).
- Exportação (`export-okf`, `POST /admin/export-okf`).
- Migrações de estrutura da wiki, com checkpoint git antes e commit único `okf-migration: conform wiki to OKF v0.2` (`docs/wiki-migrations.md`, `docs/wiki-files-layer.md`).
- Tabela `wiki_migrations` (já na V01) e a etapa de boot do `serve` que roda as migrações antes do watcher (`docs/cli.md` §3.1).

### Fora do escopo (explicitamente)
- Migrações do esquema do banco (já cobertas pelo runner da 001).

## Fontes (docs)
- `docs/okf.md`, `docs/wiki-migrations.md`, `docs/wiki-files-layer.md`, `docs/cli.md` §3.1.

## Dependências
- 002 (boot do `serve` e rotas administrativas).

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
