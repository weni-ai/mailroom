# Implementation Plan: Publicação confiável, sem atrasos em cascata e rastreável por logs para eventos de criação de ticket

**Branch**: `005-fix-sqs-ticket-publish` | **Date**: 2026-11-27 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/005-fix-sqs-ticket-publish/spec.md`

**Artefatos**: [research.md](research.md) · [data-model.md](data-model.md) · [contracts/sqs-ticket-message.md](contracts/sqs-ticket-message.md) · [contracts/publish-logs.md](contracts/publish-logs.md) · [quickstart.md](quickstart.md)

## Summary

Eventos de criação de ticket de organizações multi-agente (`is_multi_agents = true`) são publicados no SQS por uma fila Redis por org. Hoje o caminho tem quatro falhas confirmadas no código: o cliente SQS serializa todos os workers por um mutex que segura a chamada de rede; o retry é uma goroutine em memória que reenfileira no fim da fila do org; a tarefa sai da fila sem registro de processamento; e os logs não têm identificadores do ticket.

A abordagem: (1) cliente SQS com snapshot de conexão e reconexão por geração, sem lock durante rede; (2) fila `sqs_publish` com três estados persistidos no Redis — elegível (ordenada por `queued_on`), atrasada (retry com `next_attempt_at`) e em voo (lease com deadline) — com todas as transições feitas por scripts atômicos; (3) uma fonte de tarefas injetada no foreman, que faz o pop atômico e deixa a confirmação ao handler; (4) um loop de manutenção que promove retries vencidos e recupera leases expirados; (5) logs com um único conjunto de campos, sem URN, emitidos por um módulo dedicado; (6) um relatório de erro individual (`slow_delivery`), encaminhado ao Sentry pelo hook existente, quando o envio acontece mais de 5 minutos depois da criação do ticket, com a decomposição do tempo. O contrato externo (corpo e atributos SQS) não muda.

## Technical Context

**Language/Version**: Go 1.23.5 (pinado em `.github/workflows/ci.yml`, conforme a constituição).

**Primary Dependencies**: `github.com/redis/redigo` via `gomodule/redigo` (fila), `aws-sdk-go-v2/service/sqs` (cliente), `sirupsen/logrus` (logs), `pkg/errors`, `stretchr/testify`, `nyaruka/gocommon` (uuids, urns), `nyaruka/goflow` (eventos de ticket). Sem dependências novas.

**Storage**: Redis para a fila e o estado de entrega (zsets, hash e scripts Lua). PostgreSQL inalterado (nenhuma migração). Redis >= 5 necessário para `redis.call('TIME')` em script; **verificar a versão de produção**.

**Testing**: `go test ./... -p=1` com `testsuite` (PostgreSQL `mailroom_test`, Redis, RabbitMQ). SQS simulado com `httptest.Server` via `SqsEndpoint`. Logs capturados com hook do `logrus`.

**Target Platform**: Linux, workers do Mailroom em containers (processo único com foremen por fila).

**Project Type**: serviço backend (worker com fila Redis e API HTTP interna).

**Performance Goals**:
- Operação normal: 95% das mensagens enviadas até 60 s após o enfileiramento (SC-002).
- Paralelismo: um envio lento não reduz a concorrência além de um worker (SC-003).
- Latência adicional de retry: no máximo ~1 s (tick do loop de manutenção) somado ao intervalo `SqsPublishDelayIntervalMs`.

**Constraints**:
- Sem novos nomes de configuração (reutiliza `SqsPublishWorkers`, `SqsPublishMaxAttempts`, `SqsPublishDelayIntervalMs`, `ProcessingTTL`). Constantes internas nomeadas para tick e lote.
- Sem métricas, contadores, painéis, regras de alerta ou monitoramento contínuo (spec). Única exceção: o relatório pontual `slow_delivery` (FR-017), um log de nível `error` que o hook existente do Sentry encaminha.
- Limite de entrega lenta: constante interna `sqsSlowDeliveryThreshold = 5 * time.Minute`, sem novo nome de configuração.
- Corpo e atributos SQS inalterados (RF6).
- URN do contato nunca em log (L8, FR-016).
- Estado fora da memória do processo (Principle IX).

**Scale/Scope**:
- Pico declarado (assumption da spec): dezenas de aberturas de ticket por segundo somadas entre orgs multi-agente; acúmulo transitório de milhares de mensagens durante degradação do SQS.
- Número de orgs multi-agente: **não declarado** — o desenho não depende dele (estruturas globais para delayed e inflight; `active` cresce com orgs com backlog).
- Capacidade mínima de workers: ver seção "Capacidade" abaixo.

## Constitution Check

*GATE: passou antes da pesquisa; reavaliado após o desenho (Phase 1).*

Constituição: `.specify/memory/constitution.md` v1.0.0.

| Princípio | Como o plano cumpre | Status |
|-----------|---------------------|--------|
| I. Version Control and Review | PR para `main`, revisão e CI verde. | ✅ |
| II. Security and Secrets | Nenhum segredo novo. Fixtures de SQS com credenciais falsas. | ✅ |
| III. Never Trust the Client | Mensagens da fila (`PublishTask`) são validadas ao decodificar; `meta` opcional tratado com cuidado. Corpo SQS é gerado internamente. | ✅ |
| IV. Observability | Logs estruturados via `logrus` (R12); sem URN; sem métricas novas (exigência da spec, compatível com IV, que pede métricas pelos hooks existentes — não adicionamos). | ✅ |
| V. Diagnosable Errors | Erros de desistência, de persistência e `slow_delivery` carregam `ticket_uuid`, `org_id`, `correlation_id`; sem URN no Sentry. | ✅ |
| VI. Versioned Contracts | Corpo e atributos SQS inalterados (contrato de teste). Envelope Redis ganha `meta` opcional; tarefas antigas aceitas (rolling deploy). Campos de log documentados como interface. Nova entrada no `WENI-CHANGELOG.md`. | ✅ |
| VII. Fail Gracefully | Timeout de 5 s mantido em cada `SendMessage`; `ProcessingTTL` como deadline de lease; falha de persistência não confirma a tarefa; sem panic exposto. | ✅ |
| VIII. Bounded Retry Over REST | Retries limitados por `SqsPublishMaxAttempts`; reclaim conta como tentativa (R9); desistência registrada em log de erro com identificadores para reenvio manual pelo banco (Q2 = não). A constituição aceita "recorded in logs". | ✅ |
| IX. Scalability and Peak Load | Estado no Redis; leases e promoção com guard `ZREM`; pico declarado; capacidade de workers documentada (ver abaixo). | ✅ |
| X. Tests Exercise Flows | Fluxos HTTP e flow, sucesso, falha, retry, reinício, esgotamento, paralelismo — testes listados em quickstart. Caminhos de erro cobertos. | ✅ |
| XI. Explicit Over Clever | Campos de log em um módulo; transições em scripts nomeados; `TaskSource` explícito; literais em constantes. Comentários explicam o porquê (guard por `ZREM`, score = `queued_on`). | ✅ |
| XII. Contained Changes | Mudanças restritas à publicação SQS, ao cliente SQS e ao ponto de injeção do foreman. Outras filas (`send_history`, `handler`, `rabbitmq_publish`) não mudam. Mudança de `workers.go` limitada a fonte de tarefa e redação. | ✅ (justificado em Complexity Tracking) |
| XIII. Specification Traceability | Spec com bloco "Inheritance from Product Spec" no formato exigido. | ✅ |
| XIV. No Silent Divergence | Divergências em relação à spec estão explícitas: (a) semântica de tentativas mantida (`Max` = reenvios), spec corrigida; (b) `event` acrescentado a FR-015; (c) recuperação de queda depende de `ProcessingTTL` (R10), aprovada em 2026-11-27; (d) ordem por despacho, decisão Q1 = A, com SC-005 reescrito na spec. Itens (b) a (d) estão aplicados na spec. | ✅ |
| XV. Commit Messages | Conventional Commits; atômicos (ex.: `fix: ...`, `feat: ...`, `test: ...`, `docs: ...`). | ✅ |
| XVI. Changelog Maintenance | `WENI-CHANGELOG.md` com entrada para o envelope/comportamento de entrega e para os campos de log. | ✅ |

**Gates**: nenhuma violação de MUST sem justificativa. O item XIV está pendente de decisão do usuário, não de violação.

## Capacidade (Principle IX)

Vazão de um worker = `1 / latência do envio`. Vazão total = `SqsPublishWorkers / latência`.

| Cenário | Latência | Workers (padrão 2) | Vazão | Workers necessários para 50 msg/s |
|---------|----------|--------------------|-------|-----------------------------------|
| Normal | 100 ms | 2 | 20 msg/s | 5 |
| Normal | 50 ms | 2 | 40 msg/s | 3 |
| Degradado (timeouts) | 20 s (pior caso com reconexão) | 2 | 0,1 msg/s | 1000 (inviável) |

**Conclusão**: com o padrão de 2 workers, o pico declarado só é atendido se o SQS responder em ~40 ms ou menos. **Antes de produção, o time deve dimensionar `SqsPublishWorkers`** com a fórmula `pico × p95 de envio`. Não é uma mudança de código; é ajuste de configuração de implantação (sem novo nome). Em degradação severa, a vazão é limitada pelo número de workers por design — a spec já restringe o SC-002 a operação normal.

## Project Structure

### Documentation (this feature)

```text
specs/005-fix-sqs-ticket-publish/
├── spec.md
├── plan.md                  # este arquivo
├── research.md              # Phase 0
├── data-model.md            # Phase 1
├── quickstart.md            # Phase 1
├── contracts/
│   ├── sqs-ticket-message.md
│   └── publish-logs.md
├── checklists/
│   └── requirements.md
└── tasks.md                 # NÃO criado aqui (saída de /speckit-tasks)
```

### Source Code (arquivos afetados)

```text
runtime/sqs/
├── client.go                       # MODIFICADO: RWMutex só para ponteiro + geração; connectMu para reconexão; rede fora de lock
└── client_test.go                  # NOVO: falha isolada, reconexão única, envio lento não bloqueia

