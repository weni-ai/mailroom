---
description: "Task list for durable media links on Zendesk ticket opening"
---

# Tasks: Durable media links on Zendesk ticket opening

**Input**: Design documents from `/specs/002-zendesk-durable-media/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/zendesk-open-media.md, quickstart.md

**Tests**: Included. Constitution Principle X and plan.md require `Open` to be asserted through the mocked Zendesk push. Each story's tests are written first and must fail before that story's implementation task.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested as its own increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Zendesk ticketer: `services/tickets/zendesk/`
- Durable links already implemented: `core/presign/link.go`, `testsuite/presign.go`
- Changelog: `WENI-CHANGELOG.md`
- Feature docs: `specs/002-zendesk-durable-media/`

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm the existing package is green before editing it

- [X] T001 Run `go test ./services/tickets/zendesk/ -count=1 -p=1` from the repository root and confirm `services/tickets/zendesk/service_test.go` passes before any feature edit

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Put the outbound-copy call site in place without changing behavior yet

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 Add `rewriteMediaLinks(body string) string` to `services/tickets/zendesk/media_links.go`, returning `body` unchanged
- [X] T003 In `services/tickets/zendesk/service.go` `Open`, pass the original `body` to `flows.OpenTicket` and parse the plain-text or JSON push input from `rewriteMediaLinks(body)`. Constraint from data-model.md: "Only the outbound copy is rewritten."

**Checkpoint**: Existing Zendesk tests still pass. `ticket.Body()` stays the original string because `OpenTicket` does not receive the rewritten copy.

---

## Phase 3: User Story 1 - Agent can open history media from a new Zendesk ticket (Priority: P1) 🎯 MVP

**Goal**: Known-bucket file addresses in the Zendesk opening text are durable media links in the push, including when several files appear and when the storage address carries a temporary signature.

**Independent Test**: Open a Zendesk ticket whose plain opening text contains one or more `media-bucket` file URLs. The mocked `any_channel/push.json` body contains `presign.DurableURL` for each file and does not contain the raw bucket host or the temporary query.

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before T006**

- [X] T004 [P] [US1] Add `TestRewriteMediaLinks` in `services/tickets/zendesk/media_links_test.go` for one known-bucket URL, two different known-bucket URLs, the same URL twice, and a known-bucket URL whose query contains `X-Amz-Signature`. Use `testsuite.UseDurableLinks(t, "mailroom.test", "current-secret", "media-bucket")`. Expect `presign.DurableURL` for every occurrence and no query string. Constraint from data-model.md: "Known bucket and durable links configured → replace the candidate with the durable link. Drop any temporary signature." and "Every occurrence is evaluated."
- [X] T005 [P] [US1] Add `TestOpenRewritesBucketMediaLinks` in `services/tickets/zendesk/service_test.go` that calls `Open` with plain history containing `https://media-bucket.s3.sa-east-1.amazonaws.com/media/photo.jpg`, mocks `https://nyaruka.zendesk.com/api/v2/any_channel/push.json`, and asserts the logged request contains `presign.DurableURL` of that URL and does not contain `media-bucket.s3.`. Assert `ticket.Body()` is still the original plain string.

### Implementation for User Story 1

- [X] T006 [US1] Implement `rewriteMediaLinks` in `services/tickets/zendesk/media_links.go` so each literal `http://` or `https://` span is passed to `presign.DurableURL` and replaced when the result differs. Named pattern: `https?://[^\s<>"']+`. Do not log the text or the URL. Do not add an error return.

**Checkpoint**: User Story 1 is done. Plain history with known-bucket files is delivered as durable links, and the stored ticket body is unchanged.

---

## Phase 4: User Story 2 - History stays intact around the replaced links (Priority: P2)

**Goal**: Only known-bucket addresses change. Public links, links that are already durable, surrounding words, and JSON structure stay. Later Zendesk messages are not rewritten.

**Independent Test**: Open with mixed plain text and with a JSON body. The push changes only known-bucket addresses inside `message`, `description`, `subject`, and custom field values. `Forward` still matches today's file sharing.

### Tests for User Story 2 ⚠️

- [X] T007 [P] [US2] Extend `TestRewriteMediaLinks` in `services/tickets/zendesk/media_links_test.go` with a public `https://example.com/a.jpg`, an address that is already `presign.DurableURL`, and a known-bucket URL inside a sentence that ends with `. , ; : ! ? ) ] }`. Constraint from data-model.md: "Unknown host, public link, or address that is already durable → unchanged." and "Surrounding characters stay."
- [X] T008 [P] [US2] Add `TestOpenRewritesJSONFields` in `services/tickets/zendesk/service_test.go` using a JSON body with a known-bucket URL in `message`, `description`, and a `custom_fields` value, plus `https://example.com/a.jpg` in `subject`. Assert the push rewrites only the known-bucket addresses, keeps the JSON field names, and leaves `ticket.Body()` equal to the original JSON.
- [X] T009 [US2] Do not edit `Forward` or `convertAttachments` in `services/tickets/zendesk/service.go`. Re-run the existing forward case in `services/tickets/zendesk/service_test.go` and keep the `forward_message` snapshot green. Constraint from data-model.md: "file_urls | No. Open does not set this. Forward keeps today's file conversion."

### Implementation for User Story 2

- [X] T010 [US2] In `services/tickets/zendesk/media_links.go`, hold trailing `. , ; : ! ? ) ] }` off the candidate before `presign.DurableURL` and write those characters back after a replacement. If `DurableURL` returns the same string, keep the original span. Do not parse or re-serialize JSON.

