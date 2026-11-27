# Feature Specification: Publicação confiável, sem atrasos em cascata e rastreável por logs para eventos de criação de ticket

**Feature Branch**: `005-fix-sqs-ticket-publish`

**Created**: 2026-11-27

**Status**: Draft

**Input**: User description: "Corrigir o atraso e a possível perda de eventos de criação de ticket publicados para organizações com múltiplos agentes e melhorar os logs para permitir diagnóstico completo por consulta, sem adicionar métricas."

## Inheritance from Product Spec

- Product Spec: none — no tagged product spec exists; this specification is the originating description of the defect reported in the multi-agent ticket creation event delay and loss investigation
- Pinned version: none
- Architecture doc: none
- Inherited binding decisions: eventos de criação de ticket para organizações com múltiplos agentes devem chegar à fila externa de tickets sem bloqueios globais entre trabalhadores, sem perda em reinícios ou quedas de processo, sem que novas tentativas fiquem atrás de eventos posteriores da mesma organização, mantendo inalterado o contrato da mensagem consumida e permitindo reconstruir todo o percurso exclusivamente por consulta aos logs estruturados (sem URN do contato em texto claro e sem novas métricas)
- Scope of this spec: enfileiramento, envio paralelo, nova tentativa persistida, recuperação de mensagens em processamento não confirmadas, tratamento de esgotamento de tentativas e logs estruturados de diagnóstico para eventos de criação de ticket de organizações multi-agente
- Divergences: recuperação de mensagens em processamento após queda ocorre em até `ProcessingTTL` (não imediatamente na inicialização), aprovado em 2026-11-27 (R10)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Eventos de criação de ticket não se perdem em falhas ou reinícios (Priority: P1)

Quando um ticket é aberto em uma organização configurada para múltiplos agentes — seja por abertura direta de ticket ou durante a execução de um fluxo de conversa — o evento de criação de ticket aceito para publicação sempre chega à fila externa de tickets, mesmo que ocorram falhas transitórias de entrega ou que uma instância do serviço reinicie ou caia durante o envio ou durante o intervalo de espera para nova tentativa. Caso todas as tentativas permitidas se esgotem sem sucesso, o evento não desaparece em silêncio: fica registrado em erro com os identificadores necessários para reenvio manual. O formato da mensagem entregue aos sistemas consumidores permanece exatamente o mesmo.

**Why this priority**: A perda silenciosa de um evento de criação de ticket deixa o cliente aguardando atendimento humano sem que o ticket exista na fila de atendimento multi-agente, gerando falha direta na operação e sem rastro para recuperação.

**Independent Test**: Abrir tickets para uma organização multi-agente simulando falhas temporárias na fila externa, interrupção abrupta do trabalhador durante o envio e reinício do serviço durante o intervalo de espera entre tentativas. Confirmar que nenhuma mensagem é perdida, que mensagens interrompidas são recuperadas e entregues com o contrato inalterado, que novas tentativas respeitam sua posição original na fila da organização, e que o esgotamento de tentativas gera o registro de erro para reenvio manual.

**Acceptance Scenarios**:

1. **Given** uma mensagem retirada da fila para envio cujo processo é interrompido antes da confirmação de entrega, **When** o serviço reinicia ou o prazo máximo de processamento expira, **Then** a mensagem pendente é recuperada e enviada novamente até ser confirmada.
2. **Given** uma mensagem cujo envio falha e precisa aguardar o intervalo configurado para nova tentativa, **When** o serviço reinicia durante esse intervalo de espera, **Then** a mensagem permanece persistida na fila compartilhada e é enviada após o término do intervalo previsto, sem perda.
3. **Given** uma organização com múltiplas mensagens acumuladas na fila após uma mensagem mais antiga que falhou na primeira tentativa, **When** o intervalo de espera da mensagem mais antiga termina, **Then** essa nova tentativa é processada à frente das mensagens que foram enfileiradas depois da mensagem original na mesma organização, em vez de ir para o fim da fila.
4. **Given** uma falha ao persistir o reagendamento de uma nova tentativa na fila compartilhada, **When** o trabalhador trata essa falha, **Then** o erro é registrado explicitamente e a mensagem não é dada como concluída, permanecendo passível de recuperação.
5. **Given** uma mensagem que falha em todas as tentativas até atingir o limite máximo configurado, **When** a última tentativa falha, **Then** o sistema encerra novas tentativas automáticas e registra um erro contendo `ticket_uuid`, `org_id`, `correlation_id` e `queue_url` para permitir reenvio manual.
6. **Given** qualquer evento de criação de ticket entregue à fila externa (na primeira tentativa, em nova tentativa ou após recuperação), **When** o consumidor recebe a mensagem, **Then** o corpo da mensagem e seus atributos (`MessageGroupId`, `CorrelationID`, `ContentType`) estão idênticos ao contrato atual.

