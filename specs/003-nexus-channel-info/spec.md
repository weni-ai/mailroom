# Engineering Specification: Channel object on the Nexus router payload (Mailroom slice)

**Repository**: weni-ai/mailroom — backend

**Feature Branch**: `003-nexus-channel-info`

**Created**: 2026-11-17

**Status**: Draft

**Input**: User description: "Adicionar dados de canal no envio de msgs para o nexus a partir do mailroom. No mailroom tem essas informações do canal. Montar em tempo de execução um objeto de canal com channel_uuid, type, name, address e repassar para o Nexus. Isso vai ser importante para o Nexus porque a partir dessa informação eles vão poder dar ao merchant a possibilidade de filtrar as conversas por tipo de canal."

**Suggested cycle**: SDD Lite (`specify` → `implement` from this `spec.md`). Do **not** run `/speckit-plan` or `/speckit-tasks` unless this spec is amended to promote to Full. The change is additive, single-repo, and bounded to one existing outbound payload.

## Inheritance from Product Spec

- Product Spec: none — engineering origin. The "what" comes from the Nexus team request relayed in the task "Adicionar dados de canal no envio de msgs para o nexus a partir do mailroom".
- Pinned version: n/a (no product spec to pin)
- Architecture doc: none. The change adds one optional object to an existing Mailroom → Nexus HTTP payload; the contract is fully described in this spec (see Contracts).
- Inherited binding decisions:
  - BD-001: Mailroom sends Nexus a channel object built at runtime with the fields `channel_uuid`, `type`, `name`, `address`.
  - BD-002: The object describes the channel the contact's message arrived on.
  - BD-003: Purpose: Nexus lets merchants filter conversations by channel type. Storage, filtering, and UI are Nexus responsibilities.
- Scope of this spec: the Mailroom producer side of the payload sent to Nexus `POST /messages`.
- Divergences: none.

## Clarifications

### Session 2026-11-17

- Q: Should brain calls made from inside flows (goflow `services/brain`) also send the channel object in this feature? → A: No. This feature covers only the inbound path in Mailroom (`requestToRouter`). The flow path is tracked as a separate follow-up task, which needs a change in `weni-ai/goflow`.

## Scope in This Repository

**This repository implements**:

- A top-level `channel` object on the JSON body of `POST {RouterBaseURL}/messages` built by `requestToRouter` (`core/tasks/handler/worker.go`). This is the inbound path: an incoming message from an org with BrainOn, no open ticket, no active flow session, and routing allowed by `shouldRouteToBrain`.
- Values taken from the `*models.Channel` already resolved for the incoming message (org assets cache). No new database query or external call.
- Existing top-level `channel_uuid` and `channel_type` stay unchanged.
- Tests covering the new object (Principle X) and a `WENI-CHANGELOG.md` entry (contract-affecting outbound payload).

**Out of scope here**:

- Nexus persistence, API, conversation filter, and UI.
- Brain calls made from inside flows (goflow `services/brain`, resolved through the `weni-ai/goflow` fork). Tracked as a separate follow-up (see Clarifications). Until it ships, conversations that reach Nexus through a flow's brain call carry no `channel` object and will not match a channel-type filter; the Nexus team must be told.
- Any other Mailroom → Nexus traffic (e.g. WhatsApp broadcast queues).
- Translating channel type codes into display names.
- PostgreSQL schema changes.

## Current State

