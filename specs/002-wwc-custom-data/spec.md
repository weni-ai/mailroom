# Engineering Specification: custom_data on send_whatsapp_broadcast (Mailroom slice)

**Repository**: weni-ai/mailroom — backend

**Feature Branch**: `002-wwc-custom-data`

**Created**: 2026-11-04

**Status**: Draft

**Suggested cycle**: SDD Lite (`specify` → `implement` from this `spec.md`). Do **not** run `/speckit-plan` or `/speckit-tasks` for this feature unless this spec is amended to promote to Full. The change is additive, single-repo, and bounded to one existing task path; architecture is already binding.

## Inheritance from Product Spec

## Origin Without Product Spec

- Product Spec: none — engineering origin. Architecture replaces product for the inherited "what" (constitution Principle XIII format kept; fields filled from architecture Origin).
- Architecture doc: https://github.com/weni-ai/vtex-cx-experience-specs/blob/006-wwc-custom-data/arch-v1/specs/006-wwc-custom-data/architecture.md — pinned tag `006-wwc-custom-data/arch-v1` (immutable commit `1ebcd6346d46d269f6421d7f3c10344ebdda394f`; same pin as Flows `specs/012-wwc-custom-data`)
- Contracts (same pin, directory `specs/006-wwc-custom-data/contracts/`):
  - Consumer: `events/send-whatsapp-broadcast-task.schema.json` — decode `msg.custom_data` from the batch task
  - Producer: `events/courier-msg-metadata.schema.json` — `custom_data` on `msgs_msg.metadata` / Courier queue JSON
  - Shared value shape: `custom-data.schema.json` (limits enforced only by Flows, AD-002)
- Problem: opaque integrator JSON on WhatsApp broadcasts is discarded when Mailroom unmarshals `send_whatsapp_broadcast` into a typed struct that has no `custom_data` field. Downstream Courier/socket/service cannot deliver what Flows already stored.
- Acceptance criteria (Mailroom slice; end-to-end `customData` on webchat is owned by Courier/socket/service):
  - **AC-M1**: Task `send_whatsapp_broadcast` with a valid `msg.custom_data` object → each created `Msg` has `metadata.custom_data` equal to the task value (same JSON object), for WWC and non-WWC channels.
  - **AC-M2**: Task without `custom_data` or with `{}` → metadata has no present `custom_data` (same as absence).
  - **AC-M3**: Task with `custom_data` that is `null`, string, array, or number → field dropped, broadcast succeeds, messages created as today except no `custom_data` key.
  - **AC-M4**: Strings inside `custom_data` that look like `@contact.name` are stored verbatim (no Excellent evaluation).
  - **AC-M5**: Resend/replay paths that read metadata from the DB still carry `custom_data` when it was persisted.
- Inherited binding decisions (architecture AD-001–AD-007; this slice implements Mailroom halves of AD-001, AD-003, AD-004, and defensive type drop of AD-002):
  - AD-001: task field `msg.custom_data` → message `metadata.custom_data`.
  - AD-002: Mailroom MUST NOT re-validate byte size, depth, or forbidden keys; only a defensive type check. Invalid type → drop field, do not fail the task.
  - AD-003: copy for any resolved channel type; do not filter by WWC.
  - AD-004: do **not** evaluate `@contact.*` / `@fields.*` inside `custom_data`; same value for all recipients.
  - AD-005–AD-007: Courier / socket / service — out of scope here.
- Scope of this spec: the Mailroom row of the architecture Impact Map only.
- Divergences: none (no Product Spec; architecture is the origin of the "what").

## Clarifications

None. Defensive decoding, channel passthrough, verbatim copy, omit-when-empty, and deploy order are binding in the pinned architecture and event schemas.

## Scope in This Repository

**This repository implements**:

- Decode optional `msg.custom_data` from the `send_whatsapp_broadcast` task (`json.Unmarshal` → `WppBroadcast` / `WppBroadcastMessage`) after Flows validation (AD-002).
- Copy the value verbatim into `extraMetadata` for every message created in `CreateWppBroadcastMessages` (`core/models/msgs.go`), so it is merged into `msgs_msg.metadata` (same path as `ig_comment_id`, `queue`, etc.).
- Carry the same decoded value on `WppBroadcastBatch.Msg` via existing `CreateBatch` (copies `Msg`).
- Accept tasks with `custom_data` for any resolved channel type (AD-003).
- Defensive decoding: if `custom_data` is present but not a JSON object, DROP the field and process the broadcast normally; MUST NOT fail the task or batch because of `custom_data`.
- Omit `custom_data` from message metadata when absent or equivalent to `{}`.
- Tests mapping AC-M1–AC-M5 in this repo (Principle X).
- A `WENI-CHANGELOG.md` entry (contract-affecting queue/metadata field).