---

### User Story 2 - Lentidão ou falha pontual não trava os demais envios (Priority: P2)

Durante momentos de lentidão de rede, esgotamento de tempo limite em chamadas individuais ou necessidade de reconexão com a fila externa, os trabalhadores configurados continuam publicando eventos de criação de ticket em paralelo. Um envio lento ou uma reconexão não serializa todos os trabalhadores em fila única, evitando que o acúmulo cresça por horas e garantindo que os tickets continuem chegando aos agentes em tempo hábil.

**Why this priority**: A serialização global de todos os trabalhadores em torno de uma única chamada lenta derruba a vazão para uma fração mínima e provoca atrasos de horas na chegada dos tickets ao atendimento humano, mesmo quando há capacidade ociosa.

**Independent Test**: Configurar múltiplos trabalhadores de publicação e introduzir atraso ou falha de conexão em um envio específico enquanto outros tickets são abertos. Confirmar que os demais trabalhadores concluem os envios saudáveis em paralelo, sem aguardar o término da chamada lenta além do limite de concorrência configurado.

**Acceptance Scenarios**:

1. **Given** múltiplos trabalhadores configurados e diversas mensagens prontas para publicação em organizações distintas, **When** os envios são disparados simultaneamente, **Then** as entregas à fila externa ocorrem em paralelo entre os trabalhadores, sem bloqueio global que serialize as chamadas de rede.
2. **Given** um trabalhador enfrentando lentidão até o tempo limite de envio para uma mensagem, **When** outros trabalhadores processam mensagens saudáveis no mesmo período, **Then** essas outras mensagens são entregues normalmente sem aguardar o encerramento do envio lento.
3. **Given** uma falha de conexão detectada por um trabalhador, **When** a conexão é restabelecida, **Then** a recuperação da conexão ocorre de forma coordenada sem bloquear desnecessariamente envios saudáveis nos demais trabalhadores.
4. **Given** múltiplas mensagens enfileiradas em sequência para uma mesma organização sem falhas de envio, **When** são publicadas, **Then** elas são enviadas respeitando a ordem de chegada dentro daquela organização.

---

### User Story 3 - Diagnóstico de ponta a ponta por consulta aos logs (Priority: P3)

Quando a equipe de operação ou engenharia investiga o atraso ou a falha de um ticket específico, basta consultar os logs filtrando por `ticket_uuid` ou `correlation_id` para enxergar toda a linha do tempo do evento: quando o ticket foi criado, por qual caminho entrou (`http` ou `flow`), quando foi enfileirado, cada tentativa realizada com horário e desfecho, quando foi efetivamente enviado (com o tempo total desde a criação e o tempo gasto na fila em milissegundos) ou quando desistiu após o limite de tentativas, além de eventuais recuperações após queda — tudo sem expor dados sensíveis do contato e sem depender de painéis ou métricas novas.

**Why this priority**: Hoje não é possível ligar as tentativas do trabalhador ao ticket aberto nem saber quanto tempo foi gasto antes ou dentro da fila de publicação, tornando impossível diagnosticar incidentes raros de atraso.

**Independent Test**: Abrir um ticket pela via direta (`http`) e outro por fluxo (`flow`), provocar uma falha seguida de sucesso em um deles, esgotamento de tentativas em um terceiro e recuperação de pendente na inicialização em um quarto. Consultar os logs apenas por `ticket_uuid` e por `correlation_id` e verificar que todas as etapas e durações aparecem com nomes de campos idênticos e sem a URN do contato em texto claro.

**Acceptance Scenarios**:

