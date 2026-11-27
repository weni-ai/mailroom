# Quickstart: validação da publicação de tickets no SQS

**Feature**: `005-fix-sqs-ticket-publish` | **Plan**: [plan.md](plan.md)

Guia para validar a feature ponta a ponta, localmente e em staging. Não contém código de implementação.

## Pré-requisitos

- Go 1.23.5.
- PostgreSQL com banco `mailroom_test` (ver `testsuite`).
- Redis em `localhost:6379` (os testes de fila usam o banco 0 e chaves `test:*`, `qproc:*`, `unitq:*`).
- RabbitMQ (exigido pelo `testsuite`).
- Nenhum SQS real: os testes usam `httptest.Server`.

## Comandos

```bash
# formatação e análise estática
gofmt -l . && go vet ./...

# suíte completa (serial, como exige a constituição)
go test ./... -p=1

# pacotes afetados
go test ./runtime/sqs/... ./core/tasks/sqs/... ./core/queue/... -p=1 -count=1 -v
go test ./web/ticket/... ./core/handlers/... -p=1 -count=1 -run TicketOpened -v
```

## Cenários de validação

Cada cenário indica os requisitos cobertos e onde o teste deve viver. Todos os cenários de falha são obrigatórios (Principle X).

### Q-01 — Enfileiramento pelas duas origens (FR-009, L1)