core/queue/
└── (sem mudança)                   # AddTask/PopNextTask/send_history preservados (XII)

core/tasks/sqs/
├── store.go                        # NOVO: scripts Lua (enqueue, pop, ack, reschedule, promote, reclaim) e chaves Redis
├── store_test.go                   # NOVO: atomicidade, guard por ZREM, active sem negativos, ordem com retry
├── source.go                       # NOVO: implementação de TaskSource para sqs_publish (pop atômico, Release no-op)
├── worker.go                       # MODIFICADO: handler usa store (ack/reschedule), sem goroutine; logs via logs.go
├── worker_test.go                  # MODIFICADO: testes antigos de requeue por AddTask são reescritos para o novo comportamento
├── producer.go                     # MODIFICADO: EnqueuePublish* usam enqueue atômico e recebem meta
├── producer_test.go                # MODIFICADO
├── logs.go                         # NOVO: constantes de campo e funções de evento (único ponto de log)
├── logs_test.go                    # NOVO: formato de tempo, ausência de URN
├── maintenance.go                  # NOVO: InitFunction com reclaim na inicialização e loop de promote/reclaim
├── maintenance_test.go             # NOVO
└── publishers/
    ├── tickets.go                  # MODIFICADO: assinatura com origin; sem message_group_id nos logs; meta
    └── tickets_test.go             # MODIFICADO: teste de contrato do corpo e dos atributos