**Out of scope here**:

- Flows validation (`WhatsappBroadcastWriteSerializer`) and any change to `temba/mailroom/queue.py`.
- Courier handlers/weniwebchat (AD-005).
- `weni-webchat-socket`, `webchat-service`.
- `send_msg` / other send APIs / goflow.
- Re-validation of 16384 bytes, 32 depth, or `$` / `.` keys.
- Schema migrations on PostgreSQL.

## Current State

- `core/tasks/msgs/send_wpp_broadcast.go`: `handleSendWppBroadcast` does `json.Unmarshal(task.Task, broadcast)` into `*models.WppBroadcast`. Unknown JSON keys on nested structs are dropped. `CreateWppBroadcastBatches` fans out contact IDs and queues `send_whatsapp_broadcast_batch` with `CreateBatch`, which copies `Msg` unchanged. `handleSendWppBroadcastBatch` unmarshals `WppBroadcastBatch` the same way and calls `SendWppBroadcastBatch` → `CreateWppBroadcastMessages`.
- `core/models/msgs.go`, `WppBroadcastMessage` (~1776): typed fields (`text`, `template`, `ig_comment_id`, …). **No `custom_data` field** — that is why current Mailroom silently drops the key (architecture Compatibility).
- `CreateWppBroadcastMessages`: per contact, evaluates Excellent on `text` / header / footer / template / `ig_comment_id`; builds `extraMetadata` (~2174+) for `queue`, `product_carousel`, `ig_comment_id`, `ig_response_type`; then `NewOutgoingWppBroadcastMsg(..., extraMetadata)`.
- `newOutgoingMsgWpp` (~792): if `extraMetadata != nil`, merges into `m.Metadata` (`null.Map`) persisted as `msgs_msg.metadata` and queued for Courier.
- `ResendMessages` (~2297): reselects channel, sets `IsResend`, updates status; **does not rewrite metadata**. Replay therefore already carries whatever was stored.
- Existing tests: `core/models/msgs_test.go` (`TestWppBroadcastIGCommentMetadata`, carousel tests), `core/tasks/msgs/send_wpp_broadcast_test.go` (`TestWppBroadcastTask`). Neither asserts `custom_data`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Persist task custom_data on every outgoing message (Priority: P1)

Flows has already queued `send_whatsapp_broadcast` with a valid `msg.custom_data` object. Mailroom creates one outgoing message per recipient and stores that object on each message’s metadata so Courier can read it later.

**Why this priority**: Without this hop, the architecture end-to-end AC never leaves Postgres `Broadcast.metadata`.

**Independent Test**: Unmarshal a task (or `NewWppBroadcast` with the object set) and run `CreateWppBroadcastMessages` / `SendWppBroadcastBatch`; assert each `Msg.Metadata()["custom_data"]` equals the task object for a WWC-capable URN/channel and a non-WWC channel.

**Acceptance Scenarios**:

1. **Given** a `send_whatsapp_broadcast` task whose `msg.custom_data` is the contract example object, **When** Mailroom creates messages, **Then** every created message’s metadata contains `custom_data` equal to that object.
2. **Given** the same task with a resolved non-WWC channel, **When** messages are created, **Then** metadata still contains the same `custom_data` (AD-003).
3. **Given** multiple recipients in one batch, **When** messages are created, **Then** all recipients receive the same `custom_data` object.

---

### User Story 2 - Absence and empty object stay compatible (Priority: P1)

Callers and old Flows that omit `custom_data`, or send `{}`, keep today’s metadata (no new key).

**Why this priority**: MINOR additive contract; architecture Compatibility window.

**Independent Test**: Run an existing broadcast fixture without the field; run one with `{}`; assert metadata has no `custom_data` key.

**Acceptance Scenarios**:

