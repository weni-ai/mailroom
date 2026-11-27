# Research: Publicação confiável e rastreável de eventos de criação de ticket (SQS)

**Feature**: `005-fix-sqs-ticket-publish` | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

Este documento resolve as decisões técnicas do plano. Onde a decisão depende de uma escolha de produto, a opção recomendada aparece como padrão e a pergunta está marcada como **ABERTA** no plano.

## Achados do código atual (confirmados)

| # | Achado | Onde | Consequência |
|---|--------|------|--------------|
| A1 | `publish` e `publishWithAttributes` seguram `c.mu` durante toda a chamada de rede (timeout de 5s). `reconnect` usa o mesmo lock. | `runtime/sqs/client.go` (linhas ~160–240) | Serialização global dos workers (causa 1 da spec). |
| A2 | O retry é uma goroutine com `time.Sleep` e chama `queue.AddTask`, que recalcula o score com `time.Now()` e recria `QueuedOn`. O erro de `AddTask` é descartado (`_ =`). | `core/tasks/sqs/worker.go` (linhas ~62–75) | Retry vai para o fim da fila do org (RF3) e perde o `queued_on` original; perda em reinício (RF2). |
| A3 | `PopNextTask` remove a tarefa do zset e só depois o worker processa. Para `sqs_publish` não há registro em processamento. `BeginProcessing` só é chamado para `send_history`, e em outra chamada Redis (janela de perda). | `core/queue/queue.go` (`popTask`), `workers.go` (`handleTask`) | Perda em queda do processo (causa 3 / RF4). |
| A4 | `RequeueExpired` só roda no reaper do `handler` (`if f.queue == queue.HandlerQueue`) e reenfileira com `AddTaskAt` usando `now + delay` como score, o que também quebra a ordem. | `workers.go` (linhas ~66–80), `core/queue/queue.go` | Recuperação não serve para `sqs_publish` e reordena. |
| A5 | `popTask` remove o org do `active` quando o zset dele está vazio. Um org com atraso (delayed) sem item eleível sairia do `active` e nunca mais seria descoberto. | `core/queue/queue.go` (`popTask`) | Qualquer solução com fila de atraso precisa manter o org descoberto ou separar a descoberta. |
| A6 | `AddTask` faz `zadd` e `zincrby active` em pipeline sem `MULTI`/script. Queda entre os dois comandos deixa o item no zset sem o org em `active`, e o item fica preso. | `core/queue/queue.go` (`AddTask`) | Janela de perda no enfileiramento. Deve virar uma única operação atômica. |
| A7 | `publishers/tickets.go` loga `message_group_id`, que é `ProjectUUID:ChannelUUID:ContactURN`. | `core/tasks/sqs/publishers/tickets.go` | Vazamento de URN em log (viola L8 e FR-016). Precisa ser removido. |
| A8 | `workers.go` loga o corpo bruto da tarefa (`"task", string(task.Task)`) em erro, pânico e tarefa longa. Para `sqs_publish` o corpo contém a URN. | `workers.go` (linhas ~240, ~281, ~291) | Mesmo vazamento por outro caminho. Precisa de redação para o tipo `sqs_publish`. |
| A9 | Retry de `sqs_publish` respeita `SqsPublishMaxAttempts` como número de **reenvios**: com `Attempt` iniciando em 0, o envio inicial mais até `N` reenvios (N+1 envios no total). O teste `TestHandleSqsPublish_MaxRetriesReached` confirma. | `core/tasks/sqs/worker.go`, `worker_test.go` | A especificação foi corrigida neste plano para manter essa semântica. |
| A10 | `ticket_opened` (flow) enfileira dentro de `HandleEvents(ctx, rt, tx, ...)`, antes do commit da transação. | `core/handlers/ticket_opened.go`, `core/models/events.go` | Se a transação do fluxo for revertida, a mensagem já pode ter sido publicada. Fora de escopo (a spec não pede), registrado como risco. |
| A11 | Config padrão: `SqsPublishWorkers = 2`, `SqsPublishMaxAttempts = 10`, `SqsPublishDelayIntervalMs = 2000`, `ProcessingTTL = 120` (s). `SqsTicketsQueueURL` padrão não termina em `.fifo`. | `runtime/config.go` | `MessageGroupId` só é enviado para filas FIFO (`isFIFOQueue`). Com fila padrão, o SQS não garante ordem. Ver R7. |

