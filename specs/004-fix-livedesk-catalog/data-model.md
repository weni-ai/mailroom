# Data model: Live Desk catalog reaches the customer

No new table, column, or configuration key. A catalog reply during human attendance updates one existing outgoing `msgs_msg` row.

## Catalog reply from attendance

The `msg.create` content that carries a catalog. `HasCatalog` is true when `catalog.carousel`, `catalog.send_catalog`, or `catalog.products` is set.

| Field | Meaning |
| --- | --- |
| Agent text | `content.text` after the media-link rewrite already applied to ticket replies. Trimmed. |
| Title | `catalog.header`, trimmed. Whitespace-only is absent. |
| Footer | `catalog.footer` when non-empty. |
| Button label | `catalog.action` when the caller sent one. Not invented. |
| Product rows | `catalog.products` when the agent picked products. |
| Whole-shop catalog | `catalog.send_catalog` true. Product rows may be empty. |
| Carousel flag | `catalog.carousel`. Stored as today. Does not change the customer-facing list. |
| Origin | `content.uuid`, stored as `chats_msg_uuid` when present. |
| Reply target | `content.reply_to.external_id`, stored as `response_to_external_id` when present. |

**Rule**: The catalog body is the trimmed agent text, or the trimmed title when the agent text is empty. Both empty is not a stored message.

## Outgoing catalog message

One `msgs_msg` row queued for the contact.

| Field | Rule |
| --- | --- |
| `text` | Empty. Delivery must not send a plain message from this row. |
| `metadata.body` | Required. 1 to 1024 Unicode characters. The catalog body above, cut to the first 1024 characters when longer. |
| `metadata.header` | The trimmed title when present. Key absent when the agent sent no title. Never an empty string. |
| `metadata.header_text` | Same trimmed title when present, written by the existing broadcast header. |
| `metadata.header_type` | `text` when the title is present. |
| `metadata.footer` | Footer when present. |
| `metadata.action` | Button label when present. |
| `metadata.products` | Product rows when present. |
| `metadata.send_catalog` | True for a whole-shop catalog. |
| `metadata.product_carousel` | True when the callback set carousel. |
| `metadata.chats_msg_uuid` | Origin id when present. |
| `metadata.response_to_external_id` | Reply target when present. |

There is no new status. The row is either queued, or it is not inserted.

## Not stored

| Input | Outcome |
| --- | --- |
| Agent text and title both empty or whitespace | No row. Callback 400. |
| Agent text longer than 1024 characters | The overflow is not stored and is not a second row. |
| Catalog callback that matches no sendable destination | No row. Callback 400. |
| Text-only or media reply | Unchanged row shape. Plain text stays in `text`. |
| Flow catalog | Unchanged row shape from the flow builders. |