- `core/tasks/handler/worker.go`, `handleMsgEvent`: resolves `channel := oa.ChannelByID(event.ChannelID)`. When the channel is `nil` (or the contact is blocked), the message is archived as handled and never reaches the router. So every router call has a resolved channel.
- `handleBrainRouting` (~696): looks up `project_uuid`, consumes handover context, then calls `requestToRouter(event, rt.Config, contact, projectUUID, channel, routerText)`.
- `requestToRouter` (~1187): builds an anonymous struct with `project_uuid`, `contact_urn`, `text`, `attachments`, `metadata`, `msg_event`, `contact_fields`, `channel_uuid`, `channel_type`, `contact_name`, `stream_support`; posts it with `httpx.DoTrace` using the configured retries; any status `>= 400` returns an error.
- `core/models/channels.go`: `Channel` exposes `UUID()`, `Type()` (`ChannelType`, the raw code such as `WAC`, `WWC`, `IG`, `TPH`), `Name()`, `Address()`. `address` is loaded with `c.address` through `ROW_TO_JSON`, so a `NULL` address decodes to `""`.
- goflow brain path (`weni-ai/goflow@v1.21.1`, `services/brain/service.go`): sends `project_uuid`, `contact_urn`, `text`, `attachments`, `channel_uuid` (from `contact.PreferredChannel()`), `contact_fields`, `contact_name`. No channel type, name, or address. `assets.Channel` in goflow has `UUID`, `Name`, `Address` but no type. Mailroom only wraps this service in `core/goflow/signing.go` to presign attachments and cannot alter its body.
- Existing tests: `core/tasks/handler/worker_test.go` (`TestRequestToRouter` decodes the captured payload and asserts `channel_uuid` / `channel_type`; `TestRequestToRouterStreamSupportByVersion`; `TestRequestToRouterStreamSupportForTPH`), `core/tasks/handler/handler_test.go` (`TestBrainOnWithoutDisableAIResponseRoutesToRouter`, `TestBrainOnIGCommentRoutesToRouter`, etc. run the full queued task against an `httptest` router).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Nexus receives the channel of every routed message (Priority: P1)

A contact writes to a merchant on one of its channels (WhatsApp, Instagram, web chat, telephony…). Mailroom forwards the message to Nexus and tells Nexus which channel it came from: its identifier, type, name, and address. With that, Nexus can let the merchant filter conversations by channel type.

**Why this priority**: It is the whole feature. Without it Nexus has only the channel identifier and type as loose fields and no name or address to show.

**Independent Test**: Queue an incoming message for a BrainOn org on a channel with a known name and address, handle it, capture the request the router receives, and check the `channel` object.

**Acceptance Scenarios**:

1. **Given** an org with BrainOn and a channel with type `WAC`, name "WhatsApp Loja", and address "+5511999999999", **When** a contact's message on that channel is routed to Nexus, **Then** the request carries `channel` with `channel_uuid` equal to the channel UUID, `type` "WAC", `name` "WhatsApp Loja", and `address` "+5511999999999".
2. **Given** a channel with no address, **When** a message on it is routed to Nexus, **Then** `channel.address` is present and equal to `""`, and the request is sent normally.
3. **Given** any routed message, **When** the request is sent, **Then** `channel.channel_uuid` equals the top-level `channel_uuid` and `channel.type` equals the top-level `channel_type`.
4. **Given** an Instagram comment routed to Nexus (forward_comments on) or a telephony (`TPH`) message, **When** the request is sent, **Then** it carries the same `channel` object built from that channel.

---

### User Story 2 - Existing Nexus integration keeps working (Priority: P1)

Nexus already consumes the current payload. Adding the channel object must not change any existing field or when Mailroom calls Nexus.

**Why this priority**: The payload is a live contract (Principle VI); a regression would break AI responses for every BrainOn org.

**Independent Test**: Run the existing router and BrainOn tests unchanged; they stay green.

**Acceptance Scenarios**:

1. **Given** any routed message, **When** the request is sent, **Then** every field present today (`project_uuid`, `contact_urn`, `text`, `attachments`, `metadata`, `msg_event`, `contact_fields`, `channel_uuid`, `channel_type`, `contact_name`, `stream_support`) keeps its name and value.
2. **Given** a message that is not routed today (open ticket, active flow session, `disable_ai_response` true, IG comment without forward_comments), **When** it is handled, **Then** Nexus is still not called.
3. **Given** Nexus answers with an error status, **When** the message is handled, **Then** Mailroom behaves exactly as today.

---

### Edge Cases

- Channel without address: `address` is `""`; the key is always present (never omitted, never `null`).
- Channel name with non-ASCII characters, quotes, or emoji: sent verbatim, JSON-escaped.
- Channel type: always the raw code stored on the channel (`WAC`, `WWC`, `IG`, `FBA`, `TG`, `TPH`, …), identical to `channel_type`.
- Channel renamed or address changed recently: the object reflects the org assets cache at handling time; values converge once the cache refreshes. Acceptable for a filter.
- Channel deleted or not found before handling: unchanged; the message is archived before routing, so Nexus is not called.
- Handover context present: `text` changes as today; `channel` is unaffected.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every request Mailroom sends to Nexus `POST /messages` from the inbound path MUST include a top-level `channel` JSON object.
- **FR-002**: The `channel` object MUST contain exactly the string fields `channel_uuid`, `type`, `name`, and `address`.
- **FR-003**: Values MUST come from the channel the incoming message arrived on (`event.ChannelID`), not from the contact's preferred channel.
- **FR-004**: `type` MUST be the raw channel type code, identical to the top-level `channel_type`.
- **FR-005**: `name` and `address` MUST always be present; when the channel has no value, they MUST be `""`.
- **FR-006**: All fields sent today MUST keep their names and values; `channel_uuid` and `channel_type` MUST NOT be removed.
- **FR-007**: Building the object MUST NOT add database queries or external calls.
- **FR-008**: Routing decisions, retries, timeouts, and error handling of the router call MUST NOT change.
- **FR-009**: Mailroom MUST NOT log the payload or the channel address as part of this change (Principle IV).
- **FR-010**: This change MUST NOT modify the goflow brain path or bump `weni-ai/goflow`; that path is a separate follow-up.