## R1. Lock do cliente SQS: snapshot + geração para reconexão

- **Decision**: trocar o `sync.Mutex` por `sync.RWMutex` que protege apenas o ponteiro `svc` e um contador `gen`. Um `connectMu` separado serializa somente as tentativas de reconexão. Envios leem `(svc, gen)` sob `RLock`, soltam o lock e só então chamam `SendMessage`. Ao falhar com `gen = g`, o caller chama `reconnect(g)`: sob `connectMu`, se `c.gen != g` outra goroutine já reconectou e ele apenas tenta de novo com o cliente atual; caso contrário constrói o novo cliente (`config.LoadDefaultConfig`, que pode ir ao IMDS) **fora** do lock de estado e troca o ponteiro sob `Lock` com `gen++`.
- **Rationale**: envios saudáveis nunca esperam por reconexão nem por outro envio (RF1). Uma falha gera no máximo uma reconstrução do cliente, mesmo com vários workers falhando juntos (evita a tempestade de reconexões).
- **Alternatives considered**:
  - Manter um lock, mas só para a troca do ponteiro: resolve a serialização, mas não evita reconexões repetidas em cascata.
  - Um cliente por worker: mais conexões e mais estado, sem ganho de correção.
  - `singleflight` do `x/sync`: resolveria a deduplicação, mas esconde o contador de geração que queremos para decidir se a reconexão ainda é necessária.
- **Compatibilidade de testes**: `&sqsclient.Client{}` (zero value) continua válido, pois `RWMutex` e contadores têm zero value utilizável.

## R2. Política de reconexão

- **Decision**: manter a política atual (reconectar e tentar uma vez de novo em qualquer erro de envio), mas com R1. **Recomendação registrada, não aplicada neste plano**: avaliar não reconectar em `context.DeadlineExceeded`, porque um timeout de 5s seguido de reconexão de até 10s dobra o tempo de ocupação do worker. Ver "Capacidade no pior caso" no `plan.md`.
- **Rationale**: a spec pede que falhas de conexão sejam recuperadas. Mudar o critério de reconexão altera comportamento e merece validação com o time de infraestrutura.

## R3. Modelo de filas no Redis

- **Decision**: manter o layout existente por org e adicionar duas estruturas globais, sem alterar o formato da tarefa:
  - `sqs_publish:<org>` (zset, **existente**): mensagens **elegíveis**, score = `queued_on` (segundos com 6 casas, mesmo formato de `AddTask`). É a chave de ordenação.
  - `sqs_publish:active` (zset, **existente**): orgs com mensagens elegíveis ou em processamento; score = contagem de workers ocupados.
  - `sqs_publish:delayed` (zset global, **novo**): retries aguardando `next_attempt_at`. Score = `next_attempt_at`. Member = JSON da tarefa.
  - `sqs_publish:inflight` (zset global, **novo**): leases de tarefas retiradas e não confirmadas. Score = deadline (ms). Member = token da tarefa.
  - `sqs_publish:inflight:payload` (hash, **novo**): token -> JSON da tarefa, para recuperação.
- **Rationale**: separar "elegível" de "aguardando" faz o retry voltar ao score original (`queued_on`), o que cumpre RF3 sem gambiarra de score. Uma única estrutura de leases global simplifica a recuperação, porque não depende de o org continuar em `active` (ver A5).
- **Alternatives considered**:
  - Retry com score = `next_attempt_at` no mesmo zset: viola RF3, porque mensagens que chegaram entre a original e o retry passariam na frente.
  - Retry com score = `queued_on` e checagem de `next_attempt_at` na hora do pop: a mensagem em retry bloquearia o topo da fila até o horário; ou pularia itens e quebraria a ordem.
  - Leases por org (`sqs_publish:<org>:processing`, como em `send_history`): a recuperação precisaria varrer orgs, e um org fora de `active` ficaria invisível (A5).

## R4. Operações atômicas (Lua) e fronteiras de confirmação

