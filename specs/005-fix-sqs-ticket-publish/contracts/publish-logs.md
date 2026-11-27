# Contrato: logs de publicação de tickets no SQS

**Feature**: `005-fix-sqs-ticket-publish` | **Requisitos**: L1–L8, FR-009 a FR-016 | **Pesquisa**: R12

Este contrato é a interface de **consulta operacional**: equipes de operação e engenharia filtram estes campos. Mudar um nome aqui quebra consultas existentes, então é tratado como interface versionada (Principle VI).

## Formato geral

- Logs estruturados via `logrus` (campos JSON).
- Todo registro do fluxo tem: `comp = "sqs_publish"`, `event`, `result` (quando aplicável) e os campos de identificação abaixo.
- Tempos: string ISO 8601 em UTC com milissegundos, formato `2006-01-02T15:04:05.000Z`.
- Durações: inteiro, milissegundos (`*_ms`).
- **Proibido**: `contact_urn`, `message_group_id`, `Attributes`, corpo bruto da tarefa, qualquer valor que contenha a URN.

## Campos padronizados

| Campo | Tipo | Presente em | Descrição |
|-------|------|-------------|-----------|
| `event` | string | todos | `enqueued`, `attempt`, `sent`, `slow_delivery`, `retry_scheduled`, `retry_persist_failed`, `dead_lettered`, `recovered` |
| `result` | string | tentativa, sucesso, falha, desistência | `sent`, `failed`, `rescheduled`, `dead_lettered` |
| `ticket_uuid` | UUID | todos | Chave de consulta 1 |
| `correlation_id` | UUID | todos | Chave de consulta 2 |
| `org_id` | int | todos | Org |
| `queue_url` | string | todos | Fila de destino |
| `origin` | string | `enqueued`, e quando disponível nos demais | `http`, `flow` ou `unknown` |
| `attempt` | int | tentativa, sucesso, falha, desistência | 1 no envio inicial; máximo `SqsPublishMaxAttempts + 1` |
| `created_on` | time | `enqueued` | Instante de criação do ticket (evento) |
| `queued_on` | time | `enqueued`, `sent`, `retry_scheduled` | Instante do enfileiramento original (preservado) |
| `enqueued_at` | time | `enqueued` | Instante do enfileiramento confirmado |
| `attempted_at` | time | `attempt` (início), `retry_scheduled`, `dead_lettered` | Início da tentativa |
| `sent_at` | time | `sent` | Instante do envio bem-sucedido |
| `next_attempt_at` | time | `retry_scheduled` | Horário previsto da próxima tentativa |
| `created_to_sent_ms` | int | `sent` | `sent_at - created_on` |
| `queued_to_sent_ms` | int | `sent` | `sent_at - queued_on` |
| `error` | string | falhas e desistência | Mensagem do erro (sem corpo da tarefa) |
| `recovered_count` | int | `recovered` | Quantidade recuperada nesta varredura |
| `recovered` | lista | `recovered` | Itens: `ticket_uuid`, `correlation_id`, `org_id`, `queue_url` |

## Eventos

### `enqueued` (nível info)

Campos obrigatórios: `event`, `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `origin`, `created_on`, `queued_on`, `enqueued_at`.
Se o enfileiramento falhar: nível `error`, mesmos campos, `error`, `result = "failed"` (FR-009).

### `attempt` (nível info)

Emitido antes de cada chamada ao SQS. Campos: `event`, `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `attempt`, `attempted_at`.
`result` é emitido no evento de desfecho, não aqui (evita duplicar linha por tentativa).

### `sent` (nível info)

Campos: `event`, `result = "sent"`, `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `attempt`, `sent_at`, `created_on`, `queued_on`, `created_to_sent_ms`, `queued_to_sent_ms`.

### `slow_delivery` (nível error, vai ao Sentry; FR-017)

Emitido **uma vez**, logo depois de `sent`, somente quando `sent_at - created_on` é **estritamente maior** que 5 minutos (constante `sqsSlowDeliveryThreshold`). Mensagem de log fixa: `sqs ticket delivery exceeded threshold`. Não há `result` (o resultado já está em `sent`).

| Campo | Notas |
|-------|-------|
| `event` | `slow_delivery` |
| `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `origin`, `attempt` | identificação |
| `created_on`, `queued_on`, `sent_at` | três marcos do percurso |
| `created_to_queued_ms` | tempo **antes** da fila (flow, banco, enfileiramento) |
| `queued_to_sent_ms` | tempo total na fila, incluindo retries |
| `created_to_sent_ms` | soma dos dois |
| `slow_threshold_ms` | `300000` |
| `first_attempt_at` | se conhecido |
| `queued_to_first_attempt_ms` | espera por worker ou backlog; só se `first_attempt_at` conhecido |
| `first_attempt_to_sent_ms` | tempo gasto em retries e recuperação; só se `first_attempt_at` conhecido |

**Como ler**: `created_to_queued_ms` alto indica atraso no caminho de abertura (fora do escopo de correção). `queued_to_first_attempt_ms` alto indica backlog ou workers insuficientes. `first_attempt_to_sent_ms` alto, com `attempt > 1`, indica retries ou recuperação após queda.

**Sentry (Principle V)**: só identificadores opacos e tempos; sem URN, sem corpo. A mensagem fixa agrupa as ocorrências em um problema; cada ocorrência traz os campos acima como dados extras.

### `retry_scheduled` (nível warn)

Falha com reenvios restantes. Campos: `event`, `result = "rescheduled"`, `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `attempt`, `error`, `attempted_at`, `next_attempt_at`, `queued_on`.

### `retry_persist_failed` (nível error)

A gravação do reagendamento no Redis falhou. A mensagem permanece em voo até o lease expirar. Campos: mesmos de `retry_scheduled`, mais `error` da gravação e `result = "failed"`.

### `dead_lettered` (nível error, vai ao Sentry)

Limite atingido. Campos: `event`, `result = "dead_lettered"`, `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `attempt`, `attempted_at`, `error`.
Sentry (Principle V): apenas identificadores opacos e erro; sem URN, sem corpo. Não há armazenamento extra: o reenvio manual é feito a partir do banco com `ticket_uuid` e `org_id` (Q2 = não).

### `recovered` (nível warn)

Emitido pela varredura de reclaim (inicialização e periódica) quando recupera pelo menos um item. Campos: `event`, `recovered_count`, `recovered` (lista). Não é emitido com contagem zero.

## Redação

- O campo `task` (corpo bruto) é removido dos logs de erro, pânico e tarefa longa do foreman quando o tipo é `sqs_publish`.
- Teste obrigatório: capturar todos os logs do fluxo com um hook e falhar se qualquer campo contiver a URN de teste (ver [quickstart.md](../quickstart.md), cenário Q-06).

## Exemplos de consulta

```bash
# Linha do tempo por ticket
jq -c 'select(.ticket_uuid=="d2f852ec-7b4e-457f-ae7f-f8b243c49ff5")' mailroom.log

# Linha do tempo por correlation_id, somente eventos finais
jq -c 'select(.correlation_id=="<uuid>" and (.event=="sent" or .event=="dead_lettered"))' mailroom.log
```