1. **Given** a abertura de um ticket em organização multi-agente tanto pela via direta (`http`) quanto por evento de fluxo (`flow`), **When** o evento é enfileirado para publicação, **Then** é registrado um log com `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `origin` (`http` ou `flow`), `created_on`, `queued_on` e `enqueued_at` (em UTC com precisão de milissegundos).
2. **Given** uma mensagem em processamento por um trabalhador, **When** cada tentativa de envio é executada e resulta em sucesso, **Then** os logs registram a tentativa (`correlation_id`, `ticket_uuid`, `org_id`, `attempt`, `queue_url`, `result`) e o envio concluído com `sent_at` (UTC com milissegundos), `created_to_sent_ms` e `queued_to_sent_ms` calculado desde o `queued_on` original.
3. **Given** uma tentativa de envio que falha com novas tentativas ainda disponíveis, **When** a falha é tratada, **Then** o log registra `correlation_id`, `ticket_uuid`, `org_id`, `attempt`, `queue_url`, `result`, a descrição do erro, `attempted_at` (UTC com milissegundos) e `next_attempt_at` (UTC com milissegundos).
4. **Given** mensagens pendentes não confirmadas recuperadas na inicialização do serviço (ou após expiração do prazo de processamento), **When** a recuperação ocorre, **Then** o log registra a quantidade recuperada (`recovered_count`) e os identificadores de cada mensagem recuperada (`ticket_uuid`, `correlation_id`, `org_id`, `queue_url`).
5. **Given** uma mensagem enviada com sucesso mais de 5 minutos depois de `created_on`, **When** o envio é concluído, **Then** é emitido um único registro de erro `slow_delivery` encaminhado ao Sentry, com `created_to_queued_ms`, `queued_to_sent_ms`, `created_to_sent_ms`, `attempt`, `origin` e os identificadores, sem URN; e uma mensagem enviada em até 5 minutos não gera esse registro.
6. **Given** todos os logs emitidos pelo fluxo de publicação de tickets (incluindo logs de falha, tarefa lenta ou interrupção inesperada), **When** inspecionados, **Then** todos usam exatamente os mesmos nomes de campo padronizados e nenhum registro contém a URN do contato em texto claro.

---

### Edge Cases

- **Queda do processo após o envio externo, mas antes da confirmação local**: a mensagem permanece como pendente não confirmada, é recuperada e reenviada; a mensagem reenviada mantém exatamente o mesmo corpo, cabendo ao consumidor deduplicar por `ticket_uuid`.
- **Reinício após queda com mensagem em processamento**: a mensagem só é recuperada quando seu prazo de processamento (`ProcessingTTL`) expira, ou seja, em até `ProcessingTTL` após a queda, e não imediatamente na inicialização. Essa é uma decisão aprovada para manter a recuperação simples e segura entre réplicas.
- **Reinício do serviço sem mensagens pendentes a recuperar**: a verificação de recuperação na inicialização conclui sem erro e sem afetar mensagens recém-enfileiradas.
- **Múltiplas instâncias do serviço em execução simultânea**: uma mensagem em processamento ativo dentro do prazo válido por uma instância não é tomada prematuramente como expirada por outra instância que reinicia no mesmo momento; apenas mensagens cujo prazo de confirmação expirou (ou órfãs não confirmadas) são recuperadas.
- **Nova tentativa concorrendo com novas chegadas na mesma organização**: enquanto a mensagem que falhou aguarda o intervalo configurado até `next_attempt_at`, outras mensagens já elegíveis podem ser enviadas (exceção prevista de ordenação em caso de retry); assim que `next_attempt_at` é atingido, a mensagem em retry tem precedência sobre todas as mensagens enfileiradas depois da mensagem original na mesma organização.
- **Preservação do horário original de entrada na fila (`queued_on`)**: ao reagendar uma nova tentativa ou recuperar uma mensagem não confirmada, o `queued_on` original é preservado para que o tempo de fila registrado no sucesso reflita todo o período desde o enfileiramento inicial.
- **Mensagens enfileiradas antes da atualização (atualização gradual)**: mensagens que já estavam na fila no formato anterior (sem metadados novos como `origin` ou `enqueued_at`) continuam sendo processadas e entregues normalmente, preenchendo nos logs todos os campos disponíveis sem falhar.
- **Organizações sem `is_multi_agents` ativo ou com publicação desativada**: nenhum evento é enfileirado na fila externa de tickets e nenhum comportamento existente é alterado.
- **Atraso anterior ao enfileiramento**: se o ticket levou mais de 5 minutos para chegar à fila (por exemplo, no caminho de fluxo), o envio ainda gera `slow_delivery`, e `created_to_queued_ms` mostra que o tempo foi gasto antes da publicação. Isso não é corrigido aqui (fora de escopo), apenas identificado.
- **Recuperação após queda**: uma mensagem recuperada de um processo interrompido só gera `slow_delivery` se `sent_at - created_on` exceder o limite. Quando gera, `attempt > 1` indica que houve nova tentativa ou recuperação.
- **Volume de relatórios após degradação longa**: ao fim de uma indisponibilidade do SQS, várias mensagens podem ser enviadas de uma vez acima do limite e gerar um relatório cada. Como a mensagem é sempre a mesma, elas agrupam em um problema do Sentry com contagem de ocorrências; o volume não é limitado por esta especificação.
- **Exatamente 5 minutos**: o relatório só é emitido quando o tempo é estritamente maior que 5 minutos.
- **Exposição indireta de URN em campos compostos ou erros**: como o atributo `MessageGroupId` e o corpo bruto da tarefa contêm a URN do contato, nenhum desses valores brutos pode ser gravado em texto claro nos logs de enfileiramento, tentativa, erro, lentidão ou pânico.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: O sistema MUST realizar envios para a fila externa de tickets em paralelo entre os trabalhadores configurados, sem manter bloqueio global durante chamadas de rede que serialize envios de outros trabalhadores.
- **FR-002**: O sistema MUST recuperar falhas de conexão com a fila externa sem bloquear desnecessariamente envios saudáveis em andamento ou subsequentes.
- **FR-003**: O sistema MUST persistir o agendamento de novas tentativas na fila compartilhada — e MUST NOT manter a espera de retry apenas em memória do processo — respeitando o limite de reenvios já existente em `SqsPublishMaxAttempts` (o envio inicial mais até esse número de reenvios, como hoje) e o intervalo fixo `SqsPublishDelayIntervalMs` já existente na configuração do serviço.
- **FR-004**: O sistema MUST tornar uma mensagem em retry elegível para novo envio somente após o término do intervalo de espera configurado; caso a gravação do reagendamento na fila compartilhada falhe, o sistema MUST registrar o erro e manter a mensagem não confirmada para recuperação posterior.
- **FR-005**: Dentro de uma mesma organização, quando uma mensagem em nova tentativa atingir seu horário previsto de reenvio, o sistema MUST posicioná-la à frente de mensagens que foram enfileiradas depois da mensagem original nessa organização; para mensagens que não sofrerem retry, o sistema MUST preservar a ordem de chegada dentro de cada organização.
- **FR-006**: O sistema MUST registrar toda mensagem retirada da fila como em processamento até sua confirmação final (envio concluído, reagendamento persistido ou esgotamento de tentativas) e MUST recuperar e reenviar mensagens não confirmadas tanto na inicialização do serviço quanto quando o prazo de processamento expirar.
- **FR-007**: Ao atingir o limite máximo configurado de tentativas sem sucesso, o sistema MUST encerrar novas tentativas automáticas e registrar um evento em nível de erro com dados suficientes para reenvio manual: `ticket_uuid`, `org_id`, `correlation_id` e `queue_url`.
- **FR-008**: O sistema MUST manter inalterados o corpo da mensagem entregue à fila externa de tickets (`ticket_uuid`, `contact_urn`, `project_uuid`, `channel_uuid`, `created_on`) e os atributos enviados (`MessageGroupId`, `CorrelationID`, `ContentType`), bem como tolerar tarefas enfileiradas antes da atualização durante implantações graduais.
- **FR-009**: Ao enfileirar um evento de criação de ticket para publicação (e em caso de falha ao enfileirar), o sistema MUST registrar nos logs `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `origin` (`http` ou `flow`), `created_on`, `queued_on` e `enqueued_at` (horário em UTC com precisão de milissegundos).
- **FR-010**: Em cada tentativa de envio à fila externa, o sistema MUST registrar nos logs `correlation_id`, `ticket_uuid`, `org_id`, `attempt` (numeração sequencial iniciando em `1` no envio inicial; o valor máximo é `SqsPublishMaxAttempts + 1`), `queue_url` e `result`.
- **FR-011**: Em caso de envio bem-sucedido, o sistema MUST registrar nos logs `correlation_id`, `ticket_uuid`, `org_id`, `attempt`, `queue_url`, `sent_at` (horário em UTC com precisão de milissegundos), `created_to_sent_ms` (tempo desde `created_on` até `sent_at`, em milissegundos) e `queued_to_sent_ms` (tempo desde o `queued_on` original até `sent_at`, em milissegundos).
- **FR-012**: Em caso de falha de envio com novas tentativas restantes, o sistema MUST registrar nos logs `correlation_id`, `ticket_uuid`, `org_id`, `attempt`, `queue_url`, `result`, o erro ocorrido, `attempted_at` (horário da tentativa em UTC com precisão de milissegundos) e `next_attempt_at` (horário previsto da próxima tentativa em UTC com precisão de milissegundos).
- **FR-013**: Ao desistir após atingir o limite máximo de tentativas, o registro em nível de erro definido em FR-007 MUST incluir `correlation_id`, `ticket_uuid`, `org_id`, `attempt`, `queue_url`, `result`, `attempted_at` (UTC com precisão de milissegundos) e o erro final.
- **FR-014**: Ao recuperar mensagens pendentes não confirmadas na inicialização do serviço ou após expiração de processamento, o sistema MUST registrar nos logs a quantidade recuperada (`recovered_count`) e os identificadores das mensagens recuperadas (`ticket_uuid`, `correlation_id`, `org_id`, `queue_url`).
- **FR-015**: O sistema MUST utilizar exatamente os mesmos nomes de campo padronizados em todos os logs do fluxo de publicação na fila externa (`event`, `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `origin`, `attempt`, `result`, `created_on`, `queued_on`, `enqueued_at`, `attempted_at`, `sent_at`, `next_attempt_at`, `created_to_sent_ms`, `queued_to_sent_ms`, `recovered_count`), permitindo consulta direta por `correlation_id` ou `ticket_uuid`.
- **FR-016**: O sistema MUST NOT registrar a URN do contato em texto claro em nenhum log (inclusive em campos derivados ou despejos brutos de tarefa em logs de falha, lentidão ou exceção) e MUST NOT adicionar métricas, contadores, painéis, regras de alerta ou monitoramento contínuo. O relatório pontual de entrega lenta definido em FR-017 não é uma métrica nem um alerta novo: é um registro de erro individual enviado pelo mecanismo de relatório de erros que o serviço já usa.
- **FR-017**: Quando uma mensagem for enviada com sucesso e o tempo entre `created_on` e `sent_at` for **maior que 5 minutos**, o sistema MUST emitir exatamente **um** registro em nível de erro (que o mecanismo existente de relatório de erros encaminha ao Sentry) com `event = "slow_delivery"` e os campos: `ticket_uuid`, `correlation_id`, `org_id`, `queue_url`, `origin`, `attempt`, `created_on`, `queued_on`, `sent_at`, `created_to_queued_ms`, `queued_to_sent_ms`, `created_to_sent_ms`, `slow_threshold_ms` e, quando conhecidos, `first_attempt_at`, `queued_to_first_attempt_ms` e `first_attempt_to_sent_ms`. Esses campos permitem separar o tempo gasto antes do enfileiramento, na espera por um trabalhador e nas novas tentativas. O registro MUST usar sempre o mesmo texto de mensagem, para que ocorrências agrupem no mesmo problema, e MUST NOT conter a URN do contato nem o corpo da mensagem. O limite de 5 minutos é uma constante interna, sem novo nome de configuração. Envios de até 5 minutos MUST NOT gerar esse registro.

### Key Entities

- **Evento de criação de ticket para publicação**: Mensagem gerada na abertura de um ticket de organização multi-agente contendo identificadores do ticket, projeto, canal, contato e instante de criação (`created_on`), acompanhada de atributos de correlação e agrupamento exigidos pelo consumidor externo.
- **Tarefa de publicação persistida**: Registro armazenado na fila compartilhada por organização que mantém o conteúdo a publicar, a fila de destino (`queue_url`), a origem da abertura (`http` ou `flow`), o instante original de entrada na fila (`queued_on`), o instante de enfileiramento (`enqueued_at`), o contador de tentativas (`attempt`), a ordem original de chegada na organização e o horário mínimo elegível para próxima tentativa (`next_attempt_at`).
- **Registro de envio em processamento**: Marcação temporária e persistida na fila compartilhada que identifica uma tarefa retirada por um trabalhador e ainda não confirmada, associada a um prazo de expiração após o qual a tarefa se torna elegível para recuperação.
- **Trilha de logs de publicação**: Conjunto de registros estruturados emitidos nas etapas de enfileiramento, tentativa, sucesso, falha com reagendamento, recuperação de pendentes e desistência final, correlacionados pelos campos fixos `ticket_uuid` e `correlation_id`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 0% das mensagens aceitas para publicação são perdidas quando o serviço reinicia ou sofre queda durante o envio ou durante a espera de nova tentativa; 100% das mensagens interrompidas são recuperadas e enviadas.
- **SC-002**: Em operação normal da fila externa, pelo menos 95% das mensagens enfileiradas são entregues em até 60 segundos após o enfileiramento.
- **SC-003**: Quando um envio sofre lentidão até o tempo limite da chamada, os demais trabalhadores configurados continuam entregando outras mensagens em paralelo, sem redução da concorrência disponível além do trabalhador ocupado naquele envio.
- **SC-004**: Dado qualquer `ticket_uuid` ou `correlation_id`, a equipe reconstrói em 100% dos casos — apenas por consulta aos logs — o caminho completo (enfileiramento com origem, tentativas, envio ou desistência e recuperações) e o tempo gasto em cada etapa em milissegundos.
- **SC-005**: 100% das mensagens dentro de uma mesma organização que não entram em retry são retiradas para envio na exata ordem de chegada (ordem de despacho), e nenhuma mensagem em retry é posicionada atrás de mensagens enfileiradas depois dela na mesma organização após cumprir seu intervalo de espera. A ordem de chegada no consumidor não é garantida quando há mais de um trabalhador de envio ativo.
- **SC-006**: 0 registros de log emitidos pelo fluxo expõem a URN do contato em texto claro, e 100% das mensagens publicadas preservam o contrato atual de corpo e atributos para os consumidores.
- **SC-007**: 100% das mensagens enviadas mais de 5 minutos depois de `created_on` geram exatamente um relatório de entrega lenta no Sentry com a decomposição do tempo (antes do enfileiramento, na fila, nas novas tentativas), e 0% das enviadas em até 5 minutos geram esse relatório.

## Assumptions

- **Infraestrutura de fila mantida**: O armazenamento compartilhado atual continua sendo o backend da fila de publicação por organização, sem introdução de novo serviço de mensageria intermediário.
- **Semântica de entrega e deduplicação**: A entrega segue garantia *at-least-once* (pelo menos uma vez). Reenvios decorrentes de falhas de confirmação ou recuperação após queda produzem mensagens com exatamente o mesmo corpo, e o serviço consumidor já realiza deduplicação por `ticket_uuid`.
- **Escopo de ordenação**: A ordem é garantida como ordem de despacho apenas dentro de cada organização individualmente. Com múltiplos trabalhadores, duas mensagens do mesmo org podem ser enviadas em paralelo, então a ordem de chegada no consumidor não é garantida. Organizações distintas são processadas de forma independente e concorrente, e desvios pontuais de ordem dentro da mesma organização são aceitos enquanto uma mensagem com falha aguarda seu intervalo de nova tentativa.
- **Configurações preservadas**: Os parâmetros de configuração já existentes para habilitação, quantidade de trabalhadores, número máximo de tentativas, intervalo entre tentativas e prazo de processamento continuam válidos e suficientes, sem criação de novos nomes de configuração a menos que estritamente indispensável.
- **Carga de pico declarada**: O caminho deve suportar picos de abertura simultânea de dezenas de tickets por segundo somados entre organizações multi-agente e acúmulos transitórios de milhares de mensagens pendentes na fila compartilhada durante degradações da fila externa, mantendo instâncias totalmente sem estado local obrigatório para escalar horizontalmente.
- **Fora de escopo desta especificação**:
  - Investigação ou redução de latência anterior ao enfileiramento no caminho de fluxo (processamento de eventos de fluxo e sua fila própria); nesta entrega registra-se apenas `origin = flow` ou `origin = http` nos logs para distinguir onde o tempo foi gasto.
  - Criação ou alteração de métricas, contadores, painéis, regras de alerta ou monitoramento contínuo. O relatório pontual de entrega lenta (FR-017) é a única exceção, e é um registro de erro individual, não uma métrica.
  - Detecção de mensagens que ainda estão paradas e não foram enviadas. FR-017 só dispara no momento do envio; mensagens que nunca saem da fila são cobertas apenas pelo registro de desistência (FR-007) após esgotar as tentativas.
  - Alterações no esquema do corpo ou atributos da mensagem publicada na fila externa.
  - Implementação de deduplicação no consumidor da fila externa.
