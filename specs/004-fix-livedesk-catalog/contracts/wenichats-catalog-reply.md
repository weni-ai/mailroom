# Contract: Weni Chats catalog reply

Existing callback. No new path and no new request field.

`POST /mr/tickets/types/wenichats/event_callback/{ticketer}/{ticket}`

Catalog behavior applies when `type` is `msg.create` and `content.catalog` has a carousel flag, `send_catalog`, or at least one product. The stored row is defined in [data-model.md](../data-model.md).

## Request fields that affect the catalog row

| Field | Use |
| --- | --- |
| `content.text` | Catalog body, after the existing media-link rewrite and trim. |
| `content.catalog.header` | Title, and the body fallback when the agent text is blank. |
| `content.catalog.footer` | Copied when non-empty. |
| `content.catalog.action` | Copied when non-empty. |
| `content.catalog.products` | Copied when non-empty. |
| `content.catalog.send_catalog` | Whole-shop catalog. A row is still created when `products` is empty and the body is non-empty. |
| `content.catalog.carousel` | Stored as `product_carousel`. Does not select a different customer-facing form. |
| `content.uuid` | `chats_msg_uuid` when present. |
| `content.reply_to.external_id` | `response_to_external_id` when present. |

## Responses

| Situation | Status | Body |
| --- | --- | --- |
| One outgoing catalog message was queued | 200 | `{"status":"handled"}` |
| Agent text and title are empty or whitespace | 400 | `{"error":"catalog reply requires body text"}` |
| Body was valid and no outgoing message was queued | 400 | `{"error":"catalog reply was not created"}` |

Error strings do not include the agent text, the title, a URN, or a phone number. A 400 inserts no `msgs_msg` row for that callback.

## Stored row on 200

| Check | Expected |
| --- | --- |
| `msgs_msg.text` | Empty |
| `metadata.body` | Catalog body, at most 1024 characters |
| `metadata.header` | Trimmed title, or the key is absent |
| `metadata.header_text` | Same title when the title is present |
| Product rows, footer, button label, `send_catalog`, `product_carousel`, `chats_msg_uuid`, `response_to_external_id` | Present only when the request sent them |

Text-only and media `msg.create` events keep today's 200 and today's `text` / attachment columns. This contract does not apply to them.
