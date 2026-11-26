# Implementation Plan: Live Desk catalog reaches the customer

**Branch**: `004-fix-livedesk-catalog` | **Date**: 2026-11-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-fix-livedesk-catalog/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

A Live Desk catalog reply must be stored as one outgoing message whose plain text is empty and whose catalog body and title are the fields delivery already reads for a Manager catalog. Commit `c43c51f` starts that move. This plan finishes it: trim the agent text, fall back to the title when the text is blank, reject the callback when no body can be formed, cut the body at 1024 characters, and keep a whole-shop catalog (`send_catalog` with no product rows) from being discarded. Text replies, media replies, and flow catalogs stay on their current paths.

## Technical Context

**Language/Version**: Go 1.23.5 (CI `go-version`)

**Primary Dependencies**: Weni Chats `msg.create` callback in `services/tickets/wenichats`; `tickets.SendReplyWithCatalog`; `models.CreateBroadcastMessages`. Delivery reads `metadata.body` and `metadata.header` and sends `msgs_msg.text` as a separate plain message. No Courier change.

**Storage**: Existing `msgs_msg` row. No schema change and no new `MAILROOM_*` key. Plain text column empty; catalog wording in `metadata`.

**Testing**: `go test ./services/tickets/wenichats/ -count=1 -p=1` and `go test ./core/tasks/msgs/ -count=1 -p=1 -run TestBroadcastWithHeaderFooterAndCatalog`. Web cases use `web.RunWebTests` against `mailroom_test`. No live Meta or Chats calls.

**Target Platform**: Mailroom worker (Linux)

**Project Type**: background worker, ticketer callback

**Performance Goals**: No added outbound call. Body trimming and the 1024-character cut are one pass over the agent text already loaded with the callback.

**Constraints**: Do not log message text, titles, URNs, or phone numbers. Do not invent a button label. Do not render a carousel. Do not change text or media replies. Empty body is a 400, not a 200 with a rejected catalog.

**Scale/Scope**: Peak is the current rate of catalog replies during human attendance. One existing ticket-reply path; the payload of that reply changes. No new queue.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Result |
| --- | --- | --- |
| I. Version Control and Review | Change reaches `main` only by PR with review and green CI | Pass. Plan does not bypass review. |
| II. Security and Secrets | No new secret or credential | Pass. No new config. Tests keep using fixture tickets. |
| III. Never Trust the Client | Catalog callback payload is validated before a message is queued | Pass. Empty body is rejected. Length is capped. The callback already requires the ticketer and ticket UUIDs. |
| IV. Observability | Do not log message text, titles, URNs, or phone numbers | Pass. New errors name the failure, not the wording. |
| V. Diagnosable Errors | A refused catalog reply is visible to the caller without personal data | Pass. Stable 400 messages. Ticket UUID is already in the request path. |
| VI. Versioned Contracts | `msg.create` request shape unchanged; stored catalog fields match what delivery already reads | Pass. See [contracts/wenichats-catalog-reply.md](./contracts/wenichats-catalog-reply.md). `header_text` stays for readers that already use it. |
| VII. Fail Gracefully | A catalog that cannot be sent does not report success | Pass. Callback returns 400 when no body exists or no outgoing message was created. |
| VIII. Bounded Retry | No new REST call | Pass. The channel rejection this fixes is a 4xx caused by an empty body, which must not be retried as-is. |
| IX. Scalability and Peak Load | Peak declared; no new in-process state | Pass. Peak = current human-attendance catalog replies. |
| X. Tests Exercise Flows | Callback fixture covers success and the empty-body failure; broadcast catalog counts stay stable | Pass. See [quickstart.md](./quickstart.md). |
| XI. Explicit Over Clever | Named 1024-character limit; body rules in one helper | Pass. |
| XII. Contained Changes | Weni Chats catalog callback, the broadcast empty-content guard for `send_catalog`, tests, changelog | Pass. Flow catalog builders are not edited. |
| XIII. Specification Traceability | Engineering spec derives from a tagged product spec | Justified exception. See Complexity Tracking. |
| XIV. No Silent Divergence | Nothing inherited to contradict | Pass. The spec records that it is the originating description. |
| XV. Commit Messages | Follow-up commits use Conventional Commits | Pass for this design. No commit in this command. |
| XVI. Changelog | User-visible reply change recorded in `WENI-CHANGELOG.md` | Pass. Implementation adds a `fix:` bullet on the release that ships it. |

**Post-design re-check**: Same results. Phase 1 adds no endpoint, schema, or config key. The `msg.create` JSON shape is unchanged; only the outgoing `msgs_msg` text and metadata for a catalog reply change.

## Project Structure

### Documentation (this feature)

```text
specs/004-fix-livedesk-catalog/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── wenichats-catalog-reply.md
└── tasks.md             # /speckit-tasks, not created here
```

### Source Code (repository root)

```text
services/tickets/wenichats/web.go                         # catalog branch calls the helper and rejects an undelivered reply
services/tickets/wenichats/catalog_reply.go               # trim, title fallback, 1024-character cut
services/tickets/wenichats/catalog_reply_test.go          # helper edge cases
services/tickets/wenichats/testdata/event_callback.json   # callback outcomes
services/tickets/wenichats/web_test.go                    # unchanged runner; add a no-destination case if the shared fixture cannot host it
core/models/msgs.go                                       # keep a whole-shop catalog when plain text and product rows are empty
WENI-CHANGELOG.md
```

**Structure Decision**: The body rules live next to the Weni Chats callback, because only that path must clear plain text and fill the catalog body. `CreateBroadcastMessages` changes by one guard condition so `send_catalog` counts as content. Flow catalog builders (`newOutgoingMsgCatalog`, `newOutgoingMsgWpp`) stay untouched.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
| --- | --- | --- |
| XIII. No tagged product spec | The constitution's product-spec repository is still an open TODO, and this defect was specified from the Live Desk catalog investigation. `spec.md` records the absence in Inheritance from Product Spec. | Blocking the design on a product-spec tag would leave customers receiving the agent text without the products. There is no inherited "what" to restate or silently change. |

## Design notes

Decisions are in [research.md](./research.md). The stored message is in [data-model.md](./data-model.md). The callback outcomes are in [contracts/wenichats-catalog-reply.md](./contracts/wenichats-catalog-reply.md).

`handleEventCallback` already rewrites media links in `content.text` before the catalog branch. The helper takes that string and the catalog title. On success it sets `metadata.body`, sets `metadata.header` only when the title is non-empty, and `SendReplyWithCatalog` is called with an empty plain text. `header_text` / `header_type` continue to be written by `CreateBroadcastMessages` from the same trimmed title. A nil message or an empty body returns 400 and does not respond `handled`.
