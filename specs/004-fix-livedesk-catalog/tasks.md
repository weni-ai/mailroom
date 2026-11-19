---
description: "Task list for Live Desk catalog delivery"
---

# Tasks: Live Desk catalog reaches the customer

**Input**: Design documents from `/specs/004-fix-livedesk-catalog/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/wenichats-catalog-reply.md, quickstart.md

**Tests**: Included. Constitution Principle X and plan.md require the Weni Chats callback to be asserted through `web.RunWebTests` against `msgs_msg`. Each story's tests are written first and must fail before that story's implementation task, except where a callback assertion is already satisfied by commit `c43c51f`. Helper tests must fail on the Phase 2 stub.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested as its own increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Weni Chats callback: `services/tickets/wenichats/`
- Broadcast builder guard: `core/models/msgs.go`
- Changelog: `WENI-CHANGELOG.md`
- Feature docs: `specs/004-fix-livedesk-catalog/`

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: See the current callback suite before editing it

- [X] T001 Run `go test ./services/tickets/wenichats/ -count=1 -p=1` from the repository root. Record that `create product carousel catalog message` in `services/tickets/wenichats/testdata/event_callback.json` fails because it still expects `text = 'Check out our carousel'`. Leave the text-only and media cases in that file unchanged. Do not edit code in this task.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add the single helper the callback will call, without the story rules yet

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 Add `catalogBodyMaxRunes = 1024` and `catalogReplyBody(agentText, title string) (body string, header string, err error)` to `services/tickets/wenichats/catalog_reply.go`. The stub returns `"", "", nil` and is not called yet. Do not log the agent text or the title. Leave the inline body and header block in `services/tickets/wenichats/web.go` in place until T005.

**Checkpoint**: The package still compiles. `catalogReplyBody` returns an empty body. The callback behavior is still the `c43c51f` inline block.

---

## Phase 3: User Story 1 - Cliente recebe o catálogo com o texto do agente (Priority: P1) 🎯 MVP

**Goal**: A catalog with agent text and one or more products is one outgoing row. Plain text is empty, `metadata.body` is the trimmed agent text, and a known-bucket media address in that text is already rewritten.

**Independent Test**: Post the existing carousel catalog and a one-product catalog. Each inserts one `msgs_msg` row with an empty `text` and `metadata.body` equal to the agent text. A known-bucket URL in the agent text is the durable link in `metadata.body`. Text-only and media cases in the same fixture stay as they are.

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST. T003 must fail while `catalogReplyBody` is the Phase 2 stub. T004 may already pass against the `c43c51f` inline block; do not revert that block to keep T004 red.**

- [X] T003 [P] [US1] Add `TestCatalogReplyBody` in `services/tickets/wenichats/catalog_reply_test.go`. Assert a surrounding-space agent text returns that text trimmed as the body, a title `"Weekly highlights"` returns that header, and a whitespace-only title returns an empty header. Constraint from data-model.md: "The catalog body is the trimmed agent text" and "`metadata.header` | The trimmed title when present. Key absent when the agent sent no title. Never an empty string."
- [X] T004 [P] [US1] In `services/tickets/wenichats/testdata/event_callback.json`, replace the `create product carousel catalog message` db assertion so `text` is empty, `metadata.body` is `Check out our carousel`, and `metadata.header` is `Weekly highlights`. Add a one-product case whose text is `Only this ring` and whose catalog has a title and one product. Add a case whose text is `see https://media-bucket.s3.sa-east-1.amazonaws.com/media/photo.jpg` plus a catalog product, and expect `metadata.body` to be `see $durable_photo$` with `text` empty. Do not change the text-only or media cases. Constraint from data-model.md: "`text` | Empty. Delivery must not send a plain message from this row." and "`metadata.body` | Required. 1 to 1024 Unicode characters."

### Implementation for User Story 1

- [X] T005 [US1] Implement the trim path of `catalogReplyBody` in `services/tickets/wenichats/catalog_reply.go` and switch the catalog branch in `services/tickets/wenichats/web.go` to it. Delete the inline trim and `metadata["body"]` / `metadata["header"]` block from commit `c43c51f`. Set `metadata.body` only when the returned body is non-empty. Set `metadata.header` only when the returned header is non-empty. Pass that same trimmed title as `BroadcastMessageHeader.Text` so `header_text` matches. Call `SendReplyWithCatalog` with plain text `""`. Do not truncate, fall back to the title, or return an error in this task. Constraint from data-model.md: "`metadata.header_text` | Same trimmed title when present" and "`metadata.header_type` | `text` when the title is present."

**Checkpoint**: User Story 1 is done. A catalog with agent text and products is one row, the customer-facing wording is `metadata.body`, and plain text is empty. Text and media replies are unchanged.

---

## Phase 4: User Story 2 - Título, rodapé e resposta citando outra mensagem continuam no catálogo (Priority: P2)

