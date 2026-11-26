# Feature Specification: Catálogo do Live Desk chega ao cliente

**Feature Branch**: `004-fix-livedesk-catalog`

**Created**: 2026-11-17

**Status**: Draft

**Input**: User description:

## Inheritance from Product Spec

- Product Spec: none — no tagged product spec exists; this specification is the originating description of the defect reported in the Live Desk catalog investigation
- Pinned version: none
- Architecture doc: none
- Inherited binding decisions: a catalog sent by the agent during human attendance must reach the customer the same way a catalog sent from the Manager already does: one catalog message, with the agent's wording and the products together
- Scope of this spec: catalog replies sent during human attendance on WhatsApp Cloud, from the moment the attendance event is received until the customer-facing catalog message is ready to deliver
- Divergences: none

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Cliente recebe o catálogo com o texto do agente (Priority: P1)

Um contato está em atendimento humano no Live Desk, num canal WhatsApp Cloud com catálogo configurado. O agente envia um catálogo pelo Desk Copilot, com um texto e um ou mais produtos. O cliente recebe uma única mensagem de catálogo: o texto do agente acompanha os produtos. Não chega uma mensagem de texto solta antes, e o canal não recusa o catálogo por falta de texto.

**Why this priority**: É o defeito reportado. Hoje o cliente vê o texto e não vê os produtos, porque o catálogo segue sem o texto que o canal exige.

**Independent Test**: Com o contato em atendimento humano, enviar pelo Desk Copilot um catálogo com texto e mais de um produto. O cliente recebe os produtos junto com esse texto, numa única mensagem aceita pelo canal.

**Acceptance Scenarios**:

1. **Given** um contato em atendimento humano e um catálogo com texto e mais de um produto, **When** o agente envia esse catálogo pelo Desk Copilot, **Then** o cliente recebe uma única mensagem de catálogo com esse texto e esses produtos, e o canal aceita a mensagem.
2. **Given** o mesmo envio, **When** a mensagem é entregue, **Then** o cliente não recebe uma mensagem de texto separada com o mesmo texto antes do catálogo.
3. **Given** um catálogo com um único produto e texto, **When** o agente envia, **Then** o cliente também recebe texto e produto juntos, numa única mensagem aceita.
4. **Given** um catálogo cujo texto traz um endereço de mídia que o atendimento já reescreve em respostas comuns, **When** o agente envia o catálogo, **Then** o texto que acompanha os produtos usa esse mesmo endereço reescrito.
5. **Given** uma resposta só de texto ou só de mídia no mesmo atendimento, **When** o agente envia, **Then** o cliente continua recebendo essa resposta como hoje.

---

### User Story 2 - Título, rodapé e resposta citando outra mensagem continuam no catálogo (Priority: P2)

O agente pode mandar o catálogo com um título, um rodapé, um rótulo de botão, ou como resposta a uma mensagem anterior do contato. O que foi informado aparece na mensagem de catálogo. O que não foi informado não impede a entrega.

**Why this priority**: O envio pelo Manager já mostra título e texto juntos. Sem o título, o catálogo do Live Desk chega diferente do que o agente escreveu, mesmo depois de o texto principal passar a acompanhar os produtos.

**Independent Test**: Enviar um catálogo com título e outro sem título. No primeiro, o cliente vê o título na mesma mensagem dos produtos. No segundo, o catálogo é aceito mesmo sem título.

**Acceptance Scenarios**:

1. **Given** um catálogo com título e texto, **When** o agente envia, **Then** o cliente vê o título e o texto na mesma mensagem de catálogo.
2. **Given** um catálogo sem título, **When** o agente envia com texto e produtos, **Then** o canal aceita a mensagem e o cliente recebe texto e produtos.
3. **Given** um catálogo com rodapé ou rótulo de botão, **When** o agente envia, **Then** esses textos seguem na mensagem de catálogo.
4. **Given** um catálogo enviado como resposta a uma mensagem anterior do contato, **When** o agente envia, **Then** a relação com essa mensagem anterior é mantida.
5. **Given** um catálogo enviado a partir de uma mensagem já criada no atendimento, **When** a resposta é registrada, **Then** ela continua associada a essa mensagem de origem.

