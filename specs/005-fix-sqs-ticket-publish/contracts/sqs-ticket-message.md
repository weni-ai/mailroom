# Contrato: mensagem SQS de criação de ticket (INALTERADO)

**Feature**: `005-fix-sqs-ticket-publish` | **Requisito**: RF6 / FR-008

Este documento registra o contrato **externo** que esta feature não pode mudar. Consumidores dependem dele. Qualquer alteração aqui é uma mudança incompatível (Principle VI) e está fora de escopo.

## Corpo (`MessageBody`)

JSON produzido por `TicketSQSMessage.Marshal()` (`core/tasks/sqs/publishers/tickets.go`):

```json
{
  "ticket_uuid": "d2f852ec-7b4e-457f-ae7f-f8b243c49ff5",
  "contact_urn": "whatsapp:5511999999999",
  "project_uuid": "a3f852ec-7b4e-457f-ae7f-f8b243c49ff6",
  "channel_uuid": "b4f852ec-7b4e-457f-ae7f-f8b243c49ff7",
  "created_on": "2024-10-30T12:00:00Z"
}
```

| Campo | Tipo | Regra |
|-------|------|-------|
| `ticket_uuid` | UUID | Identificador do ticket. Chave de deduplicação do consumidor. |
| `contact_urn` | string | URN do contato. **Presente no corpo; nunca nos logs.** |
| `project_uuid` | UUID | Projeto da org. |
| `channel_uuid` | UUID | Canal do contato preferido. |
| `created_on` | RFC 3339 | Instante de criação do ticket (evento). |

## Atributos de mensagem

| Atributo | Tipo de dado | Valor | Obrigatório |
|----------|--------------|-------|-------------|
| `ContentType` | String | `application/json` | sim (enviado por `publishWithAttributes`) |
| `MessageGroupId` | String | `<project_uuid>:<channel_uuid>:<contact_urn>` | somente em fila FIFO |
| `CorrelationID` | String | UUID v4 gerado no enfileiramento | sim |

- `MessageDeduplicationId` (somente FIFO) = SHA-256 hex do corpo. Não muda.
- `MessageGroupId` > 128 caracteres é substituído pelo SHA-256 hex, como hoje (`fitMessageGroupID`).

## Garantias de entrega

- **At-least-once**. Reenvios (retry, reclaim, recuperação após queda) enviam **o mesmo corpo** e os **mesmos atributos**, inclusive `CorrelationID` do enfileiramento original.
- O consumidor deduplica por `ticket_uuid`.
- Em fila FIFO, reenvios dentro de 5 minutos são deduplicados pelo SQS via `MessageDeduplicationId`.

## Compatibilidade

- O envelope interno do Redis (`PublishTask`) ganha o campo opcional `meta`. Ele **não** aparece no corpo SQS.
- Teste de contrato obrigatório: capturar a requisição `SendMessage` contra o `httptest` e comparar com um fixture JSON de corpo e atributos gerado antes da mudança. Ver [quickstart.md](../quickstart.md), cenário Q-03.