**Goal**: The title is on the same row when the agent sent one, and absent when not. Footer, button label, product rows, the Chats message id, and the reply-to id stay on the row. No button label is invented.

**Independent Test**: Send one catalog with a title and one without. The first row has `metadata.header` and `metadata.header_text`. The second has neither key. A catalog sent with footer, action, `content.uuid`, and `reply_to.external_id` keeps each of those values.

### Tests for User Story 2 ⚠️

- [X] T006 [US2] Add two cases to `services/tickets/wenichats/testdata/event_callback.json`. The no-title case has agent text, one product, and no `catalog.header`; assert `text` is empty, `metadata.body` is the agent text, and both `metadata ? 'header'` and `metadata ? 'header_text'` are false. The reply case sets `catalog.footer`, `catalog.action`, `content.uuid`, and `reply_to.external_id`; assert `metadata.footer`, `metadata.action`, `metadata.chats_msg_uuid`, and `metadata.response_to_external_id` equal those values, and `metadata.product_carousel` stays true when `catalog.carousel` is true. Constraint from data-model.md: "`metadata.header` | The trimmed title when present. Key absent when the agent sent no title. Never an empty string."

### Implementation for User Story 2

- [X] T007 [US2] In `services/tickets/wenichats/web.go`, keep `chats_msg_uuid` and `response_to_external_id` on `extraMetadata` before `catalogReplyBody`, and keep passing `footer` and the `BroadcastCatalogMessage` from `BroadcastParts` into `SendReplyWithCatalog`. Do not set `metadata.header` when the helper returns an empty header. Do not invent `metadata.action`. Do not stop writing `product_carousel`. Change code only where T006 shows one of those fields missing.

**Checkpoint**: User Stories 1 and 2 both hold. Title, footer, button label, and reply linkage match what the agent sent, and a missing title does not block delivery.

---

## Phase 5: User Story 3 - Texto longo ou texto vazio não gera catálogo recusado (Priority: P3)

**Goal**: A blank agent text uses the title as the body. Both blank is a 400 and no row. Text longer than 1024 characters is stored as the first 1024 characters on the same single row. A whole-shop catalog with no product rows is still queued. A catalog that matches no sendable contact is a 400 and no row.

**Independent Test**: Post a catalog whose text is only spaces and whose title is set, a catalog with neither, a catalog whose text is longer than 1024 characters, a `send_catalog` payload with no products, and a catalog for a blocked contact. The first, third, and fourth return 200 and one row. The second and fifth return 400 and insert nothing.

### Tests for User Story 3 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before T011, T012, and T013**

- [X] T008 [P] [US3] Extend `TestCatalogReplyBody` in `services/tickets/wenichats/catalog_reply_test.go`. Whitespace-only agent text and title `"Gold"` returns body and header `"Gold"`. Whitespace-only agent text and whitespace-only title returns an error, and the error string contains neither input. A 1025-character body whose last character is `é` returns the first 1024 characters, with no ellipsis and with the header unchanged. Constraint from data-model.md: "The catalog body is the trimmed agent text, or the trimmed title when the agent text is empty. Both empty is not a stored message." and "Agent text longer than 1024 characters | The overflow is not stored and is not a second row."
- [X] T009 [P] [US3] Add four cases to `services/tickets/wenichats/testdata/event_callback.json`. Spaces-only text plus title `Gold` and one product: 200, `text` empty, `metadata.body` and `metadata.header` are `Gold`. Text and title both whitespace: 400 `{"error":"catalog reply requires body text"}` and zero rows for that `content.uuid`. Text of 1025 characters plus one product: 200, one row for that uuid, `text` empty, `char_length(metadata.body) = 1024`. `send_catalog: true` with text `Full shop` and no `products`: 200, `text` empty, `metadata.body` is `Full shop`, `metadata.send_catalog` is true, and `metadata ? 'products'` is false. Contract strings are exact. Constraint from data-model.md: "Agent text and title both empty or whitespace | No row. Callback 400."
- [X] T010 [P] [US3] Add `TestCatalogReplyBlockedContact` in `services/tickets/wenichats/web_test.go` and `services/tickets/wenichats/testdata/event_callback_blocked.json`. Set `contacts_contact.status` to `B` for Cathy, insert an open Weni Chats ticket the same way `TestEventCallback` does, and post a catalog with agent text and one product. Expect 400 `{"error":"catalog reply was not created"}` and zero rows for that `content.uuid`. Defer restoring status `A`. Constraint from data-model.md: "Catalog callback that matches no sendable destination | No row. Callback 400."

### Implementation for User Story 3