---

### User Story 3 - Texto longo ou texto vazio não gera catálogo recusado (Priority: P3)

O texto do Copilot pode passar do limite que o canal aceita no corpo do catálogo, ou pode chegar em branco. O cliente não pode ficar sem os produtos por causa disso, e o atendimento não pode ser informado de que a mensagem foi tratada se nada foi entregue.

**Why this priority**: O caso reportado tem texto curto e produtos. Sem estas regras, o mesmo caminho volta a falhar para textos longos, texto em branco, ou um catálogo da loja inteira sem lista de produtos.

**Independent Test**: Enviar um catálogo com texto acima de 1024 caracteres e outro sem texto utilizável. O primeiro chega como um único catálogo aceito, com o começo do texto. O segundo não é confirmado como entregue se não houver um corpo válido.

**Acceptance Scenarios**:

1. **Given** um texto de agente com mais de 1024 caracteres e uma lista de produtos, **When** o agente envia o catálogo, **Then** o cliente recebe uma única mensagem de catálogo aceita, o corpo traz só os primeiros 1024 caracteres, e o restante não segue como mensagem de texto separada.
2. **Given** um texto só com espaços e um título, **When** o agente envia o catálogo com produtos, **Then** o título é usado como corpo, o cliente recebe o catálogo, e o canal não recusa a mensagem por corpo vazio.
3. **Given** texto e título ambos vazios, **When** o agente envia um catálogo, **Then** a resposta falha de forma visível e não é confirmada como entregue.
4. **Given** um envio do catálogo inteiro da loja, sem produtos escolhidos e sem texto de mensagem avulsa, **When** o agente envia, **Then** uma mensagem de catálogo é criada para o cliente e o atendimento não recebe confirmação de entrega se nenhuma mensagem tiver sido criada.

---

### Edge Cases

- Texto do agente com espaços no início ou no fim: o corpo do catálogo usa o texto sem esses espaços.
- Catálogo com produtos e sem título: a mensagem é aceita; a ausência de título não é enviada como título vazio.
- Catálogo com título e sem texto utilizável: o título preenche o corpo obrigatório e também permanece como título.
- Texto acima de 1024 caracteres: o corte fica no limite do corpo; não há segunda mensagem de texto.
- Catálogo da loja inteira, sem itens escolhidos: a mensagem não é descartada só porque o texto avulso ficou vazio.
- Rodapé, rótulo de botão e resposta a uma mensagem anterior, quando vierem no envio, permanecem na mensagem.
- Respostas de texto e de mídia do mesmo atendimento não mudam.
- Catálogos disparados por fluxo, que hoje já chegam ao cliente, não mudam.
- O atendimento marca o catálogo como carrossel. Nesta correção o cliente continua recebendo uma lista de produtos, que é a forma já aceita quando o Manager envia o mesmo tipo de catálogo. Transformar isso em carrossel fica fora deste escopo.
- O Desk Copilot não envia rótulo de botão. A lista de produtos não exige esse rótulo. Nenhum rótulo é inventado.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Quando o agente envia um catálogo com texto durante o atendimento humano, o cliente MUST receber esse texto e os produtos numa única mensagem de catálogo aceita pelo canal.
- **FR-002**: Essa resposta MUST NOT ser entregue também como mensagem de texto separada.
- **FR-003**: O corpo visto pelo cliente MUST ser o texto do agente, depois da mesma reescrita de endereços de mídia já aplicada às respostas do atendimento, sem espaços no início ou no fim.
- **FR-004**: Quando o agente informa um título, o cliente MUST ver esse título na mesma mensagem de catálogo.
- **FR-005**: Quando o agente não informa título, o catálogo com texto e produtos MUST ainda ser aceito, sem título vazio.
- **FR-006**: O corpo do catálogo MUST NOT ficar vazio. Se o texto do agente estiver vazio ou só com espaços e houver título, o título MUST ser usado como corpo. Se texto e título estiverem vazios, a resposta MUST falhar de forma visível e MUST NOT ser confirmada como entregue.
- **FR-007**: Se o texto do agente tiver mais de 1024 caracteres, o corpo MUST conter apenas os primeiros 1024 caracteres, o catálogo MUST ser entregue como uma única mensagem aceita, e o excedente MUST NOT seguir como mensagem de texto separada.
- **FR-008**: Um envio do catálogo inteiro da loja, sem produtos escolhidos, MUST gerar uma mensagem de catálogo para o cliente. A resposta MUST NOT ser confirmada como entregue quando nenhuma mensagem tiver sido criada.
- **FR-009**: Rodapé, rótulo de botão, produtos escolhidos, vínculo com a mensagem de origem do atendimento e resposta a uma mensagem anterior MUST ser preservados quando tiverem sido informados.
- **FR-010**: Respostas só de texto e respostas com mídia no mesmo atendimento MUST manter o comportamento atual.
- **FR-011**: Catálogos disparados por fluxo MUST manter o comportamento atual, em que o cliente já recebe uma única mensagem de catálogo com texto e título.
- **FR-012**: O registro da resposta de catálogo MUST evitar que o texto do agente seja entregue de novo como mensagem de texto. O texto que o cliente vê fica no corpo do catálogo.