### Key Entities

- **Router channel**: the channel the contact's message arrived on, as described to Nexus. Attributes: identifier (`channel_uuid`), type code (`type`), display name (`name`), address on the provider (`address`: phone number, page ID, bot handle, or empty, depending on the type). Derived at runtime from the existing Mailroom channel record; not stored anywhere new.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After deploy, 100% of messages forwarded to Nexus from the inbound path carry the channel object with all four fields present.
- **SC-002**: In staging, Nexus can classify every forwarded conversation by channel type for every channel type active there, without asking any other service for channel data.
- **SC-003**: Zero regressions in the existing integration: the success rate of Mailroom → Nexus calls in the week after deploy stays within the normal variance of the previous week.
- **SC-004**: Message handling time for routed messages stays within its normal variance after deploy (the change adds no network or database round-trip).

## Contracts

| Interface | Role | Change | Compatibility |
|-----------|------|--------|---------------|
| `POST {RouterBaseURL}/messages?token=…` (Nexus) | producer | add optional top-level object `channel` | additive; existing fields unchanged; Nexus MUST tolerate the new key and its absence |

Request body after the change (only new and channel-related keys shown; all other keys unchanged):

```json
{
  "channel_uuid": "57f1078f-88aa-46f4-a59a-948a5739c03d",
  "channel_type": "WAC",
  "channel": {
    "channel_uuid": "57f1078f-88aa-46f4-a59a-948a5739c03d",
    "type": "WAC",
    "name": "WhatsApp Loja",
    "address": "+5511999999999"
  }
}
```

Field rules:

| Field | Type | Source | Always present | Empty value |
|-------|------|--------|----------------|-------------|
| `channel.channel_uuid` | string (UUID) | channel UUID | yes | never empty |
| `channel.type` | string | channel type code | yes | never empty |
| `channel.name` | string | channel name | yes | `""` |
| `channel.address` | string | channel address | yes | `""` |

## Data Model and Migrations

No new table, column, or RapidPro migration. Data is read from the channel already loaded in org assets.

## Non-Functional Requirements

### Peak Load

- **Declared peak**: unchanged from the current production peak of inbound messages routed to Nexus. The change sends no extra request; each existing request grows by a few hundred bytes (four short strings).
- **Statelessness**: no new process-local state; values come from the rebuildable org assets cache.

### Observability

- No new logs or metrics. Existing router error handling and HTTP tracing stay as they are. Do not log the payload or the channel address.

### Failure Handling

- No new failure mode: building the object cannot fail. Router errors (`>= 400`, connection errors) keep today's wrapped error and retry behavior.

### Security and Authorization

- The channel address is the merchant's own provider address (e.g. its business phone number), not end-user data, but it is still not logged or attached to Sentry (Principles IV/V).
- No new secrets; the router token handling is unchanged.

## Test Strategy