- **Dado** uma org com `is_multi_agents = true` e publicação habilitada.
- **Quando** um ticket é aberto por `POST /mr/ticket/open` e outro por evento `ticket_opened`.
- **Então**:
  - a elegível `sqs_publish:<org>` tem duas tarefas com `meta.origin` = `http` e `flow`;
  - o log `enqueued` de cada uma contém `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `origin`, `created_on`, `queued_on`, `enqueued_at` em UTC com milissegundos.
- **Local**: `web/ticket/open_test.go`, `core/handlers/ticket_opened_test.go`.

### Q-02 — Envio com sucesso e medição (L2, L3, FR-011)

- **Dado** `httptest` respondendo 200.
- **Quando** o foreman processa a tarefa.
- **Então**:
  - o `httptest` recebe uma requisição com corpo e atributos iguais ao fixture (Q-03);
  - logs `attempt` (com `attempt = 1`) e `sent` com `sent_at`, `created_to_sent_ms` e `queued_to_sent_ms`;
  - `inflight` e `inflight:payload` ficam vazios; `active[org]` volta a 0.
- **Local**: `core/tasks/sqs/worker_test.go`.

### Q-03 — Contrato do corpo e dos atributos (RF6, FR-008)

- **Dado** um fixture gerado antes da mudança (corpo JSON, `ContentType`, `MessageGroupId`, `CorrelationID`, e `MessageDeduplicationId` para fila `.fifo`).
- **Quando** a mesma abertura é publicada depois da mudança.
- **Então** o corpo é igual com `JSONEq` e os atributos são iguais; `MessageGroupId` continua presente só em fila FIFO.
- **Local**: `core/tasks/sqs/publishers/tickets_test.go`, `runtime/sqs/client_test.go`.

### Q-04 — Falha com reenvio persistido (RF2, RF3, FR-004, L4)

- **Dado** `httptest` respondendo 500 na primeira chamada e 200 depois, `SqsPublishDelayIntervalMs = 0` e `SqsPublishMaxAttempts = 3`.
- **Quando** a tarefa falha e é reagendada.
- **Então**:
  - a tarefa sai de `inflight` e entra em `delayed` com `attempt = 1` e `queued_on` original;
  - log `retry_scheduled` com `attempted_at` e `next_attempt_at`;
  - após o promote, a tarefa volta à elegível **na posição de `queued_on` original**, antes de uma tarefa enfileirada depois dela no mesmo org (teste de ordem com duas tarefas de mesmo org);
  - o segundo envio termina em `sent` com `attempt = 2`.
- **Local**: `core/tasks/sqs/store_test.go`, `maintenance_test.go`.

### Q-05 — Falha ao persistir o reagendamento (RF2)

- **Dado** um Redis que falha no script de reschedule (simulado por chave de teste ou por um wrapper de conexão).
- **Quando** o envio falha.
- **Então**: log `retry_persist_failed` em nível `error` com campos de L4; a tarefa **não** é confirmada (continua em `inflight`); depois de expirar o lease, o reclaim a recupera.
- **Local**: `core/tasks/sqs/worker_test.go`.

### Q-06 — Nenhuma URN nos logs (L8, FR-016, SC-006)

- **Dado** a URN `whatsapp:5511999999999` nos dados da tarefa.
- **Quando** o fluxo é executado nos caminhos: sucesso, falha com retry, desistência, reclaim, pânico no handler, tarefa longa e erro de enfileiramento.
- **Então**: nenhum registro capturado pelo hook contém a string da URN nem `message_group_id`.
- **Local**: `core/tasks/sqs/logs_test.go`, cenário de pânico em `workers_processing_test.go`.

### Q-07 — Esgotamento de tentativas (RF5, FR-007, FR-013, L5)

- **Dado** `SqsPublishMaxAttempts = 1` e `httptest` sempre com 500.
- **Quando** a tarefa falha duas vezes.
- **Então**:
  - log `dead_lettered` em `error` com `ticket_uuid`, `org_id`, `correlation_id`, `queue_url`, `attempt = 2`, `attempted_at`, `error`;
  - nenhuma nova tentativa agendada; `inflight` vazio; `active` sem negativos.
  - nenhuma estrutura Redis extra é criada para a desistência (Q2 = não).
- **Local**: `core/tasks/sqs/worker_test.go` (substitui `TestHandleSqsPublish_MaxRetriesReached`).

### Q-08 — Recuperação por lease expirado e na inicialização (RF4, FR-006, L6)

- **Dado** uma tarefa retirada com `ProcessingTTL` curto (ex.: 1 s em teste) e o processo "caído" (sem ack).
- **Quando**:
  - (a) a varredura roda antes do deadline: a tarefa **não** é recuperada (proteção de instâncias vivas);
  - (b) depois do deadline: a varredura recupera, emite `recovered` com `recovered_count = 1` e os identificadores, incrementa `attempt` e a reenvia.
- **Então** o envio posterior usa o mesmo corpo e os mesmos atributos.
- **Local**: `core/tasks/sqs/maintenance_test.go`.

### Q-09 — Paralelismo e lentidão isolada (RF1, FR-001, SC-003)

- **Dado** `SqsPublishWorkers = 2` e `httptest` que segura **uma** requisição por 3 s e responde as demais imediatamente.
- **Quando** duas tarefas de orgs diferentes (ou do mesmo org em modo A) são processadas.
- **Então**: a tarefa saudável termina em menos de 1 s, enquanto a lenta ainda está em andamento.
- **Local**: `core/tasks/sqs/worker_test.go`.

### Q-10 — Reconexão sem lock global (RF1, FR-002)

- **Dado** um cliente SQS cujo `svc` falha.
- **Quando** 10 envios falham ao mesmo tempo.
- **Então**: exatamente uma reconstrução do cliente acontece (`gen` incrementa uma vez); envios que não falharam não aguardam a reconstrução; nenhum envio é bloqueado por um `Mutex` de rede.
- **Local**: `runtime/sqs/client_test.go`.

### Q-11 — Compatibilidade com tarefas antigas (Principle VI)

- **Dado** uma tarefa `sqs_publish` gravada no formato anterior (sem `meta`, com `attempt`).
- **Quando** o foreman a processa.
- **Então**: envio normal; logs com `origin = "unknown"` e `ticket_uuid`/`created_on` derivados do corpo; sem erro.
- **Local**: `core/tasks/sqs/worker_test.go`.

### Q-13 — Relatório de entrega lenta (FR-017, SC-007, R16)

- **Dado** tarefas com `created_on` ajustado em relógio de teste (ou injetado no `meta`) e um hook de `logrus` que captura entradas.
- **Quando** três mensagens são enviadas com sucesso: (a) 4 min 59 s depois de `created_on`; (b) exatamente 5 min; (c) 5 min 1 s depois, com `attempt = 3` e `first_attempt_at` conhecido.
- **Então**:
  - (a) e (b): nenhuma entrada `slow_delivery`;
  - (c): exatamente uma entrada `slow_delivery` em nível `error`, mensagem `sqs ticket delivery exceeded threshold`, com `created_to_queued_ms`, `queued_to_sent_ms`, `created_to_sent_ms`, `slow_threshold_ms = 300000`, `queued_to_first_attempt_ms` e `first_attempt_to_sent_ms` coerentes com os marcos;
  - a entrada não contém a URN (cenário Q-06 deve incluir este caminho);
  - com `first_attempt_at` ausente (tarefa antiga), os dois últimos campos são omitidos e a entrada ainda é emitida.
- **Verificação do hook do Sentry**: teste de unidade que confirma que a entrada tem nível `error` (único requisito para o hook atual de `cmd/mailroom/main.go` encaminhá-la). Não chamar o Sentry real.
- **Local**: `core/tasks/sqs/worker_test.go`, `core/tasks/sqs/logs_test.go`.

### Q-12 — Regressão de outras filas (XII)

- **Dado** o teste existente `TestWorkerProcessingOnlyForSendHistory`.
- **Quando** a suíte roda.
- **Então** continua verde sem alteração de expectativa: `send_history` registra processamento; outras filas não.
- **Local**: `workers_processing_test.go`.

## Validação em staging (manual, após deploy)

1. Abrir um ticket por cada origem em uma org multi-agente de teste.
2. Consultar os logs pelo `ticket_uuid` e confirmar a linha do tempo: `enqueued` -> `attempt` -> `sent`, com `queued_to_sent_ms` coerente.
3. Consultar por `correlation_id` e confirmar o mesmo resultado.
4. Reiniciar o serviço com mensagens na fila de retry e confirmar que a mensagem é enviada depois do intervalo.

### Exemplos de consulta

```bash
jq -c 'select(.ticket_uuid=="<uuid>")' mailroom.log
jq -c 'select(.correlation_id=="<uuid>" and (.event=="sent" or .event=="dead_lettered"))' mailroom.log
jq -c 'select(.event=="recovered")' mailroom.log
```

## Critérios de aceite

- Todos os cenários Q-01 a Q-13 passam.
- `gofmt`, `go vet` e `go test ./... -p=1` verdes.
- Nenhum novo nome de configuração em `runtime/config.go`.
- `WENI-CHANGELOG.md` atualizado.
- Decisões Q1 e Q2 registradas em `research.md`, e a spec ajustada conforme "Pós-implementação" do `plan.md`.
