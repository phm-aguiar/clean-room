# Padrões de Arquitetura para ADR

Referência para o `sdd-generate_plan`. Absorvido de `software-architecture-design` (AI-Agents-public).

## Decision Tree: Escolha de Padrão Arquitetural

```
Projeto precisa: [Novo Sistema ou Refatoração Grande]
    ├─ Time único (<10 devs), domínio em evolução?
    │   ├─ Comece simples → Modular Monolith (módulos com bordas claras)
    │   └─ Precisa iterar rápido → Layered Architecture
    │
    ├─ Múltiplos times, bounded contexts claros?
    │   ├─ Deploy independente crítico → Microservices
    │   └─ Modelo de dados compartilhado → Modular Monolith com service modules
    │
    ├─ Workflows orientados a eventos?
    │   ├─ Processamento assíncrono → Event-Driven (Kafka, NATS, RabbitMQ)
    │   └─ State machines complexas → Saga pattern + Event Sourcing
    │
    ├─ Carga variável/imprevisível?
    │   ├─ Pay-per-use → Serverless (AWS Lambda, Cloudflare Workers)
    │   └─ Batch processing → Serverless + queues
    │
    └─ Requisitos fortes de consistência?
        ├─ Garantias ACID → Monolith ou Modular Monolith
        └─ Dados distribuídos → CQRS + Event Sourcing
```

**Fatores de decisão:**

- **Tamanho do time:** <10 devs → modular monolith geralmente supera microservices (custo operacional)
- **Estrutura do time:** Arquitetura espelha estrutura organizacional (Lei de Conway)
- **Independência de deploy:** Cada serviço pode ser deployado sozinho?
- **Consistência vs disponibilidade:** CAP theorem — o que ceder?
- **Maturidade operacional:** Tem monitoring, orquestração, observabilidade?

## Padrões por Domínio (Go)

### APIs e Serviços
| Pattern | Quando usar | Go específico |
|---|---|---|
| Layered (handler → service → repository) | CRUD simples, domínio estável | `net/http` + interfaces |
| Hexagonal (ports & adapters) | Domínio rico, múltiplos adapters | Interfaces como ports, structs como adapters |
| CQRS | Leitura e escrita com demands diferentes | Separar `*sql.DB` queries de commands |

### Comunicação entre Serviços
| Pattern | Quando usar | Go específico |
|---|---|---|
| REST + JSON | Síncrono, request-response | `net/http` client, `encoding/json` |
| gRPC | Alta performance, contrato forte | `google.golang.org/grpc` |
| Message Broker (NATS/Kafka) | Assíncrono, desacoplado | `nats.go`, `sarama` |
| WebSocket | Tempo real, bidirecional | `gorilla/websocket`, `nhooyr.io/websocket` |

### Dados
| Pattern | Quando usar | Go específico |
|---|---|---|
| Repository | Abstrair acesso a dados | Interface + implementação concreta |
| Unit of Work | Transações multi-repo | `database/sql` com `*sql.Tx` |
| Event Sourcing | Audit trail completo | Append-only log, snapshots |

## Output Guidelines para o Plano Arquitetural

Toda recomendação de arquitetura deve incluir:

1. **Tecnologias concretas:** Nomeie bibliotecas/frameworks específicos (ex: "NATS JetStream para message broker", "sqlc para type-safe SQL"), não fique no abstrato
2. **O que NÃO construir:** Chame explicitamente o que adiar ou evitar. Escopo prematuro é o erro #1 de arquitetura
3. **Alinhamento com time:** Como essa arquitetura mapeia pra estrutura do time? Quem é dono de quê?
4. **Métricas de sucesso:** Como saber se a arquitetura está funcionando? (deploy frequency, lead time, error rates, MTTR)
5. **Foco nas 3-5 decisões que importam:** Profundidade nas decisões críticas, não cobertura exaustiva

## Anti-padrões

- **Microservices prematuro:** Se tem <3 serviços e <10 devs, comece com modular monolith
- **Arquitetura de PowerPoint:** Diagramas que não refletem o código real. ADRs devem ser vivos
- **Overengineering:** Resolver problemas que você não tem (ainda). "You ain't gonna need it"
- **Tecnologia por hype:** Escolher ferramenta porque é trending, não porque resolve o problema