1. **Given** a task with no `msg.custom_data`, **When** messages are created, **Then** metadata has no `custom_data` key and other fields (`queue`, text, etc.) match today.
2. **Given** `msg.custom_data` is `{}`, **When** messages are created, **Then** metadata has no present `custom_data` (equivalent to absence).

---

### User Story 3 - Malformed custom_data never fails the broadcast (Priority: P1)

A task from an old Flows (or any producer) may put a non-object under `custom_data`. Mailroom drops the field and still sends.

**Why this priority**: Contract MUST: do not fail the task or batch because of `custom_data`. Principle VII.

**Independent Test**: Unmarshal JSON with `custom_data` as `null`, string, array, and number; run create; assert no error and no `custom_data` on metadata.

**Acceptance Scenarios**:

1. **Given** `custom_data` is `null`, a string, an array, or a number, **When** the task is processed, **Then** the task succeeds and created messages have no `custom_data` key.
2. **Given** such a payload, **When** other `msg` fields are valid, **Then** messages are created as they would be without `custom_data`.

---

### User Story 4 - Expression-looking strings stay literal (Priority: P2)

An integrator stores `"@contact.name"` (or similar) inside `custom_data`. Mailroom must not run Excellent on that tree.

**Why this priority**: AD-004; per-recipient evaluation is explicitly rejected.

**Independent Test**: Put `@contact.name` in a nested string; create messages for a named contact; assert the stored string is still `@contact.name`.

**Acceptance Scenarios**:

1. **Given** `custom_data` contains a string `@contact.name` (and/or `@fields.*`), **When** messages are created for a contact with a real name, **Then** metadata stores the original string, not the evaluated name.

---

### User Story 5 - Resend keeps persisted custom_data (Priority: P2)

Operators resend failed outgoing messages. Metadata already on the row must still include `custom_data`.

**Why this priority**: Architecture courier-msg-metadata contract: resends read metadata from the database.

**Independent Test**: Persist a message with `custom_data`, call `ResendMessages` (or the production resend entry that loads msgs from DB), assert metadata still has the object and the queued payload still includes it if the test inspects Courier JSON.

**Acceptance Scenarios**:

1. **Given** a stored outgoing message whose `msgs_msg.metadata` has `custom_data`, **When** Mailroom resends that message, **Then** the resent path still carries that `custom_data`.

---

### Edge Cases

- Entire task JSON fails to unmarshal for reasons other than `custom_data` (malformed JSON, wrong types on required fields): existing error wrapping still fails the task; this feature does not change that.
- `custom_data` present as a JSON object with extra nested keys: copy as-is; do not strip keys that Flows would have rejected (Mailroom is not the validator). In the intended deploy order, Flows step 3 already rejected those.
- `extraMetadata` already has `queue` / `ig_comment_id`: merge MUST NOT drop those keys; `custom_data` is an additional key.
- Empty `extraMetadata` besides omitted `custom_data`: do not invent a metadata document solely for an absent field (today `extraMetadata` may still be a non-nil empty map passed to `NewOutgoingWppBroadcastMsg`; do not add `custom_data` to it when dropping).
- Batch JSON (`send_whatsapp_broadcast_batch`) MUST decode `msg.custom_data` the same way as the parent task, because batches are independently unmarshaled.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Mailroom MUST decode optional `msg.custom_data` on `send_whatsapp_broadcast` and `send_whatsapp_broadcast_batch` into `WppBroadcastMessage`.
- **FR-002**: When `custom_data` is a JSON object with at least one member, Mailroom MUST copy that object verbatim into `extraMetadata["custom_data"]` for every message in `CreateWppBroadcastMessages`.
- **FR-003**: Mailroom MUST NOT evaluate Excellent (or any contact/field substitution) on `custom_data` or its nested strings (AD-004).
- **FR-004**: Mailroom MUST NOT skip the copy based on resolved channel type (AD-003).
- **FR-005**: Mailroom MUST NOT re-validate serialized byte size, nesting depth, or forbidden key characters (AD-002).
- **FR-006**: If `custom_data` is present and is not a JSON object (`null`, string, array, number, boolean), Mailroom MUST drop the field and MUST still create messages; the task MUST NOT error because of that field.
- **FR-007**: If `custom_data` is absent or is `{}`, Mailroom MUST omit the key from message metadata (equivalent to absence).
- **FR-008**: Copy MUST use the same `extraMetadata` merge already used for `queue` and `ig_comment_id`, so `msgs_msg.metadata` and the Courier queue JSON receive `custom_data`.
- **FR-009**: `ResendMessages` MUST keep existing `metadata.custom_data` (no strip on resend).
- **FR-010**: Old Mailroom remains compatible until this ships (unknown field drop). New Mailroom with old Flows (no field on task) MUST behave as today.
- **FR-011**: Structured logs and Sentry extras MUST NOT include `custom_data` contents (treat as message body; architecture Risks + Principles IV/V).