- **Decision**: toda transição de estado é uma operação atômica no Redis. Operações de escrita relevantes:
  1. **enqueue** (`ZADD sqs_publish:<org> queued_on token-json` + `ZINCRBY active 0 org`) em um único script (corrige A6).
  2. **pop** (`ZRANGE` da menor entrada de `active`, `ZREM` do head da elegível, `ZADD inflight deadline token`, `HSET payload`, `ZINCRBY active +1`) em um único script. Assim não existe mais o intervalo "retirada da fila e ainda não registrada" (A3).
  3. **ack** (sucesso ou desistência): `ZREM inflight token`; se retornar 1, `HDEL payload` e `ZINCRBY active -1` (com clamp em zero).
  4. **reschedule** (falha com reenvios restantes): se `ZREM inflight token` == 1, `ZADD delayed next_attempt_at novo-json` e `ZINCRBY active -1` no mesmo script. Se a gravação falhar, o script não executa e o lease permanece, então a mensagem é recuperada depois (RF2 / RF4).
  5. **promote** (retry vencido): por item, `ZREM delayed member` == 1 então `ZADD sqs_publish:<org> queued_on member` + `ZINCRBY active 0 org`.
  6. **reclaim** (lease expirado): por token, `ZREM inflight token` == 1 então, conforme o número de tentativas, `ZADD sqs_publish:<org> queued_on novo-json` (com `attempt + 1`) + `ZINCRBY active -1`, ou desistência (R9).
- **Regra de posse**: toda transição que tira um token de `inflight` depende do `ZREM` retornar 1. Assim exatamente um ator vence (worker que confirma, worker que reagenda, ou recuperador), e `active` nunca é decrementado duas vezes. Isso resolve a corrida entre um worker lento que passou do lease e o recuperador.
- **Rationale**: transições atômicas eliminam as janelas de perda identificadas. O guard por `ZREM` é o mecanismo de exclusão entre instâncias (Principle IX: instâncias sem estado local).
- **Alternatives considered**:
  - Transações `MULTI/EXEC` com `WATCH`: funcionam, mas exigem retry no cliente sob contenção; Lua é mais simples de testar.
  - Lock distribuído por Redis (`SET NX`) em volta de cada transição: mais round-trips e um novo ponto de falha, sem ganho sobre o guard por `ZREM`.
- **Compatibilidade de versão**: scripts que chamam `redis.call('TIME')` exigem Redis >= 5 com replicação por efeitos (padrão a partir de 5.0). **Verificar a versão do Redis de produção** antes do merge.

## R5. Relógio: Redis ou processo?

- **Decision**: para deadlines de lease e para a comparação de `next_attempt_at`, os scripts usam `redis.call('TIME')`. Os timestamps de log (`enqueued_at`, `attempted_at`, `sent_at`, `next_attempt_at`) continuam vindos do relógio do processo (UTC, milissegundos).
- **Rationale**: instâncias com relógios diferentes não podem divergir sobre quando um lease expirou (Edge case "múltiplas instâncias" da spec). O log usa o relógio local, que é aceitável para diagnóstico; a diferença de skew entre hosts é registrada como limitação.
- **Alternatives considered**: relógio local em tudo: mais simples, mas um host com relógio adiantado reclama leases de outros.

## R6. Ordem: retry na frente das mensagens posteriores (RF3)

- **Decision**: o score da elegível é sempre `queued_on` (preservado no envelope de retry, reclaim e recuperação). Um retry promovido volta para o zset com o score original, então é retirado antes de qualquer mensagem enfileirada depois da original no mesmo org.
- **Rationale**: é a única forma de cumprir RF3 sem reescrever o score do envelope. `queued_on` também é a base de `queued_to_sent_ms` (FR-011), então o tempo de fila de um retry inclui todo o período desde o enfileiramento original.

## R7. Ordem de envio entre workers (DECIDIDA: opção A, em 2026-11-27)

