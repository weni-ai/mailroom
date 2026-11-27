# Data Model: Publicação de eventos de criação de ticket

**Feature**: `005-fix-sqs-ticket-publish` | **Research**: [research.md](research.md)

Nenhuma tabela, coluna ou migração PostgreSQL. Toda a mudança de estado está no Redis, que já é o backend da fila (Assumptions da spec). O corpo e os atributos enviados ao SQS não mudam (ver [contracts/sqs-ticket-message.md](contracts/sqs-ticket-message.md)).

## Entidades

### PublishTask (envelope da tarefa `sqs_publish`)

Payload JSON dentro de `queue.Task.Task`. Continua compatível com o formato atual.

| Campo | Tipo | Origem | Obrigatório | Notas |
|-------|------|--------|-------------|-------|
| `queue_url` | string | `rt.Config.SqsTicketsQueueURL` | sim | Já existe. |
| `content_type` | string | `msg.ContentType()` | sim | Já existe. `application/json`. |
| `body` | JSON raw | `msg.Marshal()` | sim | `TicketSQSMessage`. Já existe. Contém `contact_urn`: nunca logar. |
| `attributes` | map[string]string | `MessageGroupId`, `CorrelationID` | não | Já existe. `MessageGroupId` contém a URN: nunca logar. |
| `attempt` | int | incrementado a cada reenvio ou reclaim | sim (0 no envio inicial) | Já existe. Limite: `attempt < SqsPublishMaxAttempts` para reenviar. |
| `meta` | objeto | novo | não | `ticket_uuid`, `created_on`, `origin`, `correlation_id`, `first_attempt_at`. `omitempty`. `first_attempt_at` é gravado pelo handler no início da primeira tentativa e carregado em reschedule e reclaim (R16). |

**Regras**:
- `attempt` começa em `0`. Log usa `attempt + 1`.
- `meta.origin` ∈ {`http`, `flow`, `unknown`}. `unknown` só para tarefas antigas.
- Ausência de `meta` é válida (rolling deploy); os campos de log são derivados do corpo e de `attributes`.

### Task (envelope genérico `queue.Task`)

| Campo | Uso no SQS |
|-------|------------|
| `type` | `sqs_publish` |
| `org_id` | org da mensagem; chave da fila elegível |
| `task` | `PublishTask` acima |
| `queued_on` | **Preservado** entre enfileiramento, retry, reclaim e recuperação. Base do score e de `queued_to_sent_ms`. |
| `error_count` | não usado pelo SQS (usar `attempt`) |
| `lease_token` | **novo, não persistido** (`json:"-"`): preenchido no pop, usado em ack/reschedule. |

### Lease (tarefa em voo)

| Estrutura Redis | Chave | Score / valor | Ciclo |
|-----------------|-------|---------------|-------|
| zset | `sqs_publish:inflight` | member = `lease_token`, score = deadline (ms, relógio Redis) | criado no pop; removido por ack, reschedule ou reclaim (somente se `ZREM` retornar 1) |
| hash | `sqs_publish:inflight:payload` | field = `lease_token`, value = JSON da `Task` | criado no pop; removido junto com o zset |

- `lease_token`: UUID v4 gerado pelo script ou pelo Go antes do pop. Não é hash de conteúdo (diferente de `taskFingerprint`), para que duas tarefas idênticas não colidam.
- Deadline = `now + ProcessingTTL` (s). Invariante: `ProcessingTTL` > pior caso de envio (~20 s).

### Fila elegível (por org)

| Estrutura | Chave | Score | Member |
|-----------|-------|-------|--------|
| zset | `sqs_publish:<org_id>` | `queued_on` em segundos, 6 casas | JSON da `Task` |
| zset | `sqs_publish:active` | contagem de leases do org | `org_id` |

- Ordem de retirada: menor score primeiro (`queued_on`), dentro do org.
- `active` lista orgs com elegíveis ou leases. Um org sai de `active` apenas quando sua elegível está vazia (comportamento atual de `popTask`) e volta com `enqueue`, `promote` ou `reclaim`.

### Retry agendado

| Estrutura | Chave | Score | Member |
|-----------|-------|-------|--------|
| zset (global) | `sqs_publish:delayed` | `next_attempt_at` (epoch s, relógio Redis) | JSON da `Task` com `attempt` já incrementado; `queued_on` original |

- Promovido para a elegível do org quando `score <= now`, preservando `queued_on` como score.

### Desistência (sem armazenamento, decisão Q2)

Não existe estrutura Redis para mensagens desistidas. A desistência é um evento de log `dead_lettered` (nível error) com `ticket_uuid`, `org_id` e `correlation_id`. O reenvio manual reconstrói a mensagem a partir do banco.

## Máquina de estados de uma mensagem

```text
            enqueue
  (nova) ───────────────► ELEGÍVEL(org) ──pop──► EM VOO(lease)
                              ▲  ▲                  │
          promote (vencido)   │  │ reclaim          ├─ ack(sucesso) ──────────► ENVIADA (fim)
          DELAYED ────────────┘  │ (lease expirado, │
             ▲                   │  attempt+1)      ├─ ack(desistência) ──────► DESISTIDA (fim, somente log)
             │                   │                  │
             └──── reschedule ───┴──────────────────┘   (falha com reenvios restantes:
                   (ZREM==1, attempt+1)                  ZREM inflight + ZADD delayed)
```

**Invariantes**:
1. Uma mensagem está em exatamente um estado: elegível, delayed, em voo ou finalizada. Transições são atômicas.
2. Só o ator que remove o token de `inflight` (`ZREM` == 1) pode mudar o estado de uma mensagem em voo. Isso impede envio duplicado por dois atores diferentes além do at-least-once permitido.
3. `queued_on` nunca muda depois do enqueue inicial.
4. `active[org]` nunca fica negativo (clamp em zero na escrita).
5. `attempt` só cresce; ao atingir o limite a mensagem é finalizada ou dead-lettered.

## Validações

| Regra | Origem | Onde |
|-------|--------|------|
| `queue_url` não vazio | comportamento atual | `sqs.SendTo` |
| `attempt < SqsPublishMaxAttempts` para reenviar | FR-003 / A9 | script de reschedule ou Go antes de chamar |
| `ProcessingTTL` > pior caso de envio | R9 | teste de configuração / documentação |
| `org_id` > 0 | comportamento atual | enqueue |

## Logs (consulta, não armazenamento)

Ver [contracts/publish-logs.md](contracts/publish-logs.md). Não há tabela de log; a consulta é feita sobre os logs estruturados emitidos pelo processo.
