# External documentation for Generic Ticketer integration

## Overview

The **Ticketer** is an external human-handoff system.

When a conversation needs to be transferred to human agents, the platform creates a ticket in an external system such as a helpdesk, CRM, contact center, or chat tool. From that point on, the platform and the Ticketer exchange events to keep the conversation in sync.

This documentation describes the HTTP contract that a partner must implement to be compatible as a Ticketer, without relying on internal platform details.

> **Note:** this document describes the **proposed contract** (`v1`). The current Mailroom implementation exposes return webhooks at `/mr/tickets/types/{ticketer_type}/event_callback/{ticketer_uuid}` — during the transition period, the final URL may vary by integration and will be provided during provisioning. Items marked **(roadmap)** are not yet supported by the platform.

---

## 0. Credential provisioning

Before the first traffic, the platform and the partner exchange credentials:

| Direction | Credential / config | Who defines | Where it lives |
|-----------|---------------------|-------------|----------------|
| Platform → Ticketer | `api_token` | Partner | Ticketer `config.api_token` field |
| Ticketer → Platform | `webhook_secret` | Platform | Ticketer `config.webhook_secret` field (shared with the partner during provisioning) |
| Ticketer → Platform | `skip_webhook_hmac` | Platform | Ticketer `config.skip_webhook_hmac` field (see [Section 1.2](#12-ticketer--platform-hmac)) |
| Ticketer base URL | — | Partner | `config.base_url` field |
| Platform → Ticketer | `open_template` | Platform / Partner | Optional `config.open_template` field (see [Section 2.1.1](#211-custom-payload-with-open_template)) |
| Platform → Ticketer | `open_response_template` | Platform / Partner | Optional `config.open_response_template` field (see [Section 2.1.2](#212-custom-response-with-open_response_template)) |
| Platform → Ticketer | `forward_template` | Platform / Partner | Optional `config.forward_template` field (see [Section 2.2.1](#221-custom-payload-with-forward_template)) |
| Platform → Ticketer | `forward_response_template` | Platform / Partner | Optional `config.forward_response_template` field (see [Section 2.2.2](#222-custom-response-with-forward_response_template)) |
| Platform → Ticketer | `close_template` | Platform / Partner | Optional `config.close_template` field (see [Section 2.3.1](#231-custom-payload-with-close_template)) |
| Platform → Ticketer | `close_response_template` | Platform / Partner | Optional `config.close_response_template` field (see [Section 2.3.2](#232-custom-response-with-close_response_template)) |
| Platform → Ticketer | `history_mode` | Platform / Partner | Optional `config.history_mode` field: `batch` (default) or `one_by_one` (see [Section 2.5](#25-send-conversation-history-optional)) |
| Platform → Ticketer | `history_batch_size` | Platform / Partner | Optional `config.history_batch_size` field; default `50` in `batch` mode |
| Platform → Ticketer | `route_history` | Platform / Partner | Override for the batch history route; default `/v1/tickets/{external_id}/history` |
| Platform → Ticketer | `route_history_message` | Platform / Partner | Override for the route in `one_by_one` mode; default same as `route_forward` (`/v1/tickets/{external_id}/messages`) |
| Platform → Ticketer | `history_template` | Platform / Partner | Optional `config.history_template` field (see [Section 2.5.1](#251-custom-payload-with-history_template)) |
| Platform → Ticketer | `history_response_template` | Platform / Partner | Optional `config.history_response_template` field (see [Section 2.5.2](#252-custom-response-with-history_response_template)) |
| Ticketer → Platform | `messages_template` | Platform / Partner | Optional `config.messages_template` field (see [Section 3.1.1](#311-custom-payload-with-messages_template)) |
| Ticketer → Platform | `messages_response_template` | Platform / Partner | Optional `config.messages_response_template` field (see [Section 3.1.2](#312-custom-response-with-messages_response_template)) |
| Ticketer → Platform | `tickets_close_template` | Platform / Partner | Optional `config.tickets_close_template` field (see [Section 3.2.1](#321-custom-payload-with-tickets_close_template)) |
| Ticketer → Platform | `tickets_close_response_template` | Platform / Partner | Optional `config.tickets_close_response_template` field (see [Section 3.2.2](#322-custom-response-with-tickets_close_response_template)) |
| `ticketer_uuid` | — | Platform | Identifies the ticketer in return webhooks |

By default, `webhook_secret` is **required** and inbound webhooks require HMAC. When the platform sets `skip_webhook_hmac=true` on the ticketer, `webhook_secret` becomes optional and HMAC verification is disabled — use only in staging or temporary integrations.

The partner must accept credential rotation without downtime (validate current and previous versions for a short window).

---

## 1. Authentication

By default, all requests between the platform and the Ticketer must be authenticated. The exception is inbound webhooks when the platform configures `skip_webhook_hmac=true` on the ticketer (see [Section 1.2](#12-ticketer--platform-hmac)).

### 1.1 Platform → Ticketer (Bearer)

The platform sends the token configured in the `api_token` field:

```http
Authorization: Bearer <api_token>
Content-Type: application/json
X-Request-Id: 9d81b7e2-5a4e-4fc2-b2e7-4f671e6c7770
X-API-Version: 1
```

### 1.2 Ticketer → Platform (HMAC)

By default, return webhooks are authenticated via HMAC-SHA256 over the raw request body:

```http
Content-Type: application/json
X-Webhook-Signature: sha256=<hex(HMAC_SHA256(webhook_secret, raw_body))>
X-Webhook-Timestamp: 2026-05-20T14:35:00Z
X-Request-Id: f6a22a5a-d111-4d8a-9c44-2f9f4e0b0d65
```

Verification rules (when HMAC is **enabled**, default behavior):

- The body must be the **exact bytes** received, with no JSON normalization.
- The timestamp is RFC3339 UTC (or unix seconds in base-10, accepted as a fallback).
- `X-Webhook-Signature` is **required**. Accepts the forms `sha256=<hex>` (recommended) and plain `<hex>`.
- `X-Webhook-Timestamp` is **strongly recommended**. When sent, requests more than **5 minutes** off the current clock are rejected (replay protection). The platform tolerates a missing header in v1 to ease migration, but partners should always send it.
- Signature comparison must use `hmac.equal` or an equivalent (constant-time).

#### Disable HMAC (`skip_webhook_hmac`)

The platform can turn off HMAC verification **per ticketer** by setting in the configuration:

```json
{
  "skip_webhook_hmac": "true"
}
```

Accepted values: `"true"`, `"1"`, or `"yes"` (case-insensitive). Default: HMAC **enabled** (flag absent or any other value).

| `skip_webhook_hmac` | `webhook_secret` | Required headers on webhooks |
|---------------------|------------------|------------------------------|
| absent / `false` | required | `X-Webhook-Signature` (+ `X-Webhook-Timestamp` recommended) |
| `true` | optional | no authentication headers |

When the flag is active, the platform accepts the webhook after validating the ticketer and payload — **without** verifying signature or timestamp. The partner must still send `Content-Type: application/json` and a valid body.

> **Security:** use `skip_webhook_hmac=true` only in staging or while the partner implements HMAC. In production, keep HMAC enabled (default).

### 1.3 Accepted models (Platform → Ticketer)

| Model | Usage |
|-------|-------|
| Bearer Token | Recommended default |
| API Key in header | Acceptable if sent via a dedicated header (e.g. `X-API-Key`) |
| OAuth 2.0 | Supported with prior alignment |

---

## 2. Platform → Ticketer

Endpoints that the external system exposes to receive events from the platform. The base URL is the one registered in `config.base_url`.

---

### 2.1 Open ticket

Creates a new conversation in the external system.

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
    "name": "Sales",
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
  "body": "Customer requested human support.",
  "assignee": {
    "email": "maria@example.com",
    "name": "Maria Agent"
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
      "name": "Sales flow"
    },
    "priority": "normal"
  },
  "opened_at": "2026-05-20T14:30:00Z"
}
```

#### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ticket_id` | uuid | Yes | Ticket ID on the platform |
| `topic` | object | No | Queue, subject, or topic for the conversation |
| `topic.uuid` | uuid | Yes, if `topic` present | Topic identifier |
| `topic.queue_uuid` | uuid | No | Queue/team associated with the topic |
| `contact` | object | Yes | Contact data |
| `contact.uuid` | uuid | Yes | Contact identifier on the platform |
| `contact.urn` | string | Yes | Contact's preferred URN |
| `contact.urns` | array<string> | No | Full list of URNs |
| `contact.language` | string | No | ISO 639-3 (e.g. `por`, `eng`, `spa`) |
| `body` | string | No | Initial message or description |
| `assignee` | object | No | Suggested agent |
| `assignee.email` | string | Yes, if `assignee` present | Primary agent identifier |
| `assignee.uuid` | uuid | No (roadmap) | Agent UUID |
| `metadata` | object | No | Additional data — see [standard keys](#10-metadata-standard-keys) |
| `opened_at` | string | Yes | Open date in RFC3339 UTC |

#### 2.1.1 Custom payload with `open_template`

By default, the platform sends the body documented above. When the ticketer defines `config.open_template`, that body is **replaced** by the result of a [Go `text/template`](https://pkg.go.dev/text/template) executed over the same data set.

- Optional config: if `open_template` is absent or empty, the standard contract is used.
- The template must render valid JSON; otherwise opening fails before the HTTP call.

**Available context** (same keys as the standard body, after JSON serialization):

| Variable | Description |
|----------|-------------|
| `.ticket_id` | Ticket UUID on the platform |
| `.body` | Description / initial message |
| `.opened_at` | Open date (RFC3339) |
| `.contact` | Contact object (`uuid`, `name`, `urn`, `urns`, `language`) |
| `.topic` | Topic object (when present) |
| `.assignee` | Suggested agent object (when present) |
| `.metadata` | Optional metadata (project, flow, channel, webhook_base_url, etc.) |

**Helper functions:**

| Function | Usage |
|----------|-------|
| `json` | Serializes a nested value as JSON (`{{json .contact}}`) |
| `toString` | Converts a value to string |

**Config example (request):**

```json
{
  "base_url": "https://partner.example.com",
  "api_token": "...",
  "webhook_secret": "...",
  "open_template": "{\"id\":\"{{.ticket_id}}\",\"customer\":{{json .contact}},\"subject\":\"{{.body}}\"}"
}
```

**Body sent in this example:**

```json
{
  "id": "0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71",
  "customer": {
    "uuid": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
    "name": "João Silva",
    "urn": "whatsapp:+5511999999999"
  },
  "subject": "Customer requested human support."
}
```

> **Warning:** when interpolating strings inside JSON (`"{{.body}}"`), special characters in the value can invalidate the JSON. Prefer `{{json .body}}` (or `{{json .contact}}` for objects) when the content may contain quotes or line breaks.

#### 2.1.2 Custom response with `open_response_template`

By default, the platform expects the response in the envelope documented below (`external_id`, `status`, `created_at`). When the partner responds in another format, configure `config.open_response_template` to map the received JSON to that envelope.

- Optional config: if `open_response_template` is absent or empty, the response body is parsed directly as the standard envelope.
- The template receives the partner's response JSON as context and must render valid JSON in the standard format.
- `external_id` is required after mapping; `status` and `created_at` are optional.
- HTTP errors (4xx/5xx) do **not** go through the response template — they continue in the error envelope from the [error response](#error-response) section.

**Example:** the partner responds:

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

Result interpreted by the platform:

```json
{
  "external_id": "EXT-123456",
  "status": "open",
  "created_at": "2026-05-20T14:30:03Z"
}
```

`open_template` and `open_response_template` are independent: they can be used together or separately.

#### Success response

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

#### Error response

```http
400 Bad Request
```

```json
{
  "error": "invalid_payload",
  "message": "contact.uuid is required"
}
```

#### Ticket already exists (idempotent response)

If the same `Idempotency-Key` arrives again, return `201 Created` with the same `external_id`, without creating a duplicate. If it is a reopen conflict, see `409`:

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

### 2.2 Forward contact message

Sends the Ticketer a new message from the contact while the ticket is open.

```http
POST /v1/tickets/{external_id}/messages
```

#### Example

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
  "text": "Hi, I need help with my order.",
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

#### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ticket_id` | uuid | Yes | Ticket ID on the platform |
| `external_id` | string | Yes | Ticket ID in the external system |
| `message_id` | string | Recommended | Message ID on the platform (present when the message originates from an inbound) |
| `direction` | enum | Yes | Always `incoming` |
| `sender` | object | Yes | See [`sender` object](#5-sender-object) |
| `text` | string | Conditional | Required if `attachments` is empty |
| `attachments` | array | Conditional | Required if `text` is empty |
| `metadata` | object | No | Additional data |
| `sent_at` | string | Yes | RFC3339 UTC |

#### 2.2.1 Custom payload with `forward_template`

By default, the platform sends the body documented above. When the ticketer defines `config.forward_template`, that body is **replaced** by the result of a Go `text/template` executed over the same data set.

- Optional config: if `forward_template` is absent or empty, the standard contract is used.
- The template must render valid JSON; otherwise the forward fails before the HTTP call.

**Available context** (same keys as the standard body):

| Variable | Description |
|----------|-------------|
| `.ticket_id` | Ticket UUID on the platform |
| `.external_id` | Ticket ID in the external system |
| `.message_id` | Message ID on the platform (when present) |
| `.direction` | Always `incoming` |
| `.sender` | Sender object (`type`, `id`, `name`, …) |
| `.text` | Message text |
| `.attachments` | Attachment list |
| `.metadata` | Optional metadata |
| `.sent_at` | Send date/time (RFC3339) |

The same helper functions as `open_template` are available (`json`, `toString`).

**Config example:**

```json
{
  "forward_template": "{\"ticket\":\"{{.external_id}}\",\"from\":{{json .sender}},\"body\":\"{{.text}}\",\"msg_id\":\"{{.message_id}}\"}"
}
```

**Body sent in this example:**

```json
{
  "ticket": "EXT-123456",
  "from": {
    "type": "contact",
    "id": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
    "name": "João Silva"
  },
  "body": "Hi, I need help with my order.",
  "msg_id": "msg-789"
}
```

#### 2.2.2 Custom response with `forward_response_template`

By default, the platform expects the response in the envelope documented below (`message_external_id`, `status`). When the partner responds in another format, configure `config.forward_response_template` to map the received JSON to that envelope.

- Optional config: if `forward_response_template` is absent or empty, the response body is parsed directly as the standard envelope.
- The template receives the partner's response JSON as context and must render valid JSON in the standard format.
- `message_external_id` and `status` are optional after mapping (forward does not require `message_external_id` for success).
- HTTP errors (4xx/5xx) do **not** go through the response template.

**Example:** the partner responds:

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

Result interpreted by the platform:

```json
{
  "message_external_id": "external-msg-456",
  "status": "received"
}
```

`forward_template` and `forward_response_template` are independent: they can be used together or separately.

#### Success response

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

### 2.3 Close ticket

Notifies the Ticketer that the conversation was closed on the platform.

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

| Value | Meaning |
|-------|---------|
| `platform` | Closed by a platform system |
| `agent` | Closed by a human agent |
| `contact` | Closed by the contact |
| `flow` | Closed by an automated flow |
| `system` | Automatic close (timeout, maintenance) |

`reason`:

| Value | Meaning |
|-------|---------|
| `resolved` | Conversation completed |
| `abandoned` | Contact did not reply |
| `transferred` | Transferred to another channel/team |
| `expired` | Timeout/inactivity |
| `cancelled` | Cancelled before effective start |
| `other` | Other reason (use `details.reason_text` if needed) |

#### 2.3.1 Custom payload with `close_template`

By default, the platform sends the body documented above. When the ticketer defines `config.close_template`, that body is **replaced** by the result of a Go `text/template` executed over the same data set.

- Optional config: if `close_template` is absent or empty, the standard contract is used.
- The template must render valid JSON; otherwise the close fails before the HTTP call.

**Available context** (same keys as the standard body):

| Variable | Description |
|----------|-------------|
| `.ticket_id` | Ticket UUID on the platform |
| `.external_id` | Ticket ID in the external system |
| `.closed_by` | Actor who closed (`type`, `id`, `name`) |
| `.reason` | Close reason (when present) |
| `.metadata` | Optional metadata |
| `.closed_at` | Close date/time (RFC3339) |

The same helper functions as `open_template` are available (`json`, `toString`).

**Config example:**

```json
{
  "close_template": "{\"id\":\"{{.external_id}}\",\"by\":{{json .closed_by}},\"at\":\"{{.closed_at}}\"}"
}
```

**Body sent in this example:**

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

#### 2.3.2 Custom response with `close_response_template`

By default, the platform expects the response in the envelope documented below (`status`). When the partner responds in another format, configure `config.close_response_template` to map the received JSON to that envelope.

- Optional config: if `close_response_template` is absent or empty, the response body is parsed directly as the standard envelope (empty body on 2xx/204 is still accepted).
- The template receives the partner's response JSON as context and must render valid JSON in the standard format.
- `status` is optional after mapping (close does not require `status: closed` for success — HTTP 2xx is enough).
- HTTP errors (4xx/5xx) do **not** go through the response template — 409 continues to be treated as already closed.

**Example:** the partner responds:

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

Result interpreted by the platform:

```json
{
  "status": "closed"
}
```

`close_template` and `close_response_template` are independent: they can be used together or separately.

#### Success response

```http
200 OK
```

```json
{
  "status": "closed"
}
```

#### If the ticket is already closed

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

### 2.4 Reopen ticket

Notifies the Ticketer that the conversation was reopened on the platform.

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

`reopened_by.type` accepts the same values as `closed_by.type`.

#### Success response

```http
200 OK
```

```json
{
  "status": "open"
}
```

#### If the partner does not support reopening

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

### 2.5 Send conversation history (optional)

Sends the Ticketer the previous conversation history after the ticket is opened. The platform loads contact messages from `history_after` in the ticket body (when provided); otherwise it uses the default window of **24 hours**.

> **Ordering:** history is sent **after** `2.1` and may arrive **after** the first new messages via `2.2`. The partner must order internally by `sent_at`.

#### Send modes (`history_mode`)

| Mode | Config | Default endpoint | Default payload |
|------|--------|------------------|-----------------|
| `batch` (default) | `history_mode=batch` or absent | `POST /v1/tickets/{external_id}/history` (`route_history`) | `HistoryRequest` with `messages` array (in batches of up to `history_batch_size`, default 50) |
| `one_by_one` | `history_mode=one_by_one` | `POST /v1/tickets/{external_id}/messages` (`route_history_message` or `route_forward`) | `MessageRequest` per message (same contract as [2.2](#22-forward-contact-message)) |

Both modes accept a custom route and payload via `history_template`.

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
      "text": "Hi! How can I help?",
      "attachments": [],
      "sent_at": "2026-05-20T14:20:00Z"
    },
    {
      "message_id": "msg-002",
      "direction": "incoming",
      "sender": { "type": "contact", "id": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621" },
      "text": "I want to talk to an agent.",
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

Each history message is sent individually to `route_history_message` (or `route_forward` when not configured), using the [2.2](#22-forward-contact-message) contract — including `outgoing` messages with `sender.type=bot`.

```http
POST /v1/tickets/{external_id}/messages
```

#### 2.5.1 Custom payload with `history_template`

Optional config shared by both modes. When absent, the platform sends the standard contract for the active mode.

- In **batch** mode, the context includes `.ticket_id`, `.external_id`, `.contact`, `.messages` (slice), and `.metadata`.
- In **one_by_one** mode, the context includes the same ticket/contact fields **plus** the current message fields at the top level: `.message_id`, `.direction`, `.sender`, `.text`, `.attachments`, `.sent_at`.
- The template must render valid JSON.

**Example (batch):**

```json
{
  "history_template": "{\"conversation\":\"{{.external_id}}\",\"items\":{{len .messages}}}"
}
```

#### 2.5.2 Custom response with `history_response_template`

By default, the platform interprets the response as:

```json
{
  "status": "history_received",
  "messages_received": 2
}
```

Configure `history_response_template` to map the partner's JSON to that envelope. In `one_by_one` mode, the response usually follows the message format (`message_external_id`, `status`) — the template can adapt it.

HTTP errors (4xx/5xx) do **not** go through the response template.

#### Success response

```http
200 OK
```

```json
{
  "status": "history_received",
  "messages_received": 2
}
```

#### Note

Optional endpoint. If the partner does not want to receive history, they may not implement it or return `200 OK` without performing any action. With no messages in the configured window, the platform makes no HTTP calls.

---

## 3. Ticketer → Platform

Webhooks that the partner must call to send events from the external system back to the platform.

The Mailroom base URL and `ticketer_uuid` are provided during provisioning (see [Section 0](#0-credential-provisioning)). Example:

```
https://platform.example.com/webhooks/ticketer/{ticketer_uuid}
```

> **Compatibility:** in the current implementation, these webhooks are exposed under `https://platform.example.com/mr/tickets/types/{ticketer_type}/event_callback/{ticketer_uuid}` — the platform will provide the exact URL during provisioning.

By default, all webhooks must be authenticated via HMAC (see [Section 1.2](#12-ticketer--platform-hmac)). If the platform has configured `skip_webhook_hmac=true` for the ticketer, signature headers are not required — check with the integration team to confirm the mode for your environment.

---

### 3.1 Send agent message to contact

When an agent replies in the external system, the Ticketer must call the platform to deliver that message to the contact.

```http
POST /webhooks/ticketer/{ticketer_uuid}/messages
```

#### Example

```http
POST /webhooks/ticketer/67dc9f8d-bd4d-4a97-8f8a-4d62625ff9e7/messages
```

#### Headers

Required when HMAC is enabled (default). Omitted when the platform configured `skip_webhook_hmac=true` for the ticketer.

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
    "name": "Maria Agent",
    "email": "maria@example.com"
  },
  "text": "Hi, João. I'll help you with your order.",
  "attachments": [],
  "metadata": {
    "department": "support"
  },
  "sent_at": "2026-05-20T14:35:00Z"
}
```

#### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `external_id` | string | Yes | Ticket ID in the external system |
| `message_external_id` | string | Recommended | Message ID in the external system |
| `direction` | enum | Yes | Always `outgoing` |
| `sender` | object | Yes | Typically `sender.type = "agent"` |
| `text` | string | Conditional | Required if `attachments` is empty |
| `attachments` | array | Conditional | Required if `text` is empty |
| `metadata` | object | No | Additional data |
| `sent_at` | string | Yes | RFC3339 UTC |

#### 3.1.1 Custom payload with `messages_template`

By default, the platform expects the body documented above. When the ticketer defines `config.messages_template`, the body received from the partner is **mapped** by a Go `text/template` to that envelope before processing.

- Optional config: if `messages_template` is absent or empty, the body is parsed directly.
- HMAC (when enabled) is computed over the **raw body** sent by the partner, before mapping.
- The template must render valid JSON in the standard format (`external_id`, `text`/`attachments`, etc.).
- `external_id` remains required after mapping; `text` or `attachments` as well.

**Example:** the partner sends:

```json
{
  "ticket": "EXT-123456",
  "content": "Hi, João. I'll help you with your order.",
  "agent": { "name": "Maria Agent" }
}
```

Config:

```json
{
  "messages_template": "{\"external_id\":\"{{.ticket}}\",\"direction\":\"outgoing\",\"sender\":{\"type\":\"agent\",\"name\":\"{{.agent.name}}\"},\"text\":\"{{.content}}\",\"sent_at\":\"{{.sent_at}}\"}"
}
```

> If the partner does not send `sent_at`, include a fixed value in the template or an equivalent field from the payload.

#### 3.1.2 Custom response with `messages_response_template`

By default, the platform responds with the success envelope below. When the ticketer defines `config.messages_response_template`, that response is **replaced** by the template result.

- Optional config: if `messages_response_template` is absent or empty, the standard response is used.
- The template receives the standard response JSON as context (`status`, `ticket_uuid`, `message_uuid` when present).
- Errors (4xx/5xx) do **not** go through the template — they continue in the `{error, message}` envelope.

**Available context in the standard response:**

| Variable | Description |
|----------|-------------|
| `.status` | Always `sent` on success |
| `.ticket_uuid` | Ticket UUID on the platform |
| `.message_uuid` | Sent message UUID (when available) |

**Config example:**

```json
{
  "messages_response_template": "{\"ok\":true,\"id\":\"{{.message_uuid}}\",\"ticket\":\"{{.ticket_uuid}}\"}"
}
```

#### Success response

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

### 3.2 Close ticket from the Ticketer

When the conversation is closed in the external system, the partner can notify the platform.

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
    "name": "Maria Agent"
  },
  "reason": "resolved",
  "metadata": {
    "source": "ticketer"
  },
  "closed_at": "2026-05-20T14:50:00Z"
}
```

`closed_by.type` and `reason` enums follow [Section 2.3](#23-close-ticket).

#### 3.2.1 Custom payload with `tickets_close_template`

By default, the platform expects the body documented above. When the ticketer defines `config.tickets_close_template`, the body received from the partner is **mapped** by a Go `text/template` to that envelope before processing.

- Optional config: if `tickets_close_template` is absent or empty, the body is parsed directly.
- HMAC (when enabled) is computed over the **raw body** sent by the partner, before mapping.
- The template must render valid JSON in the standard format.
- `external_id` remains required after mapping.

**Example:** the partner sends:

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

> **Note:** `close_template` / `close_response_template` (sections 2.3.1–2.3.2) apply to **platform → partner** close. `tickets_close_*` apply to the **partner → platform** webhook (`POST .../tickets/close`).

#### 3.2.2 Custom response with `tickets_close_response_template`

By default, the platform responds with the success envelope below. When the ticketer defines `config.tickets_close_response_template`, that response is **replaced** by the template result.

- Optional config: if `tickets_close_response_template` is absent or empty, the standard response is used.
- The template receives the standard response JSON as context (`status`, `ticket_uuid`).
- Errors (4xx/5xx) do **not** go through the template.

**Config example:**

```json
{
  "tickets_close_response_template": "{\"ok\":true,\"ticket\":\"{{.ticket_uuid}}\",\"state\":\"{{.status}}\"}"
}
```

#### Success response

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

### 3.3 Reopen ticket from the Ticketer

When the conversation is reopened in the external system, the partner can notify the platform.

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
    "name": "Maria Agent"
  },
  "metadata": {
    "source": "ticketer"
  },
  "reopened_at": "2026-05-20T15:05:00Z"
}
```

#### Success response

```http
200 OK
```

```json
{
  "status": "open"
}
```

---

## 4. Ticket lookup (optional, roadmap)

The partner may offer an endpoint for the platform to query the current state of a ticket.

```http
GET /v1/tickets/{external_id}
```

> **Warning:** the platform does **not** consume this endpoint at runtime today. It is useful for audit, diagnostics, and external tools; it is documented as roadmap in case the platform starts using it in the future.

#### Example

```http
GET /v1/tickets/EXT-123456
```

#### Response

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

## 5. `sender` object

```json
{
  "type": "contact",
  "id": "7ad9d98e-321f-4c61-9a50-79b1c7d7f621",
  "name": "João Silva",
  "email": "joao@example.com"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | enum | Yes | `contact`, `agent`, `bot`, `system` |
| `id` | string | Recommended | Sender ID in the origin system |
| `name` | string | No | Display name |
| `email` | string | Recommended for `agent` | Primary agent identifier |

---

## 6. Attachment format

Whenever a message has attachments, they must follow this format.

```json
{
  "id": "att-001",
  "url": "https://example.com/files/document.pdf",
  "content_type": "application/pdf",
  "filename": "document.pdf",
  "size": 512000,
  "metadata": {
    "caption": "Receipt sent by the customer"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | No | Attachment ID |
| `url` | string | Yes | Download URL (HTTPS) |
| `content_type` | string | No | MIME type. If absent, the platform uses the `Content-Type` returned by the URL itself when downloading the file |
| `filename` | string | Recommended | File name |
| `size` | number | No | Size in bytes |
| `metadata` | object | No | Additional data (e.g. `caption`) |

Current limits when downloading partner attachments: up to **10 MB** per file on generic endpoints, with media-type exceptions documented in the integration.

---

## 7. Idempotency and retries

The platform may resend requests on timeout, temporary error, or network failure.

The partner should treat operations as idempotent whenever possible.

### Headers

```http
X-Request-Id: <request_uuid>
Idempotency-Key: <unique_operation_key>
```

> **Current status:** sending `Idempotency-Key` is part of the `v1` contract but is still **not emitted on all platform paths** (roadmap). The partner should handle idempotency by (`X-Request-Id`, `external_id`, `message_id`) in the meantime.

### Retry policy

- `5xx` and `429` errors: up to **3 attempts** with exponential backoff.
- `4xx` errors (except `408`/`429`): no retry.
- The partner can signal a pause by returning `Retry-After` on `429` or `503` (seconds or HTTP date). The platform respects the header.

### Example

```http
POST /v1/tickets
Idempotency-Key: open-ticket-0f4d2c8a-2c83-4f2c-9f7d-1d4f70d50e71
```

If the same request is received again, the partner must return the same `external_id`, without creating a duplicate ticket.

---

## 8. Expected HTTP status codes

| Code | Usage |
|------|-------|
| `200 OK` | Operation completed successfully |
| `201 Created` | Resource created successfully |
| `400 Bad Request` | Invalid payload |
| `401 Unauthorized` | Missing or invalid credential |
| `403 Forbidden` | Valid credential, but no permission |
| `404 Not Found` | Ticket or resource not found |
| `409 Conflict` | Operation incompatible with current state |
| `422 Unprocessable Entity` | Operation not supported or data not processable |
| `429 Too Many Requests` | Request limit exceeded (use `Retry-After`) |
| `500 Internal Server Error` | Internal error |
| `503 Service Unavailable` | Service temporarily unavailable (use `Retry-After`) |

---

## 9. Standard error format

All errors must return JSON.

```json
{
  "error": "ticket_not_found",
  "message": "Ticket EXT-123456 was not found",
  "details": {
    "external_id": "EXT-123456"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `error` | string | Yes | Machine-readable code (snake_case) |
| `message` | string | Yes | Human-readable message |
| `details` | object | No | Additional diagnostic data |

---

## 10. Metadata: standard keys

Whenever the platform sends `metadata`, the following keys may be present (and multi-tenant partners should persist them):

| Key | Type | Origin | Description |
|-----|------|--------|-------------|
| `project_uuid` | uuid | Platform → Ticketer (`POST /v1/tickets`) | Weni project that originated the ticket |
| `project_name` | string | Platform → Ticketer | Project name (only when configured) |
| `org_uuid` | uuid | Platform → Ticketer | Organization |
| `org_id` | number | Platform → Ticketer | Legacy numeric organization ID |
| `channel` | object | Platform → Ticketer | `{ uuid, name, address }` of the contact's preferred channel |
| `flow` | object | Platform → Ticketer | `{ uuid, name }` of the flow that opened the ticket, if applicable |
| `webhook_base_url` | string | Platform → Ticketer (`POST /v1/tickets`) | Base URL the partner should use to call the webhooks in [Section 3](#3-ticketer--platform). Equivalent to `<platform>/webhooks/ticketer/{ticketer_uuid}` in the canonical contract — pre-configured partners may ignore this field |
| `source_message_external_id` | string | Platform → Ticketer (`POST /v1/tickets/{external_id}/messages`) | Original message ID on the upstream channel (e.g. WhatsApp WAMID), forwarded when available for correlation |
| `contact_uuid` | uuid | Platform → Ticketer | Redundant with `contact.uuid`, kept for compatibility |
| `priority` | enum | Platform → Ticketer | `low`, `normal`, `high`, `urgent` |

Partners may include their own keys in `metadata` on return webhooks — the platform stores them without interpreting.

---

## 11. Operational limits

| Limit | Value |
|-------|-------|
| Max body size (request) | **32 MB** |
| Max attachment size (download) | **10 MB** default; up to 100 MB for specific types |
| Timeout per request | **30 s** |
| `X-Webhook-Timestamp` validity window | **5 min** |
| Retry attempts | up to **3** with exponential backoff |

---

## 12. Versioning

The contract uses explicit versioning via URL prefix and header:

- **URL:** `/v1/tickets`, `/v1/tickets/{external_id}/messages`, etc.
- **Header:** `X-API-Version: 1`

Breaking changes only occur between major versions (`/v2/...`). Optional field additions are compatible and do not require a new version.

---

## 13. Main integration sequence

```
0. Provisioning (once)
   - Partner generates api_token, platform generates webhook_secret and ticketer_uuid

1. Platform opens ticket
   POST /v1/tickets

2. Ticketer returns the external ID
   201 Created { "external_id": "EXT-123456" }

3. (optional) Platform sends history
   POST /v1/tickets/EXT-123456/history

4. Contact sends message
   POST /v1/tickets/EXT-123456/messages

5. Agent replies in the Ticketer
   POST /webhooks/ticketer/{ticketer_uuid}/messages

6. Platform delivers the reply to the contact

7. Platform or Ticketer closes the conversation
   POST /v1/tickets/EXT-123456/close
   or
   POST /webhooks/ticketer/{ticketer_uuid}/tickets/close
```

---

## 14. Partner implementation checklist

### 14.1 Platform → Ticketer endpoints

| Item | Required |
|------|----------|
| `POST /v1/tickets` — open | Yes |
| `POST /v1/tickets/{external_id}/messages` — contact messages | Yes |
| `POST /v1/tickets/{external_id}/close` — close | Yes |
| `POST /v1/tickets/{external_id}/reopen` — reopen | Optional |
| `POST /v1/tickets/{external_id}/history` — history | Optional |
| `GET /v1/tickets/{external_id}` — lookup | Optional (roadmap) |

### 14.2 Ticketer → Platform webhooks

| Item | Required |
|------|----------|
| `POST /webhooks/ticketer/{ticketer_uuid}/messages` — agent reply | Yes |
| `POST /webhooks/ticketer/{ticketer_uuid}/tickets/close` — external close | Recommended |
| `POST /webhooks/ticketer/{ticketer_uuid}/tickets/reopen` — external reopen | Optional |

### 14.3 Operational

| Item | Required |
|------|----------|
| Bearer Token on inbound (Platform → Ticketer) | Yes |
| HMAC-SHA256 on outbound (Ticketer → Platform) | Yes (default); skippable when the platform sets `skip_webhook_hmac=true` |
| Attachment support via HTTPS URL | Recommended |
| Idempotency on open and message send | Recommended |
| Errors in JSON with clear codes | Yes |
| Respect `Retry-After` on `429`/`503` responses | Yes |
| `X-API-Version: 1` header | Yes |

---

## 15. Minimum contract for staging

For a first working version, the partner needs to implement at least:

```
POST /v1/tickets
POST /v1/tickets/{external_id}/messages
POST /v1/tickets/{external_id}/close
POST /webhooks/ticketer/{ticketer_uuid}/messages
```

Plus:

- Bearer authentication on inbound
- HMAC verification on outbound (or confirmation with the integration team that `skip_webhook_hmac=true` is active in the staging environment)
- JSON error response per [Section 9](#9-standard-error-format)

With that, the integration covers the basic cycle:

```
open conversation → forward contact messages → agent replies → close conversation
```
