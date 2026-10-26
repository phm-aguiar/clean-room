# 015 — Fidelidade dos docs ao original

Regra aplicada: em toda dúvida, reproduzir o comportamento do original, inclusive defeitos. Edições pontuais em `docs/` (todos LF, preservado). `specs/` não foi tocado.

| Arquivo:linha | Antes (resumo) | Depois (resumo) |
|---|---|---|
| downtime-recovery.md:7 | "a reimplementação deve recusar os dois" (link/junção) | o original recusa os dois; reproduzir |
| downtime-recovery.md:45 | "Defeito do original a não reproduzir… deve aceitar os 14 nomes" | `recover` recusa `copilot` com `journal names unsupported harness 'copilot'`; reproduzir |
| downtime-recovery.md:107 | "não deve reproduzir… aceitar `copilot`… pendente de confirmação" | Copilot não é recuperável; entrada fica no diário; reproduzir |
| downtime-recovery.md:117 | "Recomendação… aplicar a restrição do diário, a decidir" (ACL do spool) | só o diário recebe ACL restrita; reproduzir |
| downtime-recovery.md:121 | "a reimplementação pode usar o nome documentado" | nome com ponto duplicado `hook-spool..capacity.lock`; reproduzir |
| downtime-recovery.md:125 | FILE_SHARE_DELETE **ou** repetir em 32/5 | abrir com READ\|WRITE\|DELETE; sem repetição em 32/5 (como o original) |
| cli.md:395 | "uma reimplementação pode usar `hook-spool.capacity.lock`" | reproduzir o nome do original |
| harness-adapters.md:238 | junções: "siga o mesmo critério… valide com teste" | o original não segue/coleta junções; reproduzir (teste sugerido) |
| harness-adapters.md:625 | "decisão de implementação… pode paginar" | exportação toda-ou-nada sem paginação (4 096 eventos / 32 768 registros / 64 MiB); reproduzir |
| harness-adapters.md:664 | regra de remover `\\?\` e gerar grafias | comparação textual com o canônico estendido, como o original (consequências mantidas) |
| harness-adapters.md:674-679 | "Divergências… decisões"; copilot "deve aceitá-lo"; código do filho "deve preferir devolver" | "Defeitos conhecidos (reproduzir)"; copilot recusado; erro de descoberta faz o comando falhar e perder o código de saída |
| managed-workstreams.md:524 | "na reimplementação, compare formas normalizadas… prefixo removido" | comparar canônicos como o original, com `\\?\` |
| lifecycle-ops.md:1093 | "must match with and without `.exe`" | nome exato `ai-memory`; no Windows não acha irmãos; reproduzir (conferido no original: nome exato) |
| windows.md:623 | "must match both `ai-memory` and `ai-memory.exe`" | idem acima |
| design-handoff-offer-not-claim.md:146 | "pode normalizá-la (decisão a confirmar)" | reproduzir "ago ago" |
| agent-messaging.md:166 | "should document whichever choice" | rota administrativa só com regras embutidas; reproduzir |
| admission-webhooks.md:333 | "should do the same, or document the deviation" | faz o mesmo (redirecionamentos/proxy); reproduzir. Seção de assinatura (l.331) já descritiva: sem mudança |
| auto-scope.md:330 | "should cap both strings at 512 bytes" | sem teto; reproduzir |
| design-cross-project-profile.md:240 | "may serialize them with a lock" | sem trava entre rebuild e worker; reproduzir |
| design-cross-project-profile.md:347 | recusar delimitador órfão = "decisão explícita" | não recusa; reproduzir (teste de caracterização sugerido) |
| design-cross-project-profile.md:368, 397 | G1–G8 "must cover / acceptance criteria" | testes sugeridos, só cobertura; esperam o comportamento do original |
| consolidation-llm.md:946 | "Decisão para a reimplementação: equivalente na linguagem escolhida" | só arquivos `.rs` do repositório analisado, linhas `//!`; independe da linguagem da reimplementação (conferido no original) |
| prompts-schemas-and-sanitizer.md:1211 | recusar entradas = "decisão registrada" | aceitas na construção; reproduzir |
| database-schema.md:262 | "prefira não colocar o CHECK" | CHECK existe; tipo novo = migração de reconstrução |
| users.md:285 | "should keep the outcome equivalent" | reproduzir |
| design-per-project-authz.md:228 | "should expose only scoped lookups" | reproduzir (verificação em cada chamador) |
| okf.md:47 | "deve ordenar por caminho" | ordem de leitura do diretório, sem ordenação adicional |
| overview.md:156 | "deve exigir o tipo sanitizado" | reproduzir (sem exigir) |
| consolidation.md:145 | "recomenda-se ordenar por título" | sem ordenação adicional |

`delete-workspace` sem `confirm` (api-reference.md:243) e webhooks sem assinatura (admission-webhooks.md:331) já estavam descritivos: nada mudou.

## Segunda rodada (conferido no original)

| Arquivo:linha | Antes (resumo) | Depois (resumo) |
|---|---|---|
| harness-adapters.md:666 (6.10 item 3) | "compare de forma insensível à caixa… compare normalizado" | cursor × localizador: comparação por componentes, `\` = `/`, **diferencia caixa**, sem resolver links; raízes do Kiro v3: canonicaliza os dois lados e compara (falha ao canonicalizar = diferentes) |
| harness-adapters.md:667 (item 4) | "leia com compartilhamento de leitura/escrita" | abertura padrão do original (no Windows: leitura, escrita e exclusão); o resto do item já era factual |
| harness-adapters.md:670 (item 7) | "use as mesmas fontes do original e não arredonde" | usa os metadados do sistema (modificação/criação) sem arredondar nem corrigir; reproduzir |
| design-per-project-authz.md:225 | "a confirmar… must be treated as unauthenticated… must not add a viewer" | anônimo nunca gerencia usuários (`user management requires authentication`); o estado surge só quando a autenticação é removida depois de criar usuários; aí: sem viewer, checagem por projeto passa, rotas admin dão 401 (`admin operation requires authentication in multi-user mode`); reproduzir |

## Pendências (não alteradas)

- Itens "a confirmar" factuais (desktop/, cursor.md, design-boot-backfill.md etc.) e recomendações de operação ao usuário (install.md, security.md, https-via-proxy.md) não mandam divergir; mantidos.
- Prompts em `assets/` usam "should" como texto do produto; mantidos.