### Key Entities

- **Resposta de catálogo do atendimento**: Texto do agente, título opcional, rodapé opcional, rótulo de botão opcional, produtos escolhidos ou indicação de catálogo inteiro da loja, e o vínculo com a mensagem de origem e com uma mensagem anterior quando existirem.
- **Mensagem de catálogo do cliente**: Uma única mensagem pronta para entrega, com corpo obrigatório, título só quando o agente informou um, produtos ou catálogo inteiro, e sem texto avulso que gere uma segunda mensagem.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: No cenário reportado (atendimento humano, texto do agente e mais de um produto), o cliente recebe os produtos junto com esse texto e o canal aceita a mensagem.
- **SC-002**: Cada resposta de catálogo produz exatamente uma mensagem para o cliente e zero mensagens de texto separadas com o mesmo conteúdo.
- **SC-003**: Quando há título, ele aparece nessa mesma mensagem. Quando não há, a mensagem com texto e produtos ainda é aceita.
- **SC-004**: 100% das respostas de catálogo com texto utilizável de até 1024 caracteres são aceitas sem recusa por corpo ausente.
- **SC-005**: Texto acima de 1024 caracteres ainda produz um catálogo aceito, com o corpo limitado a 1024 caracteres e sem mensagem de texto extra.
- **SC-006**: Respostas que não conseguem formar um corpo de catálogo não vazio são recusadas no atendimento e não são confirmadas como entregues.

## Assumptions

- O defeito está só na resposta de catálogo do atendimento humano. O envio pelo Desk Copilot até o Mailroom, e o envio de catálogo pelo Manager, já estão corretos e ficam como estão.
- O cliente deve receber uma lista de produtos, como no envio bem-sucedido do Manager. Não faz parte desta correção passar a entregar carrossel.
- O canal exige um corpo de catálogo não vazio e aceita no máximo 1024 caracteres nesse corpo. O corte usa os primeiros 1024 caracteres, sem segunda mensagem.
- Título e rodapé continuam sendo os textos curtos enviados pelo agente. Esta correção não cria uma política nova de tamanho para eles.
- Se não houver texto utilizável, o título existente serve de corpo. Se os dois faltarem, a resposta falha em vez de inventar uma frase pronta.
- O rótulo de botão é opcional na lista de produtos. Como o Desk Copilot não envia esse rótulo, nenhum texto de botão é inventado.
- Esvaziar o texto avulso da mensagem de catálogo é o mesmo resultado já usado quando um fluxo envia catálogo. A sala do agente já guarda o texto original antes desta resposta, então o agente continua vendo o que escreveu.
- O volume de mensagens não muda. O caminho de resposta de ticket já existe; só muda o conteúdo da mensagem de catálogo.
- A correção já commitada coloca o texto do agente no corpo do catálogo, o título no campo de título lido na entrega, e esvazia o texto avulso. Ela ainda não cobre o limite de 1024 caracteres, o corpo vazio, o catálogo inteiro sem produtos escolhidos, nem a conferência automatizada desse resultado.