### Key Entities

- **WppBroadcast / WppBroadcastBatch**: existing queued payloads; `msg` gains optional `custom_data`.
- **Outgoing Msg**: existing `msgs_msg` row; `metadata` JSON gains optional `custom_data`.
- **custom_data**: opaque JSON object, same value for all recipients of one broadcast.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of AC-M1 fixtures (WWC and non-WWC) persist `metadata.custom_data` equal to the task object on every created message.
- **SC-002**: 100% of AC-M2 fixtures omit `custom_data` on metadata.
- **SC-003**: 100% of AC-M3 non-object fixtures complete without task error and omit `custom_data`.
- **SC-004**: AC-M4 fixtures store expression-like strings unchanged for every recipient.
- **SC-005**: Existing `TestWppBroadcastTask` and `CreateWppBroadcastMessages` tests without `custom_data` remain green (no behavior change for callers that omit the field).
- **SC-006**: Broadcast create path adds no extra per-recipient evaluation cost for `custom_data` (one decode, N copies of the same value).

## Technical Approach

### Components Affected

| Component | Change | Why |
|-----------|--------|-----|
| `core/models/msgs.go` `WppBroadcastMessage` | add a field that can hold a JSON object or be empty after defensive decode | Typed struct currently drops the key |
| `CreateWppBroadcastMessages` `extraMetadata` block (~2174) | if present object (not `{}`), set `extraMetadata["custom_data"]` without Excellent | AD-001, AD-004 |
| `core/tasks/msgs/send_wpp_broadcast.go` | none required if unmarshal lives on the struct; keep failing only on whole-task JSON errors | Defensive drop must happen inside decode of the field, not as a task-level error |
| `core/models/msgs_test.go` | AC-M1–M4 (and merge with existing extraMetadata keys) | Principle X at model layer |
| `core/tasks/msgs/send_wpp_broadcast_test.go` | task JSON → batch → created msgs (AC-M1–M3) | Full task flow |
| Resend test (existing `ResendMessages` coverage or new case in `msgs_test.go`) | AC-M5 | Contract: resend reads DB metadata |
| `WENI-CHANGELOG.md` | contract-affecting note | Principle XVI |

Defensive decode (binding contract): prefer storing the raw JSON of `custom_data` then accepting it only when it unmarshals as a JSON object (e.g. `map[string]interface{}` or equivalent). Non-object or unmarshal failure of **that field only** → treat as absent. Do not use a typed `map` JSON tag alone if that would make `json.Unmarshal` of the **whole** `WppBroadcast` fail on a string/array `custom_data`.

`CreateBatch` already copies `Msg`; once the parent broadcast decodes the field, batches inherit it. Batch tasks unmarshaled from Redis MUST apply the same field decode.

### Flow

1. Flows (other repo, deploy step 3) validates and queues `send_whatsapp_broadcast` with optional `msg.custom_data`.
2. Mailroom unmarshals the task. Non-object `custom_data` is dropped; object/`{}`/absent continue.
3. Batches copy `Msg` including accepted `custom_data`.
4. `CreateWppBroadcastMessages` copies accepted non-empty object into `extraMetadata` once per message, no evaluation.
5. `NewOutgoingWppBroadcastMsg` merges metadata; persist + Courier queue.
6. Resend loads the row; metadata already contains `custom_data`.

### Rejected Alternatives

- Filter WWC in Mailroom: contradicts AD-003.
- Recursively evaluate strings with Excellent: contradicts AD-004.
- Fail the task on non-object `custom_data`: contradicts the task schema MUST.
- Re-validate size/depth/keys: contradicts AD-002; would duplicate Flows and could drop values Flows accepted.
- Copy only on the parent task and not on batch unmarshal: batch JSON is decoded independently.

## Contracts

Do **not** copy full OpenAPI into this repository. Pin and reference:

