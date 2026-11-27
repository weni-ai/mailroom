# Tasks: Publicação confiável, sem atrasos em cascata e rastreável por logs para eventos de criação de ticket

**Input**: Design documents de `/specs/005-fix-sqs-ticket-publish/` (plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md)

**Prerequisites**: plan.md (obrigatório), spec.md (obrigatório para as user stories), research.md, data-model.md, contracts/, quickstart.md

**Tests**: incluídos. O quickstart.md define os cenários Q-01 a Q-13 como critérios de aceite ("Todos os cenários Q-01 a Q-13 passam") e a constituição (Principle X) exige testes de fluxos e de caminhos de erro.

**Organization**: tarefas agrupadas por user story (US1 P1, US2 P2, US3 P3) para implementação e teste independentes.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: pode rodar em paralelo (arquivos diferentes, sem dependência de tarefas incompletas)
- **[Story]**: história de usuário à qual a tarefa pertence (US1, US2, US3)
- Caminhos de arquivo são relativos à raiz do repositório `mailroom/`

## Pré-requisitos antes da implementação (pendências do plan.md, não são tarefas de código)

- ~~**Aprovação do time para a recuperação de queda em até `ProcessingTTL` (R10 / Principle XIV item c)**~~ **Aprovado em 2026-11-27**: recuperação por expiração de lease, sem identidade de instância (opção mais simples).
- ~~**Versão do Redis de produção >= 5**~~ **Resolvido**: produção roda Redis 6.2.6, acima do mínimo exigido por `redis.call('TIME')` (R4/R5).
- ~~**Dimensionar `SqsPublishWorkers`**~~ **Mecanismo confirmado**: o valor é configurado por variável de ambiente, sem mudança de código. O valor final para o pico declarado (ver "Capacidade" no plan.md) ainda precisa ser definido na configuração de deploy.
- ~~**Confirmar se `SqsTicketsQueueURL` de produção é FIFO**~~ **Resolvido**: a fila de produção **não** é FIFO. Logo, `MessageGroupId` e `MessageDeduplicationId` não são enviados, o SQS não garante ordem nem deduplica reenvios, e a deduplicação fica com o consumidor por `ticket_uuid` (premissa da spec). A decisão Q1 (opção A) é mantida.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: baseline e esqueleto de constantes internas