- **Problema**: com `SqsPublishWorkers > 1`, dois workers podem retirar duas mensagens do mesmo org ao mesmo tempo. Nesse caso, a ordem de **chegada ao SQS** não é garantida, mesmo com a ordem de **retirada** preservada. SC-005 ("exata ordem de chegada dentro do org") conflita com RF1 (paralelismo).
- **Fatos que reduzem o problema**: a fila padrão `flows-tickets` não é FIFO (A11). **Confirmado em 2026-11-27: a fila de produção não é FIFO.** Em fila padrão, o SQS não garante ordem de qualquer forma, e `MessageGroupId` nem é enviado. Se a produção usar fila `.fifo`, o SQS ordena apenas dentro de `MessageGroupId` (contato), não dentro do org.
- **Opções**:
  - **A (recomendada)**: paralelismo total. A ordem garantida é a **ordem de despacho** (retirada do zset), não a ordem no fio. SC-005 é reescrito como "ordem de despacho dentro do org". Melhor vazão.
  - **B**: no máximo uma mensagem em voo por org. Ordem no fio garantida por org, ao custo de vazão limitada a `1 / latência` por org (≈ 10 msg/s com 100 ms; insuficiente para um org que estoure o pico).
  - **C**: no máximo uma mensagem em voo por `MessageGroupId`. Alinha com o que um FIFO garante; ordem por org não é garantida no fio.
- **Decisão**: A. A ordem garantida dentro do org é a **ordem de despacho** (retirada do zset por `queued_on`), com exceção prevista para retries em espera. SC-005 é reescrito na spec. Se a fila de produção for FIFO, o SQS ainda ordena dentro de `MessageGroupId` por contato, o que é mais forte que o exigido.

## R8. Ponto de integração do foreman

- **Decision**: o `Foreman` recebe uma **fonte de tarefas** (`TaskSource`) com duas operações: `Pop(rc) (*queue.Task, error)` (que preenche um campo não persistido `LeaseToken` em `queue.Task`) e `Release(rc, task)`. A fonte padrão reproduz o comportamento atual (`PopNextTask` + `EndProcessing` + `MarkTaskComplete`). A fonte de `sqs_publish` faz o pop atômico de R4 e tem `Release` como no-op, porque o handler já executou ack ou reschedule.
- **Registro**: o pacote `mailroom` (raiz) expõe `RegisterTaskSource(queueName, src)`, no mesmo estilo de `AddTaskFunction`. O pacote `core/tasks/sqs` registra a sua fonte em `init`. Isso evita importação circular, porque `core/tasks/sqs` já importa `mailroom`.
- **Rationale**: mantém os outros foremen intocados (Principle XII). O único desvio é o caminho de `sqs_publish`, explícito no código.
- **Alternatives considered**:
  - Condicionais `if f.queue == queue.SqsPublish` dentro do foreman: funcionam, mas espalham regras de domínio pelo framework de workers (XI).
  - Mover todo o worker de SQS para fora do foreman: duplica o gerenciamento de workers, `PendingTasks` e shutdown.
- **Efeito colateral obrigatório**: `handleTask` não pode chamar `MarkTaskComplete` para `sqs_publish`, senão `active` é decrementado duas vezes (R4).

## R9. Esgotamento, recuperação e limite de tentativas

- **Semântica de tentativas** (A9): `attempt` na tarefa começa em 0. O envio inicial é `attempt = 0` (log `attempt = 1`). Um reenvio é permitido enquanto `attempt < SqsPublishMaxAttempts`. Assim há no máximo `SqsPublishMaxAttempts + 1` envios, como hoje.
- **Reclaim conta como tentativa**: um lease expirado incrementa `attempt`. Isso limita loops em que o processo sempre cai durante o envio de uma mensagem específica (Principle VIII: retry limitado).
- **Desistência**: ao atingir o limite, `ack` com resultado `dead_lettered`, log de nível `error` com `ticket_uuid`, `org_id`, `correlation_id`, `queue_url`, `attempt`, `result`, `attempted_at` e o erro final (sem corpo, sem URN).
- **Persistência (Q2 = não)**: não há dead letter no Redis. O reenvio manual é feito a partir do banco usando `ticket_uuid` e `org_id`, como permite RF5. Isso é suficiente para a Constituição VIII, que aceita "recorded in logs". Trade-off registrado: o reenvio manual reconstrói a mensagem, não a reenvia byte a byte.
- **Lease e tempo de envio**: o lease usa `ProcessingTTL` (120 s, já existente). Invariante: o pior caso de um envio (5 s de timeout + reconexão + 5 s de nova tentativa) deve ser menor que `ProcessingTTL`. Com os valores atuais, o pior caso é ~20 s, bem abaixo de 120 s.