mailroom.go                         # MODIFICADO: RegisterTaskSource (ou equivalente) e config de manutenção
workers.go                          # MODIFICADO: foreman usa TaskSource; Release; redação do corpo bruto para sqs_publish
workers_processing_test.go          # MODIFICADO: garante que send_history continua usando o caminho padrão
core/queue/queue.go                 # SEM MUDANÇA (apenas verificação de regressão)

web/ticket/
├── open.go                         # MODIFICADO: origin = "http"
└── open_test.go                    # MODIFICADO/NOVO: fluxo HTTP enfileira com meta e logs

core/handlers/
├── ticket_opened.go                # MODIFICADO: origin = "flow"
└── ticket_opened_test.go           # MODIFICADO/NOVO: fluxo flow enfileira com meta e logs

WENI-CHANGELOG.md                   # MODIFICADO: entrada da versão de release
```

**Structure Decision**: a lógica de fila de `sqs_publish` fica em `core/tasks/sqs` (domínio do publicador), não em `core/queue`, para manter a mudança contida (XII) e não afetar `send_history`, `handler` e `rabbitmq_publish`. O único ponto compartilhado é a injeção de `TaskSource` no foreman da raiz.

## Desenho detalhado

### Fluxo de enfileiramento

1. `web/ticket/open.go` ou `core/handlers/ticket_opened.go` chama `publishers.PublishTicketCreated(rt, orgID, msg, origin)`.
2. `publishers` gera `correlation_id`, monta `meta` e chama `sqs.EnqueuePublishWithAttributes`.
3. `store.enqueue` executa um script: `ZADD sqs_publish:<org> queued_on <task-json>` + `ZINCRBY sqs_publish:active 0 <org>`.
4. Log `enqueued` (ou `enqueued` com `result=failed` em erro).

### Fluxo de envio (worker)

1. Foreman pede a tarefa à fonte `sqs_publish`: `store.pop` (script) retira o head do org de menor `active`, registra o lease e devolve a tarefa com `LeaseToken`.
2. Handler `handleSqsPublish`:
   - decodifica `PublishTask`, deriva `meta` (com fallback para tarefas antigas); grava `meta.first_attempt_at` se ainda vazio;
   - log `attempt`;
   - `rt.SQS.SendTo…` (sem lock global);
   - **sucesso**: `store.ack(token)` e log `sent` com `created_to_sent_ms` e `queued_to_sent_ms`; se `sent_at - created_on > sqsSlowDeliveryThreshold`, também um log `slow_delivery` em nível `error` (R16);
   - **falha com `attempt < Max`**: `store.reschedule(token, next_attempt_json, next_attempt_at)`; log `retry_scheduled`; se o script falhar, log `retry_persist_failed` e **não** confirma (o lease expira e a mensagem é recuperada);
   - **falha com `attempt >= Max`**: `store.ack(token)` com desistência; log `dead_lettered`; persistência conforme Q2.
3. Foreman: `Release` é no-op para `sqs_publish`; `MarkTaskComplete` não é chamado.

### Fluxo de manutenção (InitFunction)

- Na inicialização (síncrono, antes de os foremen iniciarem): `reclaim` uma vez. Log `recovered` se houver itens.
- Loop a cada 1 s (constante): `promote` (lote de até 100 itens de `delayed` com score <= agora) e `reclaim` (lote de até 100 leases com deadline <= agora).
- `promote` por item: se `ZREM delayed member` == 1, `ZADD sqs_publish:<org> queued_on member` e `ZINCRBY active 0 org`.
- `reclaim` por item: se `ZREM inflight token` == 1, incrementa `attempt`; se `attempt <= Max`, `ZADD sqs_publish:<org> queued_on novo-json` + `ZINCRBY active -1`; senão desistência (log `dead_lettered`, sem armazenamento extra).

### Tratamento de erro em cada operação Redis

| Operação | Falha | Efeito |
|----------|-------|--------|
| enqueue | Redis indisponível | retorna erro; log `enqueued` com `result=failed` e nível error; a origem (HTTP/flow) decide se propaga (ver nota) |
| pop | erro | foreman registra erro e tenta de novo (comportamento atual) |
| ack | erro | lease permanece; reclaim após `ProcessingTTL`; possível duplicata (at-least-once) |
| reschedule | erro | `retry_persist_failed`; lease permanece; reclaim |
| promote | erro | item continua em `delayed`; próxima passada tenta |
| reclaim | erro | próxima passada tenta |

**Nota sobre enqueue**: hoje `PublishTicketCreated` retorna erro de enfileiramento, que o chamador ignora (`_ = …` na linha de flow e `sqsPublishers.PublishTicketCreated(...)` sem tratamento em `open.go`). Manter esse comportamento (não bloquear a abertura do ticket por SQS), mas registrar o erro em nível `error` com os campos de L1 (FR-009).

### Ordem e retry (RF3, FR-005)

- Score de elegível = `queued_on`, preservado em todos os caminhos.
- Retry sai de `delayed` com o mesmo `queued_on` e entra no zset do org na posição original.
- Ordem de despacho dentro do org = ordem de `queued_on`, exceto quando o retry está em `delayed` (exceção prevista na spec).

### Concorrência (RF1)

- `runtime/sqs/client.go`:
  - `mu RWMutex` protege `svc` e `gen` apenas durante leitura/troca do ponteiro.
  - `connectMu Mutex` serializa somente reconexões.
  - `publish` lê `(svc, gen)` sob `RLock`, solta e chama `SendMessage` (timeout de 5 s).
  - Em erro, `reconnect(gen)`: sob `connectMu`, se `c.gen != gen` já houve reconexão, apenas repete com o cliente atual; senão constrói `LoadDefaultConfig` fora de `mu`, troca sob `Lock` e incrementa `gen`.
- `Close()` troca `svc` por nil sob `Lock`; envios em voo mantêm sua referência.

## Plano de testes (Principle X)

Listados com IDs em [quickstart.md](quickstart.md). Resumo:

- Fluxos: HTTP e flow enfileiram; sucesso; falha com retry; retry com reinício; reclaim com lease expirado; desistência; recuperação na inicialização; paralelismo com envio lento.
- Contrato: corpo e atributos iguais ao fixture pré-mudança.
- Logs: campos obrigatórios por evento; ausência de URN em todos os caminhos (incluindo pânico e tarefa longa).
- Compatibilidade: tarefa antiga sem `meta` é processada.
- Regressão: `send_history`, `handler` e `rabbitmq_publish` seguem com `PopNextTask` (teste existente `TestWorkerProcessingOnlyForSendHistory` continua verde).
- Gates: `gofmt`, `go vet`, `go test ./... -p=1`.

## Riscos

| Risco | Probabilidade | Impacto | Mitigação |
|-------|---------------|---------|-----------|
| Ordem no fio quebrada por paralelismo (R7) | certa, se a fila for FIFO | médio | Decisão Q1 |
| Recuperação de queda leva até `ProcessingTTL` (R10) | baixa (queda) | baixo (cauda do 95%) | Documentar; Q/instância se necessário |
| Workers insuficientes para o pico (Capacidade) | média | alto | Dimensionar antes do rollout |
| Script Lua com `TIME` em Redis antigo | baixa | alto | Verificar versão de produção |
| Duplicata por lease expirado com worker vivo | baixa | baixo | At-least-once aceito; consumidor deduplica |
| Log com URN por caminho não coberto | média | alto (LGPD/Sentry) | Teste de varredura em todos os caminhos (Q-06) |
| Transação do flow revertida após publicação (A10) | não medida | médio | Fora de escopo; registrar item separado |
| Rajada de `slow_delivery` ao fim de uma degradação longa | média | baixo a médio (ruído no Sentry) | Mensagem fixa agrupa em um problema; volume não limitado pela spec; reavaliar com dados reais |
| `slow_delivery` não detecta mensagens paradas | certa | médio | Limitação aceita pela spec (sem varredura contínua); desistência cobre o caso de esgotar tentativas |
| Rolling deploy: versão antiga ainda pop sem lease | média (durante deploy) | baixo | Versões antiga e nova convivem no mesmo layout de `sqs_publish:<org>`; tarefas em voo da versão antiga não são recuperadas — aceitável por janela curta |

## Complexity Tracking

> Violações e desvios a justificar

| Desvio | Por que é necessário | Alternativa mais simples rejeitada porque |
|--------|----------------------|-------------------------------------------|
| Duas estruturas globais novas no Redis (`delayed`, `inflight`) | RF2, RF3 e RF4 exigem retry persistido na posição original e recuperação de tarefas não confirmadas | Score único (`next_attempt_at`) viola RF3; leases por org (como `send_history`) ficam invisíveis quando o org sai de `active` (A5) |
| Scripts Lua para todas as transições | Eliminar janelas de perda entre comandos (A3, A6) | `MULTI/EXEC` com `WATCH` exige retry sob contenção e fica mais difícil de testar |
| `TaskSource` injetado no foreman (toca `workers.go`) | Pop atômico e confirmação pelo handler exigem que o foreman não faça `MarkTaskComplete` para `sqs_publish` | Condicional por nome de fila dentro do foreman espalha regra de domínio pelo framework |
| Campo `LeaseToken` em `queue.Task` (não persistido) | Passar o token do pop até o handler sem mudar a assinatura de `TaskFunction` | Mudar `TaskFunction` afeta todas as filas |
| Reclaim conta como tentativa | Limitar loops quando o processo cai sempre no mesmo envio (VIII) | Reclaim sem contagem permitiria loop infinito |

## Decisões tomadas

- **Q1 — Ordem entre workers = A** (R7): paralelismo total; ordem garantida é a de despacho dentro do org. SC-005 reescrito na spec.
- **Q2 — Desistência persistida = não** (R9): só log de erro com identificadores; reenvio manual reconstrói pelo banco. Nenhuma estrutura Redis extra para desistidas.

## Ajustes de spec aplicados

1. Semântica de `SqsPublishMaxAttempts` como reenvios (FR-003 e FR-010 corrigidos).
2. SC-005 reescrito para ordem de despacho (Q1 = A).
3. FR-015: `event` acrescentado à lista de campos (R12) — **aplicado**.
3b. FR-017 e SC-007 adicionados (entrega lenta ao Sentry), com FR-016 e o fora de escopo reconciliados — **aplicado**.
4. Edge case de recuperação explicita que a recuperação de queda ocorre em até `ProcessingTTL` (R10) — **aplicado** (aprovado em 2026-11-27).

## Pendências antes de `/speckit-tasks`

- [x] Aprovação do time para a recuperação de queda em até `ProcessingTTL` (R10). Aprovado em 2026-11-27: recuperação por expiração de lease, sem identidade de instância.
- [x] Verificar a versão do Redis de produção (>= 5 para `TIME` em script). Resolvido: produção roda Redis 6.2.6.
- [x] Dimensionar `SqsPublishWorkers` para o pico declarado (ver Capacidade). Mecanismo confirmado: configurado por variável de ambiente, sem código. Valor final a definir no deploy.
- [x] Confirmar se `SqsTicketsQueueURL` de produção é FIFO. Resolvido: não é FIFO. Sem `MessageGroupId`/`MessageDeduplicationId`; ordem e deduplicação ficam com a decisão A e com o consumidor (por `ticket_uuid`).

## Done (Phase 1)

- [x] Pesquisa com decisões e alternativas (research.md)
- [x] Modelo de dados e máquina de estados (data-model.md)
- [x] Contratos: corpo SQS inalterado e logs (contracts/)
- [x] Guia de validação (quickstart.md)
- [x] Decisões Q1 e Q2 registradas
- [ ] tasks.md (próximo comando, após as pendências acima)