| Interface | Role | File (pinned) | Version impact | Backward compatible |
|-----------|------|---------------|----------------|---------------------|
| Task `send_whatsapp_broadcast` | consumer | `events/send-whatsapp-broadcast-task.schema.json` | Mailroom MINOR | yes — optional field; old Mailroom drops it |
| Outgoing msg metadata / Courier JSON | producer | `events/courier-msg-metadata.schema.json` | Mailroom MINOR | yes — optional key, absent when no value or `{}` |
| Value shape | not re-validated here | `custom-data.schema.json` | n/a in Mailroom | Flows owns limits |

Pin: tag `006-wwc-custom-data/arch-v1`, commit `1ebcd6346d46d269f6421d7f3c10344ebdda394f`.

Courier versions: Mailroom MAY produce `metadata.custom_data` only after Courier step 2 in the architecture sequence if operators follow that order; if Mailroom shipped early, old Courier would ignore the key. Architecture still requires Mailroom last so unvalidated values never propagate.

## Data Model and Migrations

No new table or RapidPro migration. `msgs_msg.metadata` is existing JSON. Peak storage: up to 16384 UTF-8 bytes per message times recipient count (architecture Risks; AD-003 applies to non-WWC too). Mailroom MUST NOT add schema that RapidPro does not own.

## Non-Functional Requirements

### Peak Load

- **Declared peak**: existing production peak of `send_whatsapp_broadcast` / `send_whatsapp_broadcast_batch` (campaign-sized fan-out), plus copying one JSON object of at most 16384 bytes into each created message’s metadata (no extra Excellent eval, no extra Redis round-trip beyond today’s batch queue).
- **Statelessness**: no new process-local state; metadata lives on `msgs_msg` and in the Courier payload.

### Observability

- Do not log `custom_data` contents. Correlation remains `org_id`, `broadcast_id`, message UUID.
- No new metric required; existing broadcast/msg hooks suffice.

### Failure Handling

- Non-object `custom_data`: drop field, continue (Principle VII).
- Other unmarshal / DB / queue errors: unchanged wrapped errors; a failing task MUST NOT crash the worker.
- Timeouts: existing `handleSendWppBroadcast` 60-minute context unchanged.

### Security and Authorization

- Task JSON is untrusted (Principle III). Defensive type check only; size/depth/key trust is Flows (AD-002) plus deploy order (Mailroom last).
- Opaque integrator data may contain PII; treat as message content (no Sentry extras).

## Test Strategy

Map each Mailroom AC to tests in this repo. Prefer `testsuite` + `mailroom_test` DB. Isolated unmarshal tests MAY cover AC-M3 shapes but MUST NOT be the only coverage for AC-M1 (Principle X: task or `CreateWppBroadcastMessages` through persisted/queued metadata).

| Acceptance criterion | How this repository proves it |
|----------------------|-------------------------------|
| AC-M1 valid object on WWC and non-WWC | `CreateWppBroadcastMessages` with `custom_data` set to the contract example; assert each `Msg` metadata map has `custom_data` equal to that object. Repeat with the org’s default/non-WWC channel used by existing broadcast tests **and** a channel typed as WWC if fixtures allow; if no WWC channel fixture exists, use two distinct `ChannelID`s / types already in testdata and document that Mailroom does not branch on type. Also a `send_wpp_broadcast_test.go` case: raw task JSON with `msg.custom_data` → `CreateWppBroadcastBatches` → `SendWppBroadcastBatch` → `msgs_msg.metadata` JSON contains the object. Multi-recipient: same object on every row. |
| AC-M2 omit and `{}` | Existing `TestWppBroadcastTask` remains green. New cases: no key; `custom_data: {}`. Assert `metadata` JSON has no `custom_data` key (`metadata::jsonb ? 'custom_data'` is false, or Go map lookup missing). |
| AC-M3 null/string/array/number | Task JSON (not only Go struct literals) for each type. Assert `SendWppBroadcastBatch` / create returns nil error; messages exist; no `custom_data` key. Unmarshal of the **whole** task MUST succeed. |
| AC-M4 verbatim `@contact.name` | Object `{"label":"@contact.name"}` for Cathy (or named contact); assert metadata string is `@contact.name`, not `Cathy`. |
| AC-M5 resend | Create/persist msg with `custom_data`; `ResendMessages`; assert loaded/updated msg metadata still contains the object. Do not require Courier HTTP. |