## R10. Recuperação "na inicialização" (FR-006 / L6)

**Status: aprovado em 2026-11-27.** Recuperação apenas por expiração de lease (`ProcessingTTL`), sem identidade de instância. É a opção mais simples, com menos pontos de falha e menos complexidade. A recuperação imediata após reinício não será implementada.

- **Decision**: o reclaim roda uma vez de forma síncrona durante a inicialização (`InitFunction`, antes de os foremen iniciarem) e depois a cada tick (1 s). Na inicialização ele recupera **leases já expirados**. Leases do processo que caiu só expiram após `ProcessingTTL`, então são recuperados nesse prazo, não imediatamente.
- **Rationale**: sem identidade estável de instância, não há como saber quais leases pertencem ao processo que reiniciou sem tirar de outra réplica uma tarefa ainda válida (Edge case "múltiplas instâncias" da spec). Expiração por lease é o único critério seguro entre réplicas.
- **Impacto**: um ticket de um processo que caiu durante o envio pode levar até 120 s para ser reenviado. Isso fica fora do percentil de 95% em 60 s (é a cauda). **Se o time quiser recuperação imediata**, é preciso marcar leases com um identificador de instância estável e único (não basta `InstanceName`, que é o nome de analytics e pode ser compartilhado). Registrado como risco; não bloqueia o plano.

## R11. Promoção e reclaim: background, não por pop

- **Decision**: um loop de manutenção (`InitFunction`, sem novo nome de configuração) executa, a cada 1 s, `promote` para os retries vencidos e `reclaim` para os leases expirados. Lotes limitados por constante nomeada (ex.: `sqsPublishMaintenanceBatch = 100`). Os dois só rodam se `SqsPublishEnabled` e `rt.SQS != nil`.
- **Rationale**: o pop continua O(1) e não precisa conhecer delayed. Latência máxima adicional de retry: ~1 s.
- **Alternatives considered**: promover dentro do pop, com a fila de atraso consultada a cada retirada: acopla pop e atraso, e exige que o pop lide com orgs só com itens futuros (A5).

## R12. Logs: formato, campos e redação

- **Decision**:
  - Um único arquivo `core/tasks/sqs/logs.go` com constantes de campo e uma função por evento. Nenhum outro código monta campos de log de SQS à mão.
  - Campos de tempo como string `2006-01-02T15:04:05.000Z` em UTC (milissegundos explícitos). Durações como inteiros em milissegundos.
  - Campo `event` (valores: `enqueued`, `attempt`, `sent`, `retry_scheduled`, `retry_persist_failed`, `dead_lettered`, `recovered`) e campo `result` (valores: `sent`, `failed`, `rescheduled`, `dead_lettered`). **Adição à lista de FR-015**: `event` é necessário para consulta sem ambiguidade.
  - **Proibido nos logs de SQS**: `contact_urn`, `message_group_id`, corpo bruto da tarefa, `Attributes["MessageGroupId"]`. A redação de `workers.go` para `sqs_publish` remove o campo `task` dos logs de erro, pânico e tarefa longa.
  - `queued_on` = `time.Now()` antes da escrita no Redis (guardado no envelope e base do score); `enqueued_at` = instante logado depois da confirmação do script. A diferença mede a latência de enfileiramento.
- **Rationale**: consulta por `correlation_id` ou `ticket_uuid` exige nomes idênticos em todas as linhas (L7). Redação na fonte evita vazamento por caminhos esquecidos (A7, A8).
- **Alternatives considered**: campo `contact_hash` (hash da URN) para correlação de contato: não pedido e fora do escopo; descartado para manter o mínimo de dados (L8).

## R16. Relatório de entrega lenta ao Sentry (FR-017)

