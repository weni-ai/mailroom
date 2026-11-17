# Quickstart: Live Desk catalog reaches the customer

Validates [wenichats-catalog-reply.md](./contracts/wenichats-catalog-reply.md) against the ticket callback and the stored `msgs_msg` row. No live Chats, Courier, or Meta calls.

## Prerequisites

- Go 1.23.5
- Local `mailroom_test` Postgres, Redis, and RabbitMQ, the same dependencies `services/tickets/wenichats` tests already use via `testsuite.Get()`

## Run

From the repository root:

```sh
go test ./services/tickets/wenichats/ -count=1 -p=1
go test ./core/tasks/msgs/ -count=1 -p=1 -run TestBroadcastWithHeaderFooterAndCatalog
```

The Weni Chats package test is the flow check: `TestEventCallback` posts `msg.create` and asserts the row. `TestBroadcastWithHeaderFooterAndCatalog` checks that broadcast catalogs which already have plain text or product rows still produce the same message counts.

Targeted runs once the tests exist:

```sh
go test ./services/tickets/wenichats/ -count=1 -p=1 -run 'TestEventCallback|TestCatalogReplyBody'
```

## Scenarios

The shared fixture is `services/tickets/wenichats/testdata/event_callback.json`. Catalog assertions key off `chats_msg_uuid` or the body text so they do not depend on unrelated rows. Media-link cases use `testsuite.UseDurableLinks` the way the existing rewrite case does.

| Scenario | Callback | Expected |
| --- | --- | --- |
| Reported catalog | Text, title, footer, button label, carousel, more than one product id, `content.uuid` | 200 `handled`. One row. `text` empty. `metadata.body` is the agent text. `metadata.header` and `metadata.header_text` are the title. Footer, action, products, `product_carousel`, and `chats_msg_uuid` kept. |
| Single product | Text and one product, no title | 200. `metadata.header` absent. `metadata.body` is the agent text. `text` empty. |
| Media link in the wording | Agent text contains a known-bucket URL | 200. `metadata.body` contains the durable link. `text` empty. |
| Blank text, title present | Text is spaces, title and products set | 200. `metadata.body` and `metadata.header` are the trimmed title. `text` empty. |
| Nothing to show | Text and title empty or whitespace, catalog still set | 400 `catalog reply requires body text`. No row for that `content.uuid`. |
| Long wording | Text longer than 1024 characters, products set | 200. `char_length(metadata.body)` is 1024. Body is the first 1024 characters. `text` empty. No second row with the overflow. |
| Whole shop | `send_catalog` true, no product rows, text set | 200. One row. `text` empty. `metadata.body` set. `metadata.send_catalog` true. |
| Reply to a previous message | Catalog plus `reply_to.external_id` | 200. `metadata.response_to_external_id` equals that id. |
| Text and media | Existing non-catalog cases in the same fixture | Unchanged status, text, and attachments. |
| No destination | Catalog with a valid body for a contact that cannot be sent to | 400 `catalog reply was not created`. No new outgoing row. Separate test if the shared fixture cannot change the contact without breaking earlier cases. |
| Broadcast catalogs | `TestBroadcastWithHeaderFooterAndCatalog` | Existing counts for text, `header_text`, products, and carousel stay the same. |

Helper checks in `TestCatalogReplyBody`: trim, title fallback, whitespace-only title, both empty returns an error, and a 1025-character string (including a non-ASCII character) returns 1024 characters with no ellipsis.