**Gates**: `gofmt`, `go vet`, `go test ./... -p=1` (at least `./core/models` and `./core/tasks/msgs`). Coverage SHOULD NOT drop.

## Rollout and Rollback

Mailroom is **deploy step 4 — last** in the architecture Delivery Sequence:

1. webchat-service and weni-webchat-socket (other repos).
2. courier (other repo).
3. **flows** (validate `msg.custom_data`) — **MUST be deployed before this Mailroom change**. This repo MUST NOT ship while Flows still accepts arbitrary `msg` keys without AD-002, because new Mailroom would copy unvalidated objects.
4. **This Mailroom change** (propagate).
5. Announce to integrators (not this repo’s job beyond `WENI-CHANGELOG.md`).

**Rollback of Mailroom alone is safe**: old Mailroom drops unknown `custom_data` again; no task failures; field stops arriving at Courier. Reverting Mailroom does not require reverting Flows. Reverting Flows while this Mailroom stays deployed is **unsafe** (architecture Rollback).

No feature flag; deploy order is the flag.

## Constitution Alignment

| Principle | How this spec complies | Exception + justification |
|-----------|------------------------|---------------------------|
| I. Version Control and Review | PR to `weni-ai/mailroom` `main`, review + green `ci.yml`. | none |
| II. Security and Secrets | No new secrets; fixtures remain fake. | none |
| III. Never Trust the Client | Task JSON untrusted; defensive type check; do not fail closed on that field per contract. | Size/depth trusted to Flows + deploy order, not re-checked here (AD-002). |
| IV. Observability | Do not log `custom_data`; keep `org_id` / `broadcast_id`. | none |
| V. Diagnosable Errors | Task errors stay wrapped without payload body in Sentry extras. | none |
| VI. Versioned Contracts | Additive optional field; MINOR; tolerate old/new during rolling deploy. | none |
| VII. Fail Gracefully | Non-object `custom_data` dropped; worker not crashed; other errors still wrapped. | none |
| VIII. Bounded Retry Over REST | No new REST hop; Courier queue unchanged except metadata key. | none |
| IX. Scalability and Peak Load | Peak declared: current wpp-broadcast fan-out + ≤16 KB copy per message; stateless. | none |
| X. Tests Exercise Flows | Task → created msgs; success and drop paths; resend. | none |
| XI. Explicit Over Clever | Field copy in the existing `extraMetadata` block; comments explain AD-004 (no Excellent). | none |
| XII. Contained Changes | Only wpp-broadcast decode + extraMetadata + tests + changelog. | none |
| XIII. Specification Traceability | Origin Without Product Spec; architecture pinned `006-wwc-custom-data/arch-v1` (`1ebcd6346d46d269f6421d7f3c10344ebdda394f`). | Engineering origin allowed; architecture is the "what". |
| XIV. No Silent Divergence | No Product Spec; architecture ADs inherited; no silent WWC filter or Excellent eval. | none |
| XV. Commit Messages | Conventional Commits, ≤50 character descriptions. | none |
| XVI. Changelog Maintenance | `WENI-CHANGELOG.md` for the metadata/task field. | none |

SDD Lite: implement from this spec; do not generate `plan.md` / `tasks.md` unless promoted.

## Implementation Decisions

- Decode `custom_data` so a non-object cannot fail `json.Unmarshal` of the entire `WppBroadcast` / `WppBroadcastBatch`.
- Copy only in `CreateWppBroadcastMessages` `extraMetadata` (single write path to `msgs_msg.metadata`).
- Treat `{}` as absence at copy time (and after decode).
- Do not add a Mailroom HTTP API or config flag.

## Assumptions

- Flows `queue.py` already puts `Broadcast.metadata` on task `msg`; Mailroom does not need a Flows change in this repo.
- Testdata has at least one channel used by current wpp broadcast tests; AD-003 is proven by not branching on `channel.Type()` plus creating msgs on whatever channel the batch resolves.
- Tag `006-wwc-custom-data/arch-v1` remains `1ebcd6346d46d269f6421d7f3c10344ebdda394f`.
- Resend does not rebuild metadata from the original task; proving the DB value survives `ResendMessages` is sufficient for AC-M5.

## Open Questions

None.