- **Decision**: no handler, logo após o envio bem-sucedido, se `sent_at - created_on > sqsSlowDeliveryThreshold` (constante nomeada de 5 minutos), emitir um `logrus` de nível `error` com a mensagem fixa `sqs ticket delivery exceeded threshold` e `event = "slow_delivery"`. O hook já configurado em `cmd/mailroom/main.go` (níveis panic, fatal e error) encaminha ao Sentry. Nenhum cliente Sentry novo, nenhuma métrica.
- **Rationale**: reaproveita o único mecanismo de relatório de erros existente (Principle IV pede telemetria pelos hooks existentes). Mensagem fixa faz as ocorrências agruparem num único problema; os detalhes ficam nos campos, que o hook anexa como dados extras.
- **Decomposição do tempo**: `created_to_queued_ms` (antes da fila: flow, DB), `queued_to_first_attempt_ms` (espera por worker ou backlog), `first_attempt_to_sent_ms` (retries e recuperação). Para calcular os dois últimos, `meta` ganha `first_attempt_at`, gravado pelo handler no início da primeira tentativa e carregado no envelope quando há `reschedule` ou `reclaim`. Se o processo cair antes de o envelope ser reescrito, `first_attempt_at` fica ausente e esses dois campos são omitidos.
- **Disparo único**: só no caminho de sucesso e só uma vez por mensagem. Tentativas falhas já têm seus próprios eventos; a desistência já é `error`.
- **Limitações registradas**:
  - Só dispara no envio. Mensagem parada sem sair da fila não gera relatório até esgotar tentativas. Detectar isso exigiria varredura contínua, que a spec exclui.
  - `created_on` vem do evento e `sent_at` do relógio do worker; skew entre hosts afeta o valor em segundos, não em horas.
  - Após degradação longa, o fim da indisponibilidade pode gerar muitos relatórios de uma vez. Agrupam por mensagem fixa; o volume não é limitado.
- **Alternatives considered**:
  - Nível `warn` com regra de alerta no Sentry: `warn` não passa pelo hook atual, e regras de alerta são monitoramento novo, fora de escopo.
  - `sentry.CaptureMessage` direto: criaria um segundo caminho de envio e escaparia da redação centralizada em `logs.go`.
  - Limite configurável: a spec pede nenhum novo nome de configuração; uma constante basta e é fácil de promover a configuração depois.

## R13. Metadados de origem e compatibilidade do envelope (Principle VI)

- **Decision**: `PublishTask` ganha um campo opcional `meta` (`omitempty`) com `ticket_uuid`, `created_on`, `origin` (`http` ou `flow`), `correlation_id` e `first_attempt_at` (preenchido pelo handler, ver R16). O corpo SQS (`TicketSQSMessage`) e `Attributes` não mudam. Tarefas antigas sem `meta` continuam válidas: `ticket_uuid` e `created_on` são lidos do corpo (que é o `TicketSQSMessage`), `correlation_id` de `Attributes["CorrelationID"]`, e `origin = "unknown"`.
- **Rationale**: o envelope é interno ao Redis; o contrato externo é o corpo SQS. O campo opcional permite rolling deploy.
- **Assinatura**: `PublishTicketCreated(rt, orgID, msg, origin)`. Os dois chamadores (`web/ticket/open.go` e `core/handlers/ticket_opened.go`) passam `http` e `flow`.

## R14. Testes sem serviço externo real (Principle X)

- **Decision**: SQS é simulado com `httptest.Server` apontando `SqsEndpoint` (já suportado por `runtime.Config`). Redis e Postgres reais via `testsuite`. Logs capturados com um `logrus` test hook.
- **Rationale**: a constituição exige mocks de dependências HTTP externas. `httptest` permite controlar atraso e erro por requisição, o que é necessário para os cenários de paralelismo e de lentidão.

## R15. Pontos fora de escopo, registrados

- Transação do fluxo `ticket_opened` (A10): a publicação acontece antes do commit. Pode gerar mensagem de ticket que a transação reverteu. A spec não pede correção; recomenda-se abrir item separado.
- Latência do caminho de flow (`HandlerQueue`): explicitamente fora de escopo pela spec.
- Reconexão em timeout (R2): decisão futura.