**Checkpoint**: User Stories 1 and 2 both hold. Mixed history and JSON ticket fields stay readable, and `Forward` is unchanged.

---

## Phase 5: User Story 3 - Ticket opening still succeeds when a durable link cannot be issued (Priority: P3)

**Goal**: A missing durable link does not fail `Open`. The address follows the existing attachment fallback. An id-only reopen still sends no opening text.

**Independent Test**: Open with a known-bucket URL while durable links are unavailable. The ticket is created. With a signer and no media-link secret the push has a short-lived signed address. With no signer the original URL is pushed. `{"id":10}` still only updates the existing ticket.

### Tests for User Story 3 ⚠️

- [X] T011 [P] [US3] Extend `TestRewriteMediaLinks` in `services/tickets/zendesk/media_links_test.go` with an empty string, `testsuite.UseAttachmentSigner` and no link secret, and no signer at all. Constraint from data-model.md: "Known bucket and durable links unavailable, signing available → replace with the short-lived signed address (`presigned-fallback`)." and "Known bucket and no signer → leave the candidate unchanged."
- [X] T012 [P] [US3] Add `TestOpenFallbackAndReopen` in `services/tickets/zendesk/service_test.go`: signer without a media-link secret still returns a successful `Open` whose push contains `X-Amz-Algorithm`; no signer still returns a successful `Open` whose push contains the original URL; body `{"id":10}` does not push opening text. Constraint from data-model.md: "`{"id": N}` with no push of opening text still updates the existing Zendesk ticket and does not send the text."

### Implementation for User Story 3

- [X] T013 [US3] Keep `rewriteMediaLinks` in `services/tickets/zendesk/media_links.go` as a call to `presign.DurableURL` with no error return. Do not add a Zendesk-specific signer. `Open` in `services/tickets/zendesk/service.go` must still fail only when the existing push fails.

**Checkpoint**: All three stories work. Fallback and id-only reopen match the contract in `specs/002-zendesk-durable-media/contracts/zendesk-open-media.md`.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Record the user-visible fix and run the full package check

- [X] T014 [P] Add a `fix:` bullet to `WENI-CHANGELOG.md` stating that Zendesk ticket opening replaces known-bucket media addresses in the opening text with durable media links
- [X] T015 Run `go test ./services/tickets/zendesk/ -count=1 -p=1` and confirm the scenarios in `specs/002-zendesk-durable-media/quickstart.md` are covered, including the unchanged cookie fixture and the existing forward snapshot

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies
- **Foundational (Phase 2)**: Depends on Setup. Blocks every user story
- **User Story 1 (Phase 3)**: Depends on Foundational. No dependency on US2 or US3
- **User Story 2 (Phase 4)**: Depends on User Story 1 because it extends `services/tickets/zendesk/media_links.go`, `services/tickets/zendesk/media_links_test.go`, and `services/tickets/zendesk/service_test.go`
- **User Story 3 (Phase 5)**: Depends on User Story 2 because it extends those same files
- **Polish (Phase 6)**: Depends on the user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: First increment. Plain known-bucket history is enough to demo the fix
- **User Story 2 (P2)**: Independently testable with mixed and JSON bodies, but it edits the User Story 1 helper
- **User Story 3 (P3)**: Independently testable with signer and secret variants, but it edits the same helper and tests

### Within Each User Story

- Tests are written and fail before the implementation task in that phase
- `rewriteMediaLinks` stays the only rewrite. `Open` already calls it after Phase 2
- Do not start the next story's edits to the same file until the current story's tests pass

### Parallel Opportunities

- T004 and T005 can run together (different test files)
- T007 and T008 can run together after User Story 1. T009 edits `services/tickets/zendesk/service_test.go`, so it follows T008
- T011 and T012 can run together after User Story 2
- T014 can run beside the final test command only after the Go files have stopped changing
- User Stories 2 and 3 are not parallel with each other or with User Story 1: they share `media_links.go` and the two test files

---

## Parallel Example: User Story 1

```bash
# Tests first, different files:
Task: "T004 TestRewriteMediaLinks in services/tickets/zendesk/media_links_test.go"
Task: "T005 TestOpenRewritesBucketMediaLinks in services/tickets/zendesk/service_test.go"

# Then, after both tests fail for the right reason:
Task: "T006 Implement rewriteMediaLinks in services/tickets/zendesk/media_links.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 and Phase 2
2. Complete Phase 3 (tests T004 and T005, then T006)
3. Stop and run `go test ./services/tickets/zendesk/ -count=1 -p=1 -run 'TestRewriteMediaLinks|TestOpenRewritesBucketMediaLinks'`
4. Plain history with known-bucket files is the demo

### Incremental Delivery

1. Setup + Foundational → call site exists, behavior unchanged
2. User Story 1 → durable links for plain history
3. User Story 2 → surrounding text, JSON fields, and `Forward` stay intact
4. User Story 3 → missing secret or signer does not fail ticket opening
5. Polish → changelog and the quickstart command

### Parallel Team Strategy

One implementer is the fit. The rewrite lives in one function and the tests share two files, so the stories run in priority order rather than as three simultaneous edits.

---

## Notes

- [P] tasks use different files and do not wait on an unfinished task in the same phase
- [US1], [US2], and [US3] map to spec.md priorities P1, P2, and P3
- Quote the data-model constraints in the task text so the helper is not redesigned during implementation
- Do not log opening text or media URLs
- Commit after each phase, using a Conventional Commit when a commit is requested