- [ ] T001 Registrar baseline verde antes de qualquer alteração: `go test ./runtime/sqs/... ./core/tasks/sqs/... ./core/queue/... ./web/ticket/... ./core/handlers/... -p=1 -count=1`
- [ ] T002 Criar `core/tasks/sqs/maintenance.go` com as constantes internas nomeadas `sqsPublishMaintenanceTick = time.Second` e `sqsPublishMaintenanceBatch = 100` (sem novos nomes de configuração em `runtime/config.go`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: estado persistido no Redis, envelope, fonte de tarefas no foreman e logs centralizados. MUST estar completo antes de qualquer user story.

**⚠️ CRITICAL**: nenhuma tarefa de user story começa antes desta fase.

- [ ] T003 Capturar fixture do contrato SQS **antes de qualquer mudança**: corpo JSON, `ContentType`, `MessageGroupId` (FIFO), `CorrelationID` e `MessageDeduplicationId` gerados pelo código atual, em `core/tasks/sqs/publishers/testdata/ticket_sqs_contract.json` (cenário Q-03; base para T016)
- [ ] T004 [P] Criar `core/tasks/sqs/logs.go`: constantes de campo (`event`, `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `origin`, `attempt`, `result`, `created_on`, `queued_on`, `enqueued_at`, `attempted_at`, `sent_at`, `next_attempt_at`, `created_to_sent_ms`, `queued_to_sent_ms`, `recovered_count`, `recovered`, `error`), valores de `event` e `result`, formato de tempo `2006-01-02T15:04:05.000Z` em UTC, constante `sqsSlowDeliveryThreshold = 5 * time.Minute`, mensagem fixa `sqs ticket delivery exceeded threshold` e uma função por evento (único ponto de log de SQS). Não incluir `contact_urn`, `message_group_id` nem corpo bruto (contracts/publish-logs.md, R12)
- [ ] T005 [P] Criar `core/tasks/sqs/store.go`: chaves Redis (`sqs_publish:<org_id>`, `sqs_publish:active`, `sqs_publish:delayed`, `sqs_publish:inflight`, `sqs_publish:inflight:payload`) e scripts Lua nomeados para `enqueue`, `pop`, `ack`, `reschedule`, `promote` e `reclaim`, com guard por `ZREM == 1`, `active` com clamp em zero e `TIME` do Redis para deadlines (data-model.md, R3, R4, R5)
- [ ] T006 [P] Em `core/queue/queue.go`, adicionar ao struct `Task` o campo `LeaseToken string` com tag `json:"-"` (não persistido; preenchido no pop). Mudança não-quebrante; os demais campos e funções permanecem iguais (ver nota de divergência no plan.md, "Complexity Tracking")
- [ ] T007 Criar `core/tasks/sqs/store_test.go`: atomicidade de cada script, guard por `ZREM` (dois atores não consomem o mesmo token), `active` nunca negativo, `enqueue` atômico (item e org em `active` juntos) (depende de T005)
- [ ] T008 Modificar `workers.go`: introduzir a interface `TaskSource` (`Pop(rc) (*queue.Task, error)` e `Release(rc, task) error`) e a fonte padrão que reproduz o comportamento atual (`PopNextTask` + `EndProcessing` + `MarkTaskComplete`); o foreman usa a fonte registrada para sua fila (depende de T006; R8)
- [ ] T009 Modificar `mailroom.go`: adicionar `RegisterTaskSource(queueName string, src TaskSource)` no mesmo estilo de `AddTaskFunction` (depende de T008)
- [ ] T010 Criar `core/tasks/sqs/source.go`: implementação de `TaskSource` para `sqs_publish` (pop atômico via script de T005, preenche `LeaseToken`, `Release` é no-op) e registro em `init` via `mailroom.RegisterTaskSource` (depende de T005, T008, T009; R8)
- [ ] T011 Modificar `core/tasks/sqs/producer.go`: adicionar `Meta *PublishMeta` (`json:"meta,omitempty"`) em `PublishTask` com `ticket_uuid`, `created_on`, `origin`, `correlation_id` e `first_attempt_at`; `EnqueuePublishWithAttributes` passa a usar o `enqueue` atômico de T005 e aceita `meta` (depende de T004, T005; data-model.md, R13)
- [ ] T012 Modificar `core/tasks/sqs/publishers/tickets.go`: assinatura `PublishTicketCreated(rt, orgID, msg, origin)`, gera `correlation_id`, monta `meta`, e remove `message_group_id` dos logs existentes (A7); corpo e atributos SQS permanecem iguais (depende de T011, T003)
- [ ] T013 Atualizar os dois chamadores para passar a origem: `web/ticket/open.go` com `origin = "http"` e `core/handlers/ticket_opened.go` com `origin = "flow"` (depende de T012)
- [ ] T014 Verificar regressão de outras filas: `TestWorkerProcessingOnlyForSendHistory` em `workers_processing_test.go` continua verde sem alteração de expectativa (`send_history`, `handler` e `rabbitmq_publish` usam a fonte padrão) (depende de T008, T009; Q-12)

**Checkpoint**: fundação pronta. Estado no Redis, fonte de tarefas, envelope com `meta` e logs centralizados existem. User stories podem começar.

---

## Phase 3: User Story 1 - Eventos de criação de ticket não se perdem em falhas ou reinícios (Priority: P1) 🎯 MVP

**Goal**: toda mensagem aceita para publicação chega ao SQS, mesmo com falhas transitórias, reinício ou queda; esgotamento de tentativas gera erro com identificadores para reenvio manual; corpo e atributos não mudam.

**Independent Test**: abrir tickets para org multi-agente com falha temporária no SQS, interrupção do worker durante o envio e reinício durante a espera do retry. Nenhuma mensagem perdida; retry respeita a posição original; esgotamento gera `dead_lettered`; corpo e atributos iguais ao fixture.

### Tests for User Story 1 ⚠️

> **Escrever estes testes antes da implementação (T020, T021) e confirmar que falham.**

- [ ] T015 [P] [US1] Criar `core/tasks/sqs/maintenance_test.go`: Q-08 — (a) lease ainda válido não é recuperado; (b) lease expirado é recuperado com `attempt + 1`, mesmo corpo e mesmos atributos. Q-04 — `promote` devolve o retry à elegível na posição de `queued_on` original, à frente de tarefa enfileirada depois dela no mesmo org
- [ ] T016 [P] [US1] Modificar `core/tasks/sqs/publishers/tickets_test.go`: Q-03 — corpo comparado com `JSONEq` ao fixture de T003; atributos iguais; para a fila de produção (padrão, não FIFO) os atributos enviados são só `ContentType` e `CorrelationID`, sem `MessageGroupId` nem `MessageDeduplicationId`; os atributos FIFO continuam cobertos para filas `.fifo` (FR-008, US1 cenário 6)
- [ ] T017 [US1] Modificar `core/tasks/sqs/worker_test.go`: Q-04 — falha com reenvio restante sai de `inflight` e entra em `delayed` com `attempt = 1` e `queued_on` original; segundo envio termina em `sent` com `attempt = 2`. Q-11 — tarefa antiga sem `meta` é enviada normalmente
- [ ] T018 [US1] Modificar `core/tasks/sqs/worker_test.go`: Q-05 — falha ao persistir o reagendamento (Redis falha no script de reschedule) não confirma a tarefa, mantém o lease e emite `retry_persist_failed` em `error`
- [ ] T019 [US1] Modificar `core/tasks/sqs/worker_test.go`: Q-07 — com `SqsPublishMaxAttempts = 1` e SQS sempre 500, a tarefa é desistida após 2 envios, `inflight` fica vazio, `active` não fica negativo e nenhuma estrutura Redis extra é criada. Substitui `TestHandleSqsPublish_MaxRetriesReached`

### Implementation for User Story 1

- [ ] T020 [US1] Modificar `core/tasks/sqs/worker.go` (`handleSqsPublish`): remover a goroutine com `time.Sleep` e a chamada a `queue.AddTask`; sucesso → `store.ack`; falha com `attempt < SqsPublishMaxAttempts` → `store.reschedule` com `attempt + 1`; falha de persistência → não confirma; `attempt >= SqsPublishMaxAttempts` → `store.ack` com desistência (depende de T010; ver plan.md, "Fluxo de envio")
- [ ] T021 [P] [US1] Implementar em `core/tasks/sqs/maintenance.go`: `InitFunction` com `reclaim` síncrono antes de os foremen iniciarem e loop a cada `sqsPublishMaintenanceTick` com `promote` e `reclaim` em lotes de `sqsPublishMaintenanceBatch`; registrar com `mailroom.AddInitFunction`; executa só se `SqsPublishEnabled` e `rt.SQS != nil` (depende de T002, T005; R10, R11)

**Checkpoint**: US1 funcional e testável de forma independente (Q-03, Q-04, Q-05, Q-07, Q-08, Q-11 passam).

---

## Phase 4: User Story 2 - Lentidão ou falha pontual não trava os demais envios (Priority: P2)

**Goal**: os workers publicam em paralelo; um envio lento ou uma reconexão não serializa os demais; ordem de despacho preservada dentro de cada org.

**Independent Test**: `SqsPublishWorkers = 2`, `httptest` que segura uma requisição por 3 s e responde as demais; a tarefa saudável termina em menos de 1 s; 10 falhas simultâneas geram uma única reconstrução do cliente.

### Tests for User Story 2 ⚠️

- [ ] T022 [P] [US2] Modificar `runtime/sqs/client_test.go`: Q-10 — 10 envios falham ao mesmo tempo → exatamente uma reconstrução (`gen` incrementa uma vez); envios que não falharam não esperam a reconstrução; nenhum envio bloqueado por mutex de rede; `Close()` com envio em voo não quebra o envio

### Implementation for User Story 2

- [ ] T023 [US2] Modificar `runtime/sqs/client.go`: `mu sync.RWMutex` protege apenas `svc` e `gen`; `connectMu sync.Mutex` serializa somente reconexões; `publish` lê `(svc, gen)` sob `RLock`, solta e chama `SendMessage` com timeout de 5 s; `reconnect(gen)` constrói o cliente com `config.LoadDefaultConfig` fora de `mu` e troca sob `Lock` com `gen++`; `Close()` troca por nil sob `Lock` (depende de T022; R1)
- [ ] T024 [P] [US2] Modificar `core/tasks/sqs/store_test.go`: ordem de despacho dentro do org — pop devolve a menor `queued_on` primeiro; orgs distintos são independentes (US2 cenário 4; SC-005)
- [ ] T025 [US2] Modificar `core/tasks/sqs/worker_test.go`: Q-09 — `SqsPublishWorkers = 2`, `httptest` segura uma requisição por 3 s; a tarefa saudável de outra org termina em menos de 1 s enquanto a lenta continua em andamento (depende de T017–T019; SC-003)

**Checkpoint**: US1 e US2 funcionam de forma independente (Q-09 e Q-10 passam).

---

## Phase 5: User Story 3 - Diagnóstico de ponta a ponta por consulta aos logs (Priority: P3)

**Goal**: consultar por `ticket_uuid` ou `correlation_id` mostra toda a linha do tempo (enfileiramento com origem, tentativas, envio ou desistência, recuperações) com os mesmos nomes de campo e sem URN; entregas acima de 5 minutos geram um único `slow_delivery`.

**Independent Test**: abrir um ticket por `http` e outro por `flow`; provocar falha seguida de sucesso, esgotamento e recuperação. Consultar por `ticket_uuid` e `correlation_id`; todas as etapas e durações aparecem; nenhuma URN em texto claro.

### Tests for User Story 3 ⚠️

- [ ] T026 [P] [US3] Criar `core/tasks/sqs/logs_test.go`: formato de tempo (UTC com milissegundos), duração em inteiro de milissegundos, nomes de campo fixos e ausência de `contact_urn` e `message_group_id` em todos os eventos de `logs.go` (Q-06, FR-015, FR-016)
- [ ] T027 [P] [US3] Modificar `web/ticket/open_test.go`: Q-01 pela via HTTP — o enfileiramento tem `meta.origin = "http"` e o log `enqueued` contém `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `origin`, `created_on`, `queued_on`, `enqueued_at` (FR-009)
- [ ] T028 [P] [US3] Modificar `core/handlers/ticket_opened_test.go`: Q-01 pela via flow — mesmo conjunto de campos com `origin = "flow"` (FR-009)
- [ ] T029 [P] [US3] Modificar `workers_processing_test.go`: Q-06 — nos caminhos de pânico no handler, tarefa longa e erro de enfileiramento, nenhum registro capturado contém a URN de teste (`whatsapp:5511999999999`) nem `message_group_id`
- [ ] T030 [US3] Modificar `core/tasks/sqs/worker_test.go`: Q-13 — (a) 4 min 59 s e (b) exatamente 5 min não geram `slow_delivery`; (c) 5 min 1 s com `attempt = 3` e `first_attempt_at` gera exatamente um registro `error` com `event = "slow_delivery"`, mensagem fixa e campos `created_to_queued_ms`, `queued_to_sent_ms`, `created_to_sent_ms`, `slow_threshold_ms = 300000`, `queued_to_first_attempt_ms`, `first_attempt_to_sent_ms`; sem `first_attempt_at`, os dois últimos são omitidos e o registro ainda sai; nenhum registro contém a URN (sequencial no arquivo após T017–T019 e T025)
- [ ] T031 [US3] Modificar `core/tasks/sqs/worker_test.go`: Q-06 — varredura de logs nos caminhos de sucesso, retry, desistência e reclaim; nenhum registro contém a URN nem o corpo bruto (depende de T030)

### Implementation for User Story 3

- [ ] T032 [P] [US3] Modificar `core/tasks/sqs/producer.go`: emitir o log `enqueued` (nível info) após a confirmação do script, ou `enqueued` com `result = "failed"` em nível `error` quando o enfileiramento falhar; usar apenas funções de `logs.go` (depende de T004, T011; FR-009)
- [ ] T033 [US3] Modificar `core/tasks/sqs/worker.go`: emitir `attempt` antes de cada `SendMessage` (com `attempted_at`), `sent` (com `sent_at`, `created_to_sent_ms`, `queued_to_sent_ms`), `retry_scheduled` (com `attempted_at`, `next_attempt_at`, `error`), `retry_persist_failed` e `dead_lettered` (nível `error`, com `error` e `attempted_at`), usando apenas `logs.go` (depende de T020; FR-010 a FR-013)
- [ ] T034 [US3] Modificar `core/tasks/sqs/worker.go`: gravar `meta.first_attempt_at` no início da primeira tentativa, carregá-lo em `reschedule` e `reclaim`, e após `sent` emitir `slow_delivery` (nível `error`, uma vez) quando `sent_at - created_on > sqsSlowDeliveryThreshold`, com a decomposição de tempo de FR-017 (depende de T033; FR-017, R16)
- [ ] T035 [P] [US3] Modificar `core/tasks/sqs/maintenance.go`: emitir `recovered` (nível warn) com `recovered_count` e a lista `ticket_uuid`, `correlation_id`, `org_id`, `queue_url` por item, somente quando a contagem é maior que zero, tanto na inicialização quanto no loop (depende de T021; FR-014)
- [ ] T036 [P] [US3] Modificar `workers.go`: para tarefas `sqs_publish`, remover o campo `task` (corpo bruto) dos logs de erro, pânico e tarefa longa (A8; FR-016) (depende de T008, T009)

**Checkpoint**: US1, US2 e US3 funcionam de forma independente (Q-01, Q-06, Q-13 passam; `ticket_uuid` e `correlation_id` reconstroem a linha do tempo).

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: documentação, validação completa e fechamento

- [ ] T037 [P] Atualizar `WENI-CHANGELOG.md` com a entrada da versão de release: novo envelope (`meta` opcional), comportamento de entrega (paralelismo, retry persistido, recuperação) e contrato de campos de log (Principle XVI)
- [ ] T038 [P] Revisar `core/tasks/sqs/logs.go` e `core/tasks/sqs/worker.go` contra `specs/005-fix-sqs-ticket-publish/contracts/publish-logs.md`: nomes de campo, valores de `event`/`result` e níveis de log conferem com o contrato
- [ ] T039 Executar `gofmt -l . && go vet ./...` e `go test ./... -p=1`; todos verdes (quickstart.md, Critérios de aceite)
- [ ] T040 Executar os comandos dos pacotes afetados do quickstart.md (`go test ./runtime/sqs/... ./core/tasks/sqs/... ./core/queue/... -p=1 -count=1 -v` e `go test ./web/ticket/... ./core/handlers/... -p=1 -count=1 -run TicketOpened -v`) e confirmar que `runtime/config.go` não recebeu novos nomes de configuração

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: sem dependências.
- **Foundational (Phase 2)**: depende de Setup; **bloqueia todas as user stories**.
- **User Stories (Phase 3+)**: dependem de Foundational. US1 é o MVP. US2 e US3 podem começar depois de Foundational, mas US2 e US3 tocam `worker_test.go` e `worker.go`, então, na prática, seguem a ordem de prioridade.
- **Polish (Phase 6)**: depende das user stories desejadas.

### User Story Dependencies

- **US1 (P1)**: começa após Foundational. Sem dependência de US2 ou US3.
- **US2 (P2)**: começa após Foundational. Usa `worker_test.go` de US1 (T025 após T017–T019).
- **US3 (P3)**: começa após Foundational. Os logs de `worker.go` (T033, T034) dependem da implementação de US1 (T020).

### Within Each User Story

- Testes são escritos antes da implementação e devem falhar antes dela.
- Implementação do store e da fonte antes do handler.
- Handler antes da emissão de logs.

### Parallel Opportunities

- Setup: T001 e T002 em sequência.
- Foundational: T004, T005 e T006 em paralelo (arquivos diferentes). T003 antes de T012.
- US1: T015 e T016 em paralelo com T021. T017–T019 e T020 em sequência (`worker_test.go` e `worker.go`).
- US2: T022 e T024 em paralelo. T023 depois de T022.
- US3: T026, T027, T028, T029, T032, T035 e T036 em paralelo (arquivos diferentes). T030 e T031 em sequência (`worker_test.go`), depois de T033 e T034.
- Polish: T037 e T038 em paralelo.

---

## Parallel Example: User Story 1

```bash
# Testes e maintenance em paralelo (arquivos diferentes):
Task: "Q-08 e Q-04 em core/tasks/sqs/maintenance_test.go"          # T015
Task: "Q-03 contrato em core/tasks/sqs/publishers/tickets_test.go" # T016
Task: "Implementar loop de manutenção em core/tasks/sqs/maintenance.go" # T021
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Completar Phase 1: Setup (T001–T002).
2. Completar Phase 2: Foundational (T003–T014). **CRÍTICO**: bloqueia as stories.
3. Completar Phase 3: US1 (T015–T021).
4. **PARAR e validar**: Q-03, Q-04, Q-05, Q-07, Q-08, Q-11.
5. Antes do deploy, resolver as pendências de pré-requisitos.

### Incremental Delivery

1. Setup + Foundational → fundação pronta.
2. US1 → testar de forma independente → MVP (nenhuma perda em falhas e reinícios).
3. US2 → testar de forma independente → paralelismo e isolamento de lentidão.
4. US3 → testar de forma independente → diagnóstico por logs e `slow_delivery`.
5. Polish → changelog, gates e validação do quickstart.

---

## Notes

- Tarefas [P] = arquivos diferentes, sem dependência de tarefas incompletas.
- [Story] mapeia cada tarefa à user story para rastreabilidade.
- Divergência em relação ao plan.md: a seção "Source Code" do plan marca `core/queue/queue.go` como "SEM MUDANÇA", mas a seção "Complexity Tracking" exige o campo `LeaseToken` em `queue.Task`, que está nesse arquivo. T006 adota a segunda leitura, que é a única que funciona; a alteração é não-quebrante.
- Commit após cada tarefa ou grupo lógico (Principle XV, Conventional Commits).
- Pare em cada checkpoint para validar a story de forma independente.
- Evite: tarefas vagas, dois agentes editando o mesmo arquivo em paralelo, e dependências cruzadas entre stories.