| Acceptance scenario | How this repository proves it |
|---------------------|-------------------------------|
| US1-1 four values | Extend `TestRequestToRouter` (`worker_test.go`): decode `channel` from the captured body and assert `channel_uuid`, `type`, `name`, `address` against the `channelModel` used in the call. |
| US1-1 end to end (Principle X) | Extend a full-task BrainOn test in `handler_test.go` (e.g. `TestBrainOnWithoutDisableAIResponseRoutesToRouter`): insert the channel with a known name, set its `address` (`UPDATE channels_channel SET address = …`, since `testdata.InsertChannel` leaves it `NULL`), flush the cache, handle the queued event, and assert the decoded `channel` object from the router request. |
| US1-2 empty address | Same full-task test style with `address` left `NULL`; assert `channel.address == ""` and that the key is present in the raw JSON. |
| US1-3 consistency | In `TestRequestToRouter`, assert `channel.channel_uuid == channel_uuid` and `channel.type == channel_type`. |
| US1-4 IG / TPH | Extend `TestBrainOnIGCommentRoutesToRouter` and `TestRequestToRouterStreamSupportForTPH` with an assertion on `channel.type`. |
| US2-1 existing fields | Existing assertions in `TestRequestToRouter` and the stream-support tests stay unchanged and green. |
| US2-2 no routing | Existing `TestBrainOnIGCommentWithoutForwardSkipsRouter`, `TestBrainOnDisableAIResponseHandlesAsInbox`, `TestPendingHandoverNotConsumedWithOpenTicket`, `TestPendingHandoverNotConsumedWithWaitingSession` stay green. |
| US2-3 router error | Existing behavior; no new test required beyond the current suite staying green. |

External HTTP stays mocked with `httptest` servers, as in the current tests.

**Gates**: `gofmt`, `go vet`, `go test ./... -p=1` (at least `./core/tasks/handler`). Coverage SHOULD NOT drop.

## Rollout and Rollback

- **Order**: Mailroom can deploy on its own. Nexus is expected to ignore unknown JSON keys today; confirm with the Nexus team before deploy. Nexus can start reading `channel` whenever it is ready and MUST treat its absence as "no channel information" (messages forwarded before the deploy, or after a rollback).
- **Rollback**: reverting Mailroom removes the key; existing fields are untouched, so the integration keeps working.
- No feature flag: the change is additive and has no behavioral branch.

## Constitution Alignment

| Principle | How this spec complies | Exception + justification |
|-----------|------------------------|---------------------------|
| I. Version Control and Review | PR to `weni-ai/mailroom` `main`, review + green `ci.yml`. | none |
| II. Security and Secrets | No new secrets; fixtures stay fake. | none |
| III. Never Trust the Client | No new input; values come from Mailroom's own channel records. | none |
| IV. Observability | No payload or address logging. | none |
| V. Diagnosable Errors | Router errors unchanged; no PII added to error reports. | none |
| VI. Versioned Contracts | Additive optional key on the Nexus payload; existing keys kept; tolerant of old/new during rolling deploy. | none |
| VII. Fail Gracefully | No new failure mode; existing timeouts and wrapped errors. | none |
| VIII. Bounded Retry Over REST | Existing `httpx` retries unchanged. | none |
| IX. Scalability and Peak Load | Peak declared: same request count as today, slightly larger body; stateless. | none |
| X. Tests Exercise Flows | Full queued-task test asserts the object at the router; isolated `requestToRouter` tests cover value variations. | none |
| XI. Explicit Over Clever | Named struct with fixed JSON tags built from the channel in one place. | none |
| XII. Contained Changes | Only `requestToRouter` payload, its tests, and the changelog. | none |
| XIII. Specification Traceability | Engineering origin, no product spec (as in `specs/002-wwc-custom-data`). | No product spec exists for this request; the Nexus team request is the origin of the "what". |
| XIV. No Silent Divergence | Binding decisions BD-001–BD-003 implemented as stated; exclusion of the flow path is recorded in Clarifications and Out of scope. | none |
| XV. Commit Messages | Conventional Commits, ≤ 50 character descriptions. | none |
| XVI. Changelog Maintenance | `WENI-CHANGELOG.md` entry for the new payload field. | none |

## Implementation Decisions

- Represent the object with a named struct (fixed JSON tags `channel_uuid`, `type`, `name`, `address`) and a small constructor from `*models.Channel`, used by `requestToRouter`.
- Add it as field `channel` on the existing request body; do not restructure the body.
- Keep `channel_uuid` / `channel_type` at the top level for compatibility; their eventual deprecation is a separate change agreed with Nexus.
- Do not add a config flag or new HTTP endpoint.

## Assumptions

- Field names follow the task literally: `channel_uuid`, `type`, `name`, `address` inside an object named `channel`.
- Nexus ignores unknown keys on `POST /messages`, so deploying Mailroom first is safe.
- Nexus maps raw type codes to display names for the filter; Mailroom does not translate them.
- A short delay between a channel edit and the value sent (org assets cache refresh) is acceptable for conversation filtering.

## Open Questions

None. Follow-up outside this spec: send the channel object from the goflow brain path (`weni-ai/goflow` `services/brain`).
