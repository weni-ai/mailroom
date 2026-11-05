# Tasks: Stable Media Links on Outbound Webhooks

**Input**: Design documents from `/specs/002-stable-media-links/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Included. Constitution principle X and `quickstart.md` require a flow test for the webhook exit. Write the story's test first and confirm it fails before the implementation task.

**Organization**: Tasks are grouped by user story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1)
- Include exact file paths in descriptions

## Path Conventions

Go module at the repository root. Paths come from `plan.md`: `core/goflow/`, `core/presign/link.go` (unchanged), `web/media/media.go` (unchanged), `WENI-CHANGELOG.md`.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Keep the existing link and media handler as the only stable-link implementation

- [x] T001 Confirm `core/presign/link.go` and `web/media/media.go` stay unchanged and that this feature adds no module, config key, or schema change

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Both stories call the existing stable-link function. No new helper.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T002 Confirm both later edits call `presign.DurableURL` in `core/presign/link.go` and do not add a second link function. A known-location object becomes `https://<domain>/mr/media/<token>`. The same object, an already stable link, and a short-lived signed address of that object return that same link. Any other address, including an unreadable one, is returned unchanged.

**Checkpoint**: Foundation ready — user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - External systems receive a stable file link from a webhook (Priority: P1) 🎯 MVP

**Goal**: Before a webhook is sent, each known-location file address in a body field becomes the stable link for that file. Other addresses and the stored conversation stay unchanged.

**Independent Test**: Send a webhook whose body includes one known-location file address and one unrelated address. The call that arrives contains a stable link for the known file and the unrelated address unchanged.

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T003 [US1] Update `core/goflow/signing_test.go` to the contract in `specs/002-stable-media-links/contracts/webhook-body.md`. Use `testsuite.UseDurableLinks`. A JSON field that holds a known-bucket address, including that address inside surrounding text and at any nesting depth, becomes `https://<test domain>/mr/media/<token>`. A second address on another host stays unchanged. The same object as a canonical address and as an expired signed address yields one token. An `application/x-www-form-urlencoded` field receives that stable link and the encoded body has no raw `&`. A query-string address stays unchanged. A multipart body and any body that is the file itself stay unchanged. With no signer, the body stays unchanged. A body that cannot be read still fails the call. `GetBody` returns the rewritten body.

### Implementation for User Story 1

- [x] T004 [US1] In `core/goflow/signing.go`, pass each `http` or `https` address in a JSON body and in each `application/x-www-form-urlencoded` field value through `presign.DurableURL`. On a form value, keep the original address when the result contains `&`. Do not rewrite the query string, headers, or a multipart body. Set `ContentLength` and `GetBody` from the rewritten body. Leave `signingBrainService` on `presign.SignAll`.

**Checkpoint**: User Story 1 is functional on its own. `go test ./core/goflow/ -count=1 -run 'TestSigningWebhook'` passes.

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: Record the partner-visible change and run the feature validation

- [x] T005 [P] Add a `WENI-CHANGELOG.md` entry that webhook bodies carry the stable media link for known-location files
- [x] T006 Run the commands in `specs/002-stable-media-links/quickstart.md` and confirm `core/goflow` webhook tests pass

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup — blocks the user story
- **User Story (Phase 3)**: Depends on Foundational
- **Polish (Phase 4)**: Depends on the user story being complete

### User Story Dependencies

- **User Story 1 (P1)**: Starts after Foundational. No other story.

### Within Each User Story

- The test task is written and fails before the implementation task
- T004 depends on T003
- T006 depends on T004 and T005

### Parallel Opportunities

- T005 can be written while T006 waits for the webhook tests to pass

---

## Parallel Example: User Story 1

```bash
# After T002, write the failing test, then implement the exit:
Task: "T003 Update core/goflow/signing_test.go for the webhook body contract"
Task: "T004 Rewrite webhook bodies in core/goflow/signing.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: `go test ./core/goflow/ -count=1 -run 'TestSigningWebhook'`
5. Webhook receivers can fetch known-location files from the stable link

### Incremental Delivery

1. Setup + Foundational
2. User Story 1 → webhook bodies carry stable links
3. Polish → changelog and the quickstart command

### Parallel Team Strategy

1. Complete Setup and Foundational together
2. One person takes `core/goflow/signing.go` and `core/goflow/signing_test.go`
3. Do not edit `core/presign/link.go` or `web/media/media.go`

---

## Notes

- [P] tasks = different files, no dependencies
- [US1] maps to the story in `spec.md`
- Commit after each task or logical group
- Avoid editing the brain signer or the media handler in any task