- [X] T011 [US3] Finish `catalogReplyBody` in `services/tickets/wenichats/catalog_reply.go`. After trim, use the title as the body when the agent text is empty. Return an error when both are empty; the error must not include either string. When the body has more than `catalogBodyMaxRunes` Unicode characters, keep the first 1024 runes and do not add an ellipsis. Leave the header as the trimmed title. Do not log the text or the title.
- [X] T012 [P] [US3] In `core/models/msgs.go` `CreateBroadcastMessages`, skip the contact only when plain text, attachments, product rows, and `send_catalog` are all absent. Do not edit `newOutgoingMsgCatalog` or `newOutgoingMsgWpp`. Constraint from data-model.md: "Flow catalog | Unchanged row shape from the flow builders."
- [X] T013 [US3] In the catalog branch of `services/tickets/wenichats/web.go`, when `catalogReplyBody` returns an error, respond 400 with value `errors.New("catalog reply requires body text")` and a nil hard error, and do not call `SendReplyWithCatalog`. When `SendReplyWithCatalog` returns a nil message and a nil error, respond 400 with value `errors.New("catalog reply was not created")`. Do not include the agent text, the title, a URN, or a phone number in either error. Leave the text-only and media branches on their current status codes. Constraint from data-model.md: "Text-only or media reply | Unchanged row shape. Plain text stays in `text`."

**Checkpoint**: All three stories hold. Oversized, blank, whole-shop, and undeliverable catalog replies follow the contract, and flow catalog builders are untouched.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Record the user-visible fix and run the quickstart

- [X] T014 [P] Add a `fix:` bullet as the first item of the top version section in `WENI-CHANGELOG.md` describing that a Live Desk catalog reply is delivered as one catalog message whose body is the agent text. Do not edit `CHANGELOG.md`.
- [X] T015 Run the quickstart commands from `specs/004-fix-livedesk-catalog/quickstart.md`: `go test ./services/tickets/wenichats/ -count=1 -p=1` and `go test ./core/tasks/msgs/ -count=1 -p=1 -run TestBroadcastWithHeaderFooterAndCatalog`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies
- **Foundational (Phase 2)**: Depends on Setup. Blocks every user story
- **User Stories (Phase 3+)**: Depend on Foundational. Run in priority order because they share `web.go`, `catalog_reply.go`, and `event_callback.json`
- **Polish (Phase 6)**: Depends on the desired stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Starts after Phase 2. No dependency on US2 or US3
- **User Story 2 (P2)**: Starts after US1. It extends the same callback row and must not empty `metadata.body`
- **User Story 3 (P3)**: Starts after US1. It extends `catalogReplyBody` and the callback errors. It does not require the US2 fixture cases, but it edits `event_callback.json` after those cases exist

### Within Each User Story

- Tests are written first
- `catalogReplyBody` is updated before `web.go` starts returning its error
- The `send_catalog` guard in `core/models/msgs.go` does not wait on the helper
- A story is green before the next story edits the same files

### Parallel Opportunities

- T003 and T004 can run together (helper test file and callback fixture)
- T008, T009, and T010 can run together (three test files)
- T012 can run beside T011 (guard versus helper)
- T013 waits for T011 because it maps the helper error
- T014 can be written while T015 is not yet running
- US2 and US3 are not parallel with US1 or with each other: they share `web.go` and `event_callback.json`

---

## Parallel Example: User Story 3

```bash
# Tests first, different files:
Task: "T008 Extend TestCatalogReplyBody in services/tickets/wenichats/catalog_reply_test.go"
Task: "T009 Add the four edge cases to services/tickets/wenichats/testdata/event_callback.json"
Task: "T010 Add TestCatalogReplyBlockedContact in services/tickets/wenichats/web_test.go"

# Then the helper and the broadcast guard together:
Task: "T011 Finish catalogReplyBody in services/tickets/wenichats/catalog_reply.go"
Task: "T012 Update the empty-content guard in core/models/msgs.go"

# Then the callback status mapping:
Task: "T013 Map helper and nil-message failures in services/tickets/wenichats/web.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 and Phase 2
2. Complete Phase 3 (T003 and T004, then T005)
3. Stop and run `go test ./services/tickets/wenichats/ -count=1 -p=1 -run 'TestEventCallback|TestCatalogReplyBody'`
4. The reported catalog, one product, and a media link in the agent text are the demo

### Incremental Delivery

1. Setup + Foundational → helper exists and still returns an empty body
2. User Story 1 → one catalog row with the agent text as the body and empty plain text
3. User Story 2 → title, footer, button label, and reply linkage match the callback
4. User Story 3 → blank text, 1024-character cut, whole-shop catalog, and blocked contact
5. Polish → changelog and both quickstart commands

### Parallel Team Strategy

One implementer is the fit. The callback, the helper, and the shared fixture are the same files across the three stories, so the stories run in priority order. Within User Story 3, the helper tests, the fixture, and the blocked-contact test can be written together, and the broadcast guard can be edited while the helper is finished.

---

## Notes

- [P] tasks use different files and do not wait on an unfinished task in the same phase
- [US1], [US2], and [US3] map to spec.md priorities P1, P2, and P3
- Quote the data-model constraints in the task text so the body rule is not redesigned during implementation
- Do not log agent text, titles, URNs, or phone numbers
- Commit after each phase, using a Conventional Commit when a commit is requested
