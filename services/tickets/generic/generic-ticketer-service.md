# Documentação externa para integração como Ticketer

## Visão geral

O **Ticketer** é um sistema externo de transbordo de atendimento.

Quando uma conversa precisa ser transferida para atendimento humano, a plataforma cria um ticket em um sistema externo, como helpdesk, CRM, contact center ou ferramenta de chat. A partir desse momento, a plataforma e o Ticketer trocam eventos para manter o atendimento sincronizado.

Esta documentação descreve o contrato HTTP que o parceiro precisa implementar para ser compatível como Ticketer, sem depender de detalhes internos da plataforma.

> **Nota:** este documento descreve o **contrato proposto** (`v1`). A implementação atual do Mailroom expõe os webhooks de retorno em `/mr/tickets/types/{ticketer_type}/event_callback/{ticketer_uuid}` — durante o período de transição, a URL final pode variar por integração e será informada no provisionamento. Itens marcados com **(roadmap)** ainda não estão suportados pela plataforma.

---

## 0. Provisionamento de credenciais

Antes do primeiro tráfego, plataforma e parceiro trocam credenciais:

| Sentido | Credencial / config | Quem define | Onde fica |
|---------|---------------------|-------------|-----------|
| Plataforma → Ticketer | `api_token` | Parceiro | Campo `config.api_token` do ticketer |
| Ticketer → Plataforma | `webhook_secret` | Plataforma | Campo `config.webhook_secret` do ticketer (compartilhado com o parceiro no provisionamento) |
| Ticketer → Plataforma | `skip_webhook_hmac` | Plataforma | Campo `config.skip_webhook_hmac` do ticketer (ver [Seção 1.2](#12-ticketer--plataforma-hmac)) |
| URL base do Ticketer | — | Parceiro | Campo `config.base_url` |
| Plataforma → Ticketer | `open_template` | Plataforma / Parceiro | Campo `config.open_template` opcional (ver [Seção 2.1.1](#211-payload-customizado-com-open_template)) |
| Plataforma → Ticketer | `open_response_template` | Plataforma / Parceiro | Campo `config.open_response_template` opcional (ver [Seção 2.1.2](#212-resposta-customizada-com-open_response_template)) |
| Plataforma → Ticketer | `forward_template` | Plataforma / Parceiro | Campo `config.forward_template` opcional (ver [Seção 2.2.1](#221-payload-customizado-com-forward_template)) |
| Plataforma → Ticketer | `forward_response_template` | Plataforma / Parceiro | Campo `config.forward_response_template` opcional (ver [Seção 2.2.2](#222-resposta-customizada-com-forward_response_template)) |
| Plataforma → Ticketer | `close_template` | Plataforma / Parceiro | Campo `config.close_template` opcional (ver [Seção 2.3.1](#231-payload-customizado-com-close_template)) |
| Plataforma → Ticketer | `close_response_template` | Plataforma / Parceiro | Campo `config.close_response_template` opcional (ver [Seção 2.3.2](#232-resposta-customizada-com-close_response_template)) |
| Plataforma → Ticketer | `history_mode` | Plataforma / Parceiro | Campo `config.history_mode` opcional: `batch` (default) ou `one_by_one` (ver [Seção 2.5](#25-enviar-histórico-da-conversa-opcional)) |
| Plataforma → Ticketer | `history_batch_size` | Plataforma / Parceiro | Campo `config.history_batch_size` opcional; default `50` no modo `batch` |
| Plataforma → Ticketer | `route_history` | Plataforma / Parceiro | Override da rota de histórico em batch; default `/v1/tickets/{external_id}/history` |
| Plataforma → Ticketer | `route_history_message` | Plataforma / Parceiro | Override da rota no modo `one_by_one`; default igual a `route_forward` (`/v1/tickets/{external_id}/messages`) |
| Plataforma → Ticketer | `history_template` | Plataforma / Parceiro | Campo `config.history_template` opcional (ver [Seção 2.5.1](#251-payload-customizado-com-history_template)) |
| Plataforma → Ticketer | `history_response_template` | Plataforma / Parceiro | Campo `config.history_response_template` opcional (ver [Seção 2.5.2](#252-resposta-customizada-com-history_response_template)) |
| Ticketer → Plataforma | `messages_template` | Plataforma / Parceiro | Campo `config.messages_template` opcional (ver [Seção 3.1.1](#311-payload-customizado-com-messages_template)) |
| Ticketer → Plataforma | `messages_response_template` | Plataforma / Parceiro | Campo `config.messages_response_template` opcional (ver [Seção 3.1.2](#312-resposta-customizada-com-messages_response_template)) |
| Ticketer → Plataforma | `tickets_close_template` | Plataforma / Parceiro | Campo `config.tickets_close_template` opcional (ver [Seção 3.2.1](#321-payload-customizado-com-tickets_close_template)) |
| Ticketer → Plataforma | `tickets_close_response_template` | Plataforma / Parceiro | Campo `config.tickets_close_response_template` opcional (ver [Seção 3.2.2](#322-resposta-customizada-com-tickets_close_response_template)) |
| `ticketer_uuid` | — | Plataforma | Identifica o ticketer nos webhooks de retorno |

Por padrão, `webhook_secret` é **obrigatório** e os webhooks inbound exigem HMAC. Quando a plataforma define `skip_webhook_hmac=true` no ticketer, o `webhook_secret` torna-se opcional e a verificação HMAC é desligada — use apenas em homologação ou integrações temporárias.

O parceiro deve aceitar rotação de credenciais sem downtime (validar versão atual e anterior por uma janela curta).

---

## 1. Autenticação

Por padrão, todas as requisições entre a plataforma e o Ticketer devem ser autenticadas. A exceção são os webhooks inbound quando a plataforma configura `skip_webhook_hmac=true` no ticketer (ver [Seção 1.2](#12-ticketer--plataforma-hmac)).

### 1.1 Plataforma → Ticketer (Bearer)

A plataforma envia o token configurado no campo `api_token`:

```http
Authorization: Bearer <api_token>
Content-Type: application/json
X-Request-Id: 9d81b7e2-5a4e-4fc2-b2e7-4f671e6c7770
X-API-Version: 1
```

### 1.2 Ticketer → Plataforma (HMAC)

Por padrão, os webhooks de retorno são autenticados via HMAC-SHA256 sobre o corpo bruto da requisição:

```http
Content-Type: application/json
X-Webhook-Signature: sha256=<hex(HMAC_SHA256(webhook_secret, raw_body))>
X-Webhook-Timestamp: 2026-05-20T14:35:00Z
X-Request-Id: f6a22a5a-d111-4d8a-9c44-2f9f4e0b0d65
```

Regras de verificação (quando HMAC está **habilitado**, comportamento padrão):

- O corpo deve ser o **bytes exatos** recebidos, sem normalização de JSON.
- O timestamp é em RFC3339 UTC (ou unix seconds em base-10, aceito como fallback).
- `X-Webhook-Signature` é **obrigatório**. Aceita as formas `sha256=<hex>` (recomendada) e `<hex>` puro.
- `X-Webhook-Timestamp` é **fortemente recomendado**. Quando enviado, requisições com mais de **5 minutos** de diferença do relógio atual são rejeitadas (proteção contra replay). A plataforma tolera ausência do header em v1 para facilitar migração, mas partners devem sempre enviá-lo.
- Comparação de assinatura deve usar `hmac.Equal` ou equivalente (constant-time).

#### Desabilitar HMAC (`skip_webhook_hmac`)

A plataforma pode desligar a verificação HMAC **por ticketer**, definindo na configuração:

```json
{
  "skip_webhook_hmac": "true"
}
```

Valores aceitos: `"true"`, `"1"` ou `"yes"` (case-insensitive). Default: HMAC **habilitado** (flag ausente ou qualquer outro valor).

| `skip_webhook_hmac` | `webhook_secret` | Headers exigidos nos webhooks |
|---------------------|------------------|-------------------------------|
| ausente / `false` | obrigatório | `X-Webhook-Signature` (+ `X-Webhook-Timestamp` recomendado) |
| `true` | opcional | nenhum header de autenticação |

Quando a flag está ativa, a plataforma aceita o webhook após validar o ticketer e o payload — **sem** verificar assinatura ou timestamp. O parceiro ainda deve enviar `Content-Type: application/json` e um body válido.

> **Segurança:** use `skip_webhook_hmac=true` apenas em homologação ou enquanto o parceiro implementa HMAC. Em produção, mantenha HMAC habilitado (default).

### 1.3 Modelos aceitos (Plataforma → Ticketer)

| Modelo | Uso |
|--------|-----|
| Bearer Token | Padrão recomendado |
| API Key em header | Aceitável, se enviada via header dedicado (ex.: `X-API-Key`) |
| OAuth 2.0 | Suportado mediante alinhamento prévio |

---

## 2. Plataforma → Ticketer

Endpoints que o sistema externo expõe para receber eventos da plataforma. A URL base é a registrada em `config.base_url`.

---

### 2.1 Abrir ticket

Cria um novo atendimento no sistema externo.

```http
POST /v1/tickets
```

#### Headers

```http
Authorization: Bearer <api_token>
Content-Type: application/json
X-Request-Id: 9d81b7e2-5a4e-4fc2-b2e7-4f671e6c7770
X-API-Version: 1
Idempotency-Key: open-0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71
```

#### Body

```json
{
  "ticket_id": "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
  "topic": {
    "uuid": "a1d2b8c3-9e4f-4a5b-8c6d-7e8f9a0b1c2d",
    "name": "Vendas",
    "queue_uuid": "c4d5e6f7-8a9b-4c0d-1e2f-3a4b5c6d7e8f"
  },
  "contact": {
    "uuid": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
    "name": "João Silva",
    "urn": "whatsapp:+5511999999999",
    "urns": [
      "whatsapp:+5511999999999",
      "tel:+5511999999999"
    ],
    "language": "por"
  },
  "body": "Cliente pediu atendimento humano.",
  "assignee": {
    "email": "maria@example.com",
    "name": "Maria Atendente"
  },
  "metadata": {
    "project_uuid": "f1e2d3c4-b5a6-4978-8c9d-0a1b2c3d4e5f",
    "org_uuid": "1a2b3c4d-5e6f-4708-9192-a3b4c5d6e7f8",
    "channel": {
      "uuid": "9b8a7c6d-5e4f-4302-1a2b-3c4d5e6f7a8b",
      "name": "WhatsApp BR",
      "address": "+5511888888888"
    },
    "flow": {
      "uuid": "8c7b6a5d-4e3f-4201-9a8b-7c6d5e4f3a2b",
      "name": "Fluxo de vendas"
    },
    "priority": "normal"
  },
  "opened_at": "2026-05-20T14:30:00Z"
}
```

#### Campos

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `ticket_id` | uuid | Sim | ID do ticket na plataforma |
| `topic` | object | Não | Fila, assunto ou tópico do atendimento |
| `topic.uuid` | uuid | Sim, se `topic` presente | Identificador do tópico |
| `topic.queue_uuid` | uuid | Não | Fila/setor associado ao tópico |
| `contact` | object | Sim | Dados do contato |
| `contact.uuid` | uuid | Sim | Identificador do contato na plataforma |
| `contact.urn` | string | Sim | URN preferida do contato |
| `contact.urns` | array<string> | Não | Lista completa de URNs |
| `contact.language` | string | Não | ISO 639-3 (ex.: `por`, `eng`, `spa`) |
| `body` | string | Não | Mensagem inicial ou descrição |
| `assignee` | object | Não | Agente sugerido |
| `assignee.email` | string | Sim, se `assignee` presente | Identificador primário do agente |
| `assignee.uuid` | uuid | Não (roadmap) | UUID do agente |
| `metadata` | object | Não | Dados adicionais — ver [chaves padrão](#metadata-chaves-padrão) |
| `opened_at` | string | Sim | Data de abertura em RFC3339 UTC |

#### 2.1.1 Payload customizado com `open_template`

Por padrão, a plataforma envia o body documentado acima. Quando o ticketer define `config.open_template`, esse body é **substituído** pelo resultado de um [Go `text/template`](https://pkg.go.dev/text/template) executado sobre o mesmo conjunto de dados.

- Config opcional: se `open_template` estiver ausente ou vazio, o contrato padrão é usado.
- O template deve renderizar JSON válido; caso contrário a abertura falha antes da chamada HTTP.

**Contexto disponível** (mesmas chaves do body padrão, após serialização JSON):

| Variável | Descrição |
|----------|-----------|
| `.ticket_id` | UUID do ticket na plataforma |
| `.body` | Descrição / mensagem inicial |
| `.opened_at` | Data de abertura (RFC3339) |
| `.contact` | Objeto do contato (`uuid`, `name`, `urn`, `urns`, `language`) |
| `.topic` | Objeto do tópico (quando houver) |
| `.assignee` | Objeto do agente sugerido (quando houver) |
| `.metadata` | Metadados opcionais (project, flow, channel, webhook_base_url, etc.) |

**Funções auxiliares:**

| Função | Uso |
|--------|-----|
| `json` | Serializa um valor aninhado como JSON (`{{json .contact}}`) |
| `toString` | Converte um valor para string |

**Exemplo de config (request):**

```json
{
  "base_url": "https://partner.example.com",
  "api_token": "...",
  "webhook_secret": "...",
  "open_template": "{\"id\":\"{{.ticket_id}}\",\"customer\":{{json .contact}},\"subject\":\"{{.body}}\"}"
}
```

**Body enviado nesse exemplo:**

```json
{
  "id": "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
  "customer": {
    "uuid": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
    "name": "João Silva",
    "urn": "whatsapp:+5511999999999"
  },
  "subject": "Cliente pediu atendimento humano."
}
```

> **Atenção:** ao interpolar strings dentro de JSON (`"{{.body}}"`), caracteres especiais no valor podem invalidar o JSON. Prefira `{{json .body}}` (ou `{{json .contact}}` para objetos) quando o conteúdo puder conter aspas ou quebras de linha.

#### 2.1.2 Resposta customizada com `open_response_template`

Por padrão, a plataforma espera a resposta no envelope documentado abaixo (`external_id`, `status`, `created_at`). Quando o parceiro responde em outro formato, configure `config.open_response_template` para mapear o JSON recebido para esse envelope.

- Config opcional: se `open_response_template` estiver ausente ou vazio, o body da resposta é parseado diretamente como o envelope padrão.
- O template recebe o JSON da resposta do parceiro como contexto e deve renderizar JSON válido no formato padrão.
- `external_id` é obrigatório após o mapeamento; `status` e `created_at` são opcionais.
- Erros HTTP (4xx/5xx) **não** passam pelo template de resposta — continuam no envelope de erro da [seção de erros](#resposta-de-erro).

**Exemplo:** o parceiro responde:

```json
{
  "data": {
    "id": "EXT-123456",
    "state": "open",
    "created": "2026-05-20T14:30:03Z"
  }
}
```

Config:

```json
{
  "open_response_template": "{\"external_id\":\"{{.data.id}}\",\"status\":\"{{.data.state}}\",\"created_at\":\"{{.data.created}}\"}"
}
```

Resultado interpretado pela plataforma:

```json
{
  "external_id": "EXT-123456",
  "status": "open",
  "created_at": "2026-05-20T14:30:03Z"
}
```

`open_template` e `open_response_template` são independentes: podem ser usados juntos ou isoladamente.

#### Resposta de sucesso

```http
201 Created
```

```json
{
  "external_id": "EXT-123456",
  "status": "open",
  "created_at": "2026-05-20T14:30:03Z"
}
```

#### Resposta de erro

```http
400 Bad Request
```

```json
{
  "error": "invalid_payload",
  "message": "contact.uuid is required"
}
```

#### Ticket já existente (resposta idempotente)

Se a mesma `Idempotency-Key` chegar novamente, retornar `201 Created` com o mesmo `external_id`, sem criar duplicata. Caso seja reaberto, ver `409`:

```http
409 Conflict
```

```json
{
  "error": "ticket_already_open",
  "message": "An open ticket already exists for this contact",
  "details": { "external_id": "EXT-123456" }
}
```

---

### 2.2 Encaminhar mensagem do contato

Envia ao Ticketer uma nova mensagem enviada pelo contato enquanto o ticket está aberto.

```http
POST /v1/tickets/{external_id}/messages
```

#### Exemplo

```http
POST /v1/tickets/EXT-123456/messages
```

#### Body

```json
{
  "ticket_id": "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
  "external_id": "EXT-123456",
  "message_id": "msg-789",
  "direction": "incoming",
  "sender": {
    "type": "contact",
    "id": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
    "name": "João Silva"
  },
  "text": "Olá, preciso de ajuda com meu pedido.",
  "attachments": [
    {
      "id": "att-001",
      "url": "https://example.com/files/image.jpg",
      "content_type": "image/jpeg",
      "filename": "image.jpg",
      "size": 204800
    }
  ],
  "metadata": {
    "channel": {
      "uuid": "9b8a7c6d-5e4f-4302-1a2b-3c4d5e6f7a8b",
      "name": "WhatsApp BR"
    }
  },
  "sent_at": "2026-05-20T14:32:00Z"
}
```

#### Campos

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `ticket_id` | uuid | Sim | ID do ticket na plataforma |
| `external_id` | string | Sim | ID do ticket no sistema externo |
| `message_id` | string | Recomendado | ID da mensagem na plataforma (presente quando a mensagem origina de um inbound) |
| `direction` | enum | Sim | Sempre `incoming` |
| `sender` | object | Sim | Ver [objeto `sender`](#objeto-sender) |
| `text` | string | Condicional | Obrigatório se `attachments` estiver vazio |
| `attachments` | array | Condicional | Obrigatório se `text` estiver vazio |
| `metadata` | object | Não | Dados adicionais |
| `sent_at` | string | Sim | RFC3339 UTC |

#### 2.2.1 Payload customizado com `forward_template`

Por padrão, a plataforma envia o body documentado acima. Quando o ticketer define `config.forward_template`, esse body é **substituído** pelo resultado de um Go `text/template` executado sobre o mesmo conjunto de dados.

- Config opcional: se `forward_template` estiver ausente ou vazio, o contrato padrão é usado.
- O template deve renderizar JSON válido; caso contrário o forward falha antes da chamada HTTP.

**Contexto disponível** (mesmas chaves do body padrão):

| Variável | Descrição |
|----------|-----------|
| `.ticket_id` | UUID do ticket na plataforma |
| `.external_id` | ID do ticket no sistema externo |
| `.message_id` | ID da mensagem na plataforma (quando houver) |
| `.direction` | Sempre `incoming` |
| `.sender` | Objeto do remetente (`type`, `id`, `name`, …) |
| `.text` | Texto da mensagem |
| `.attachments` | Lista de anexos |
| `.metadata` | Metadados opcionais |
| `.sent_at` | Data/hora do envio (RFC3339) |

As mesmas funções auxiliares de `open_template` estão disponíveis (`json`, `toString`).

**Exemplo de config:**

```json
{
  "forward_template": "{\"ticket\":\"{{.external_id}}\",\"from\":{{json .sender}},\"body\":\"{{.text}}\",\"msg_id\":\"{{.message_id}}\"}"
}
```

**Body enviado nesse exemplo:**

```json
{
  "ticket": "EXT-123456",
  "from": {
    "type": "contact",
    "id": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
    "name": "João Silva"
  },
  "body": "Olá, preciso de ajuda com meu pedido.",
  "msg_id": "msg-789"
}
```

#### 2.2.2 Resposta customizada com `forward_response_template`

Por padrão, a plataforma espera a resposta no envelope documentado abaixo (`message_external_id`, `status`). Quando o parceiro responde em outro formato, configure `config.forward_response_template` para mapear o JSON recebido para esse envelope.

- Config opcional: se `forward_response_template` estiver ausente ou vazio, o body da resposta é parseado diretamente como o envelope padrão.
- O template recebe o JSON da resposta do parceiro como contexto e deve renderizar JSON válido no formato padrão.
- `message_external_id` e `status` são opcionais após o mapeamento (o forward não exige `message_external_id` para sucesso).
- Erros HTTP (4xx/5xx) **não** passam pelo template de resposta.

**Exemplo:** o parceiro responde:

```json
{
  "result": {
    "id": "external-msg-456",
    "state": "received"
  }
}
```

Config:

```json
{
  "forward_response_template": "{\"message_external_id\":\"{{.result.id}}\",\"status\":\"{{.result.state}}\"}"
}
```

Resultado interpretado pela plataforma:

```json
{
  "message_external_id": "external-msg-456",
  "status": "received"
}
```

`forward_template` e `forward_response_template` são independentes: podem ser usados juntos ou isoladamente.

#### Resposta de sucesso

```http
200 OK
```

```json
{
  "message_external_id": "external-msg-456",
  "status": "received"
}
```

---

### 2.3 Fechar ticket

Notifica o Ticketer que o atendimento foi fechado na plataforma.

```http
POST /v1/tickets/{external_id}/close
```

#### Body

```json
{
  "ticket_id": "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
  "external_id": "EXT-123456",
  "closed_by": {
    "type": "platform",
    "id": "system"
  },
  "reason": "resolved",
  "metadata": {
    "source": "platform"
  },
  "closed_at": "2026-05-20T14:50:00Z"
}
```

#### Enums

`closed_by.type`:

| Valor | Significado |
|-------|-------------|
| `platform` | Fechado por sistema da plataforma |
| `agent` | Fechado por agente humano |
| `contact` | Fechado pelo próprio contato |
| `flow` | Fechado por um flow automatizado |
| `system` | Fechamento automático (timeout, manutenção) |

`reason`:

| Valor | Significado |
|-------|-------------|
| `resolved` | Atendimento concluído |
| `abandoned` | Contato não respondeu |
| `transferred` | Transferido para outro canal/equipe |
| `expired` | Timeout/inatividade |
| `cancelled` | Cancelado antes do início efetivo |
| `other` | Outro motivo (usar `details.reason_text` se necessário) |

#### 2.3.1 Payload customizado com `close_template`

Por padrão, a plataforma envia o body documentado acima. Quando o ticketer define `config.close_template`, esse body é **substituído** pelo resultado de um Go `text/template` executado sobre o mesmo conjunto de dados.

- Config opcional: se `close_template` estiver ausente ou vazio, o contrato padrão é usado.
- O template deve renderizar JSON válido; caso contrário o close falha antes da chamada HTTP.

**Contexto disponível** (mesmas chaves do body padrão):

| Variável | Descrição |
|----------|-----------|
| `.ticket_id` | UUID do ticket na plataforma |
| `.external_id` | ID do ticket no sistema externo |
| `.closed_by` | Ator que fechou (`type`, `id`, `name`) |
| `.reason` | Motivo do fechamento (quando houver) |
| `.metadata` | Metadados opcionais |
| `.closed_at` | Data/hora do fechamento (RFC3339) |

As mesmas funções auxiliares de `open_template` estão disponíveis (`json`, `toString`).

**Exemplo de config:**

```json
{
  "close_template": "{\"id\":\"{{.external_id}}\",\"by\":{{json .closed_by}},\"at\":\"{{.closed_at}}\"}"
}
```

**Body enviado nesse exemplo:**

```json
{
  "id": "EXT-123456",
  "by": {
    "type": "platform",
    "id": "system"
  },
  "at": "2026-05-20T14:50:00Z"
}
```

#### 2.3.2 Resposta customizada com `close_response_template`

Por padrão, a plataforma espera a resposta no envelope documentado abaixo (`status`). Quando o parceiro responde em outro formato, configure `config.close_response_template` para mapear o JSON recebido para esse envelope.

- Config opcional: se `close_response_template` estiver ausente ou vazio, o body da resposta é parseado diretamente como o envelope padrão (body vazio em 2xx/204 continua aceito).
- O template recebe o JSON da resposta do parceiro como contexto e deve renderizar JSON válido no formato padrão.
- `status` é opcional após o mapeamento (o close não exige `status: closed` para sucesso — o HTTP 2xx já basta).
- Erros HTTP (4xx/5xx) **não** passam pelo template de resposta — o 409 continua tratado como já fechado.

**Exemplo:** o parceiro responde:

```json
{
  "result": {
    "state": "closed"
  }
}
```

Config:

```json
{
  "close_response_template": "{\"status\":\"{{.result.state}}\"}"
}
```

Resultado interpretado pela plataforma:

```json
{
  "status": "closed"
}
```

`close_template` e `close_response_template` são independentes: podem ser usados juntos ou isoladamente.

#### Resposta de sucesso

```http
200 OK
```

```json
{
  "status": "closed"
}
```

#### Caso o ticket já esteja fechado

```http
409 Conflict
```

```json
{
  "error": "ticket_already_closed",
  "message": "Ticket is already closed"
}
```

---

### 2.4 Reabrir ticket

Notifica o Ticketer que o atendimento foi reaberto na plataforma.

```http
POST /v1/tickets/{external_id}/reopen
```

#### Body

```json
{
  "ticket_id": "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
  "external_id": "EXT-123456",
  "reopened_by": {
    "type": "platform",
    "id": "system"
  },
  "metadata": {
    "source": "platform"
  },
  "reopened_at": "2026-05-20T15:05:00Z"
}
```

`reopened_by.type` aceita os mesmos valores de `closed_by.type`.

#### Resposta de sucesso

```http
200 OK
```

```json
{
  "status": "open"
}
```

#### Caso o parceiro não suporte reabertura

```http
422 Unprocessable Entity
```

```json
{
  "error": "reopen_not_supported",
  "message": "This ticketer does not support ticket reopening"
}
```

---

### 2.5 Enviar histórico da conversa (opcional)

Envia ao Ticketer o histórico anterior da conversa após a abertura do ticket. A plataforma carrega mensagens do contato a partir de `history_after` no body do ticket (quando informado), senão usa a janela padrão de **24 horas**.

> **Ordering:** o histórico é enviado **após** `2.1` e pode chegar **depois** das primeiras mensagens novas via `2.2`. O parceiro deve ordenar internamente por `sent_at`.

#### Modos de envio (`history_mode`)

| Modo | Config | Endpoint padrão | Payload padrão |
|------|--------|-----------------|----------------|
| `batch` (default) | `history_mode=batch` ou ausente | `POST /v1/tickets/{external_id}/history` (`route_history`) | `HistoryRequest` com array `messages` (em lotes de até `history_batch_size`, default 50) |
| `one_by_one` | `history_mode=one_by_one` | `POST /v1/tickets/{external_id}/messages` (`route_history_message` ou `route_forward`) | `MessageRequest` por mensagem (mesmo contrato de [2.2](#22-encaminhar-mensagem-do-contato)) |

Ambos os modos aceitam rota customizada e payload via `history_template`.

#### Batch (default)

```http
POST /v1/tickets/{external_id}/history
```

#### Body (batch)

```json
{
  "ticket_id": "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
  "external_id": "EXT-123456",
  "contact": {
    "uuid": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
    "name": "João Silva",
    "urn": "whatsapp:+5511999999999"
  },
  "messages": [
    {
      "message_id": "msg-001",
      "direction": "outgoing",
      "sender": { "type": "bot" },
      "text": "Olá! Como posso ajudar?",
      "attachments": [],
      "sent_at": "2026-05-20T14:20:00Z"
    },
    {
      "message_id": "msg-002",
      "direction": "incoming",
      "sender": { "type": "contact", "id": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621" },
      "text": "Quero falar com um atendente.",
      "attachments": [],
      "sent_at": "2026-05-20T14:21:00Z"
    }
  ],
  "metadata": {
    "project_uuid": "f0e1d2c3-b4a5-4968-8c7d-9e0f1a2b3c4d"
  }
}
```

#### One-by-one (`history_mode=one_by_one`)

Cada mensagem do histórico é enviada individualmente para `route_history_message` (ou `route_forward` quando não configurado), usando o contrato de [2.2](#22-encaminhar-mensagem-do-contato) — inclusive mensagens `outgoing` com `sender.type=bot`.

```http
POST /v1/tickets/{external_id}/messages
```

#### 2.5.1 Payload customizado com `history_template`

Config opcional compartilhada pelos dois modos. Quando ausente, a plataforma envia o contrato padrão do modo ativo.

- No modo **batch**, o contexto inclui `.ticket_id`, `.external_id`, `.contact`, `.messages` (slice) e `.metadata`.
- No modo **one_by_one**, o contexto inclui os mesmos campos de ticket/contato **mais** os campos da mensagem atual no topo: `.message_id`, `.direction`, `.sender`, `.text`, `.attachments`, `.sent_at`.
- O template deve renderizar JSON válido.

**Exemplo (batch):**

```json
{
  "history_template": "{\"conversation\":\"{{.external_id}}\",\"items\":{{len .messages}}}"
}
```

#### 2.5.2 Resposta customizada com `history_response_template`

Por padrão, a plataforma interpreta a resposta como:

```json
{
  "status": "history_received",
  "messages_received": 2
}
```

Configure `history_response_template` para mapear o JSON do parceiro para esse envelope. No modo `one_by_one`, a resposta costuma seguir o formato de mensagem (`message_external_id`, `status`) — o template pode adaptá-la.

Erros HTTP (4xx/5xx) **não** passam pelo template de resposta.

#### Resposta de sucesso

```http
200 OK
```

```json
{
  "status": "history_received",
  "messages_received": 2
}
```

#### Observação

Endpoint opcional. Caso o parceiro não queira receber histórico, pode não implementá-lo ou retornar `200 OK` sem executar nenhuma ação. Sem mensagens na janela configurada, a plataforma não faz chamadas HTTP.

---

## 3. Ticketer → Plataforma

Webhooks que o parceiro deve chamar para enviar eventos do sistema externo de volta para a plataforma.

A URL base do Mailroom e o `ticketer_uuid` são informados no provisionamento (ver [Seção 0](#0-provisionamento-de-credenciais)). Exemplo:

```
https://platform.example.com/webhooks/ticketer/{ticketer_uuid}
```

> **Compatibilidade:** na implementação atual, esses webhooks são expostos sob `https://platform.example.com/mr/tickets/types/{ticketer_type}/event_callback/{ticketer_uuid}` — a plataforma fornecerá a URL exata no provisionamento.

Por padrão, todos os webhooks devem ser autenticados via HMAC (ver [Seção 1.2](#12-ticketer--plataforma-hmac)). Se a plataforma tiver configurado `skip_webhook_hmac=true` para o ticketer, os headers de assinatura não são exigidos — consulte o time de integração para confirmar o modo do seu ambiente.

---

### 3.1 Enviar mensagem do agente ao contato

Quando um agente responder no sistema externo, o Ticketer deve chamar a plataforma para entregar essa mensagem ao contato.

```http
POST /webhooks/ticketer/{ticketer_uuid}/messages
```

#### Exemplo

```http
POST /webhooks/ticketer/67dc9f8d-bd4d-4a97-8f8a-4d62625ff9e7/messages
```

#### Headers

Obrigatórios quando HMAC está habilitado (default). Omitidos quando a plataforma configurou `skip_webhook_hmac=true` para o ticketer.

```http
Content-Type: application/json
X-Webhook-Signature: sha256=<hex(HMAC_SHA256(webhook_secret, raw_body))>
X-Webhook-Timestamp: 2026-05-20T14:35:00Z
X-Request-Id: f6a22a5a-d111-4d8a-9c44-2f9f4e0b0d65
```

#### Body

```json
{
  "external_id": "EXT-123456",
  "message_external_id": "external-msg-999",
  "direction": "outgoing",
  "sender": {
    "type": "agent",
    "id": "agent-1",
    "name": "Maria Atendente",
    "email": "maria@example.com"
  },
  "text": "Olá, João. Vou te ajudar com seu pedido.",
  "attachments": [],
  "metadata": {
    "department": "support"
  },
  "sent_at": "2026-05-20T14:35:00Z"
}
```

#### Campos

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `external_id` | string | Sim | ID do ticket no sistema externo |
| `message_external_id` | string | Recomendado | ID da mensagem no sistema externo |
| `direction` | enum | Sim | Sempre `outgoing` |
| `sender` | object | Sim | Tipicamente `sender.type = "agent"` |
| `text` | string | Condicional | Obrigatório se `attachments` estiver vazio |
| `attachments` | array | Condicional | Obrigatório se `text` estiver vazio |
| `metadata` | object | Não | Dados adicionais |
| `sent_at` | string | Sim | RFC3339 UTC |

#### 3.1.1 Payload customizado com `messages_template`

Por padrão, a plataforma espera o body documentado acima. Quando o ticketer define `config.messages_template`, o body recebido do parceiro é **mapeado** por um Go `text/template` para esse envelope antes do processamento.

- Config opcional: se `messages_template` estiver ausente ou vazio, o body é parseado diretamente.
- O HMAC (quando habilitado) é calculado sobre o **body bruto** enviado pelo parceiro, antes do mapeamento.
- O template deve renderizar JSON válido no formato padrão (`external_id`, `text`/`attachments`, etc.).
- `external_id` continua obrigatório após o mapeamento; `text` ou `attachments` também.

**Exemplo:** o parceiro envia:

```json
{
  "ticket": "EXT-123456",
  "content": "Olá, João. Vou te ajudar com seu pedido.",
  "agent": { "name": "Maria Atendente" }
}
```

Config:

```json
{
  "messages_template": "{\"external_id\":\"{{.ticket}}\",\"direction\":\"outgoing\",\"sender\":{\"type\":\"agent\",\"name\":\"{{.agent.name}}\"},\"text\":\"{{.content}}\",\"sent_at\":\"{{.sent_at}}\"}"
}
```

> Se o parceiro não envia `sent_at`, inclua um valor fixo no template ou um campo equivalente do payload.

#### 3.1.2 Resposta customizada com `messages_response_template`

Por padrão, a plataforma responde com o envelope de sucesso abaixo. Quando o ticketer define `config.messages_response_template`, essa resposta é **substituída** pelo resultado do template.

- Config opcional: se `messages_response_template` estiver ausente ou vazio, a resposta padrão é usada.
- O template recebe o JSON da resposta padrão como contexto (`status`, `ticket_uuid`, `message_uuid` quando houver).
- Erros (4xx/5xx) **não** passam pelo template — continuam no envelope `{error, message}`.

**Contexto disponível na resposta padrão:**

| Variável | Descrição |
|----------|-----------|
| `.status` | Sempre `sent` em sucesso |
| `.ticket_uuid` | UUID do ticket na plataforma |
| `.message_uuid` | UUID da mensagem enviada (quando disponível) |

**Exemplo de config:**

```json
{
  "messages_response_template": "{\"ok\":true,\"id\":\"{{.message_uuid}}\",\"ticket\":\"{{.ticket_uuid}}\"}"
}
```

#### Resposta de sucesso

```http
200 OK
```

```json
{
  "status": "sent",
  "ticket_uuid": "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
  "message_uuid": "msg-platform-123"
}
```

---

### 3.2 Fechar ticket a partir do Ticketer

Quando o atendimento for fechado no sistema externo, o parceiro pode notificar a plataforma.

```http
POST /webhooks/ticketer/{ticketer_uuid}/tickets/close
```

#### Body

```json
{
  "external_id": "EXT-123456",
  "closed_by": {
    "type": "agent",
    "id": "agent-1",
    "name": "Maria Atendente"
  },
  "reason": "resolved",
  "metadata": {
    "source": "ticketer"
  },
  "closed_at": "2026-05-20T14:50:00Z"
}
```

Enums `closed_by.type` e `reason` seguem [Seção 2.3](#23-fechar-ticket).

#### 3.2.1 Payload customizado com `tickets_close_template`

Por padrão, a plataforma espera o body documentado acima. Quando o ticketer define `config.tickets_close_template`, o body recebido do parceiro é **mapeado** por um Go `text/template` para esse envelope antes do processamento.

- Config opcional: se `tickets_close_template` estiver ausente ou vazio, o body é parseado diretamente.
- O HMAC (quando habilitado) é calculado sobre o **body bruto** enviado pelo parceiro, antes do mapeamento.
- O template deve renderizar JSON válido no formato padrão.
- `external_id` continua obrigatório após o mapeamento.

**Exemplo:** o parceiro envia:

```json
{
  "ticket": "EXT-123456",
  "reason": "resolved",
  "closed_at": "2026-05-20T14:50:00Z"
}
```

Config:

```json
{
  "tickets_close_template": "{\"external_id\":\"{{.ticket}}\",\"reason\":\"{{.reason}}\",\"closed_at\":\"{{.closed_at}}\"}"
}
```

> **Nota:** `close_template` / `close_response_template` (seções 2.3.1–2.3.2) aplicam-se ao fechamento **plataforma → parceiro**. `tickets_close_*` aplicam-se ao webhook **parceiro → plataforma** (`POST .../tickets/close`).

#### 3.2.2 Resposta customizada com `tickets_close_response_template`

Por padrão, a plataforma responde com o envelope de sucesso abaixo. Quando o ticketer define `config.tickets_close_response_template`, essa resposta é **substituída** pelo resultado do template.

- Config opcional: se `tickets_close_response_template` estiver ausente ou vazio, a resposta padrão é usada.
- O template recebe o JSON da resposta padrão como contexto (`status`, `ticket_uuid`).
- Erros (4xx/5xx) **não** passam pelo template.

**Exemplo de config:**

```json
{
  "tickets_close_response_template": "{\"ok\":true,\"ticket\":\"{{.ticket_uuid}}\",\"state\":\"{{.status}}\"}"
}
```

#### Resposta de sucesso

```http
200 OK
```

```json
{
  "status": "closed",
  "ticket_uuid": "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71"
}
```

---

### 3.3 Reabrir ticket a partir do Ticketer

Quando o atendimento for reaberto no sistema externo, o parceiro pode notificar a plataforma.

```http
POST /webhooks/ticketer/{ticketer_uuid}/tickets/reopen
```

#### Body

```json
{
  "external_id": "EXT-123456",
  "reopened_by": {
    "type": "agent",
    "id": "agent-1",
    "name": "Maria Atendente"
  },
  "metadata": {
    "source": "ticketer"
  },
  "reopened_at": "2026-05-20T15:05:00Z"
}
```

#### Resposta de sucesso

```http
200 OK
```

```json
{
  "status": "open"
}
```

---

## 4. Consulta de ticket (opcional, roadmap)

O parceiro pode oferecer um endpoint para a plataforma consultar o estado atual de um ticket.

```http
GET /v1/tickets/{external_id}
```

> **Atenção:** a plataforma **não consome** este endpoint em runtime hoje. Ele é útil para auditoria, diagnóstico e ferramentas externas; está documentado como roadmap caso a plataforma passe a usá-lo no futuro.

#### Exemplo

```http
GET /v1/tickets/EXT-123456
```

#### Resposta

```json
{
  "external_id": "EXT-123456",
  "status": "open",
  "contact": {
    "name": "João Silva"
  },
  "created_at": "2026-05-20T14:30:03Z",
  "updated_at": "2026-05-20T14:35:00Z"
}
```

---

## 5. Objeto `sender`

```json
{
  "type": "contact",
  "id": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
  "name": "João Silva",
  "email": "joao@example.com"
}
```

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `type` | enum | Sim | `contact`, `agent`, `bot`, `system` |
| `id` | string | Recomendado | ID do remetente no sistema de origem |
| `name` | string | Não | Nome exibível |
| `email` | string | Recomendado para `agent` | Identificador primário do agente |

---

## 6. Formato dos anexos

Sempre que uma mensagem tiver anexos, eles devem seguir este formato.

```json
{
  "id": "att-001",
  "url": "https://example.com/files/document.pdf",
  "content_type": "application/pdf",
  "filename": "document.pdf",
  "size": 512000,
  "metadata": {
    "caption": "Comprovante enviado pelo cliente"
  }
}
```

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `id` | string | Não | ID do anexo |
| `url` | string | Sim | URL para download (HTTPS) |
| `content_type` | string | Não | MIME type. Se ausente, a plataforma usa o `Content-Type` retornado pela própria URL ao baixar o arquivo |
| `filename` | string | Recomendado | Nome do arquivo |
| `size` | number | Não | Tamanho em bytes |
| `metadata` | object | Não | Dados adicionais (ex.: `caption`) |

Limites atuais ao baixar anexos do parceiro: até **10 MB** por arquivo em endpoints genéricos, com exceções por tipo de mídia documentadas na integração.

---

## 7. Idempotência e retries

A plataforma pode reenviar requisições em caso de timeout, erro temporário ou falha de rede.

O parceiro deve tratar as operações como idempotentes sempre que possível.

### Headers

```http
X-Request-Id: <uuid_da_requisicao>
Idempotency-Key: <chave_unica_da_operacao>
```

> **Status atual:** o envio de `Idempotency-Key` é parte do contrato `v1` mas ainda **não é emitido por todos os caminhos da plataforma** (roadmap). O parceiro deve tratar idempotência por (`X-Request-Id`, `external_id`, `message_id`) enquanto isso.

### Política de retry

- Erros `5xx` e `429`: até **3 tentativas** com backoff exponencial.
- Erros `4xx` (exceto `408`/`429`): sem retry.
- O parceiro pode sinalizar pausa retornando `Retry-After` em `429` ou `503` (segundos ou data HTTP). A plataforma respeita o header.

### Exemplo

```http
POST /v1/tickets
Idempotency-Key: open-ticket-0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71
```

Se a mesma requisição for recebida novamente, o parceiro deve retornar o mesmo `external_id`, sem criar um ticket duplicado.

---

## 8. Códigos HTTP esperados

| Código | Uso |
|--------|-----|
| `200 OK` | Operação executada com sucesso |
| `201 Created` | Recurso criado com sucesso |
| `400 Bad Request` | Payload inválido |
| `401 Unauthorized` | Credencial ausente ou inválida |
| `403 Forbidden` | Credencial válida, mas sem permissão |
| `404 Not Found` | Ticket ou recurso não encontrado |
| `409 Conflict` | Operação incompatível com o estado atual |
| `422 Unprocessable Entity` | Operação não suportada ou dados não processáveis |
| `429 Too Many Requests` | Limite de requisições excedido (usar `Retry-After`) |
| `500 Internal Server Error` | Erro interno |
| `503 Service Unavailable` | Serviço temporariamente indisponível (usar `Retry-After`) |

---

## 9. Formato padrão de erro

Todos os erros devem retornar JSON.

```json
{
  "error": "ticket_not_found",
  "message": "Ticket EXT-123456 was not found",
  "details": {
    "external_id": "EXT-123456"
  }
}
```

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `error` | string | Sim | Código legível por máquina (snake_case) |
| `message` | string | Sim | Mensagem legível por humano |
| `details` | object | Não | Dados adicionais para diagnóstico |

---

## 10. Metadata: chaves padrão

Sempre que a plataforma enviar `metadata`, as seguintes chaves podem estar presentes (e parceiros multi-tenant devem persisti-las):

| Chave | Tipo | Origem | Descrição |
|-------|------|--------|-----------|
| `project_uuid` | uuid | Plataforma → Ticketer (`POST /v1/tickets`) | Projeto Weni que originou o ticket |
| `project_name` | string | Plataforma → Ticketer | Nome do projeto (somente quando configurado) |
| `org_uuid` | uuid | Plataforma → Ticketer | Organização |
| `org_id` | number | Plataforma → Ticketer | ID numérico legado da organização |
| `channel` | object | Plataforma → Ticketer | `{ uuid, name, address }` do canal preferido do contato |
| `flow` | object | Plataforma → Ticketer | `{ uuid, name }` do flow que abriu o ticket, se aplicável |
| `webhook_base_url` | string | Plataforma → Ticketer (`POST /v1/tickets`) | URL base que o parceiro deve usar para chamar os webhooks da [Seção 3](#3-ticketer--plataforma). Equivale a `<plataforma>/webhooks/ticketer/{ticketer_uuid}` no contrato canônico — partners pré-configurados podem ignorar este campo |
| `source_message_external_id` | string | Plataforma → Ticketer (`POST /v1/tickets/{external_id}/messages`) | ID da mensagem original no canal upstream (ex.: WhatsApp WAMID), repassado quando disponível para correlação |
| `contact_uuid` | uuid | Plataforma → Ticketer | Redundante com `contact.uuid`, mantido por compatibilidade |
| `priority` | enum | Plataforma → Ticketer | `low`, `normal`, `high`, `urgent` |

Parceiros podem incluir chaves próprias em `metadata` nos webhooks de retorno — a plataforma armazena sem interpretar.

---

## 11. Limites operacionais

| Limite | Valor |
|--------|-------|
| Tamanho máximo de body (request) | **32 MB** |
| Tamanho máximo de anexo (download) | **10 MB** padrão; até 100 MB para tipos específicos |
| Timeout por requisição | **30 s** |
| Janela de validade do `X-Webhook-Timestamp` | **5 min** |
| Tentativas de retry | até **3** com backoff exponencial |

---

## 12. Versionamento

O contrato segue versionamento explícito via prefixo de URL e header:

- **URL:** `/v1/tickets`, `/v1/tickets/{external_id}/messages`, etc.
- **Header:** `X-API-Version: 1`

Mudanças breaking só ocorrem entre versões maiores (`/v2/...`). Adições de campos opcionais são compatíveis e não exigem nova versão.

---

## 13. Sequência principal da integração

```
0. Provisionamento (uma vez)
   - Parceiro gera api_token, plataforma gera webhook_secret e ticketer_uuid

1. Plataforma abre ticket
   POST /v1/tickets

2. Ticketer retorna o ID externo
   201 Created { "external_id": "EXT-123456" }

3. (opcional) Plataforma envia histórico
   POST /v1/tickets/EXT-123456/history

4. Contato envia mensagem
   POST /v1/tickets/EXT-123456/messages

5. Agente responde no Ticketer
   POST /webhooks/ticketer/{ticketer_uuid}/messages

6. Plataforma entrega a resposta ao contato

7. Plataforma ou Ticketer fecha o atendimento
   POST /v1/tickets/EXT-123456/close
   ou
   POST /webhooks/ticketer/{ticketer_uuid}/tickets/close
```

---

## 14. Checklist de implementação do parceiro

### 14.1 Endpoints Plataforma → Ticketer

| Item | Obrigatório |
|------|-------------|
| `POST /v1/tickets` — abertura | Sim |
| `POST /v1/tickets/{external_id}/messages` — mensagens do contato | Sim |
| `POST /v1/tickets/{external_id}/close` — fechamento | Sim |
| `POST /v1/tickets/{external_id}/reopen` — reabertura | Opcional |
| `POST /v1/tickets/{external_id}/history` — histórico | Opcional |
| `GET /v1/tickets/{external_id}` — consulta | Opcional (roadmap) |

### 14.2 Webhooks Ticketer → Plataforma

| Item | Obrigatório |
|------|-------------|
| `POST /webhooks/ticketer/{ticketer_uuid}/messages` — resposta do agente | Sim |
| `POST /webhooks/ticketer/{ticketer_uuid}/tickets/close` — fechamento externo | Recomendado |
| `POST /webhooks/ticketer/{ticketer_uuid}/tickets/reopen` — reabertura externa | Opcional |

### 14.3 Operacional

| Item | Obrigatório |
|------|-------------|
| Bearer Token na entrada (Plataforma → Ticketer) | Sim |
| HMAC-SHA256 na saída (Ticketer → Plataforma) | Sim (default); dispensável quando a plataforma define `skip_webhook_hmac=true` |
| Suporte a anexos via URL HTTPS | Recomendado |
| Idempotência em abertura e envio de mensagens | Recomendado |
| Erros em JSON com códigos claros | Sim |
| Respeitar `Retry-After` em respostas `429`/`503` | Sim |
| Header `X-API-Version: 1` | Sim |

---

## 15. Contrato mínimo para homologação

Para uma primeira versão funcional, o parceiro precisa implementar no mínimo:

```
POST /v1/tickets
POST /v1/tickets/{external_id}/messages
POST /v1/tickets/{external_id}/close
POST /webhooks/ticketer/{ticketer_uuid}/messages
```

Mais:

- Autenticação Bearer na entrada
- Verificação HMAC na saída (ou confirmação com o time de integração de que `skip_webhook_hmac=true` está ativo no ambiente de homologação)
- Resposta de erro em JSON conforme [Seção 9](#9-formato-padrão-de-erro)

Com isso, a integração cobre o ciclo básico:

```
abrir atendimento → encaminhar mensagens do contato → agente responder → fechar atendimento
```
