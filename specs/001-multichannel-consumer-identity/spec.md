# Engineering Spec: Multichannel consumer identity

**Feature Branch**: `feat/multichannel-consumer-identity`  
**Created**: 2026-10-07  
**Status**: Draft  
**Product spec**: `vtex-cx-engine-specs` / `specs/009-multichannel-customer-identity/spec.md`

This is the Mailroom engineering spec. Product requirements and binding decisions live in the product spec and MUST be followed. This document records the HOW inside Mailroom.

## Inheritance from Product Spec

- Product Spec: Multichannel Consumer Identity — `vtex-cx-engine-specs/specs/009-multichannel-customer-identity/spec.md`
- Pinned version: `c8d007a120bd6cccb67c2eba92afaab773434fc1`
- Architecture doc: `vtex-cx-engine-specs/specs/009-multichannel-customer-identity/architecture.md` @ `c8d007a120bd6cccb67c2eba92afaab773434fc1`
- Inherited binding decisions: BD-001–BD-022, applied only to the Mailroom slice
- Scope of this spec: protocol resolution, follow-up, message binding, AI and human inactivity timers, and close hooks already present on the Weni Chats ticket service
- Divergences: none

Out of this repository: the Consumer graph and attach transaction (`flows`); channel ingest (`courier`); the agent attach command (`weni-cli`). Schema migrations for `msgs_protocol` and `msgs_msg.protocol_id` ship in Flows. Mailroom reads and writes those tables after that migration.

Coordination plan: `docs/plans/multichannel-consumer-identity-plano.md` in the workspace.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An inbound message is bound to one open protocol (Priority: P1)

Courier asks Mailroom which protocol an inbound message belongs to. If the channel names an open protocol, or the URN already has one open protocol, the message binds to it. Otherwise Mailroom opens a new protocol. A message that arrives on a closed protocol opens a new one linked by `predecessor_id` and does not reopen the closed row.

**Why this priority**: Every message that enters through Courier must carry one unit of service.

**Independent Test**: Resolve twice for the same URN and assert the same open protocol. Close it, resolve again, and assert a new protocol whose predecessor is the closed one.

**Acceptance Scenarios**:

1. **Given** a URN with no open protocol, **When** resolve runs, **Then** a protocol in state `open` is created and its id is returned.
2. **Given** a URN with one open protocol, **When** resolve runs without another protocol id, **Then** that protocol is returned and `created` is false.
3. **Given** a closed protocol on the URN, **When** the next message resolves, **Then** a new open protocol is created with `predecessor_id` set and the closed row stays closed.
4. **Given** the same external message id, **When** resolve runs twice, **Then** both calls return the same `protocol_id` and only one message binding exists.
5. **Given** resolve cannot decide, **When** it fails safe, **Then** it opens a new protocol and does not drop the message.

### User Story 2 - An AI-handled protocol closes on CSAT or inactivity (Priority: P1)

Before handover, the protocol closes when the shopper answers the AI CSAT or when the AI inactivity period elapses. The period comes from org config, from 1 to 24 hours, default 1 hour. A second close does not create a second CSAT.

**Why this priority**: Two of the four close routes belong entirely to this service.

**Independent Test**: Open a protocol, record an AI CSAT answer, and assert one close with reason `ai_csat`. Open another, advance past the AI deadline with no handover, and assert one close with reason `ai_inactivity`.

**Acceptance Scenarios**:

1. **Given** a protocol still in the AI phase, **When** the shopper answers the AI CSAT, **Then** the protocol closes once with reason `ai_csat`.
2. **Given** a protocol still in the AI phase, **When** the configured AI inactivity elapses with no shopper message, **Then** the protocol closes once with reason `ai_inactivity`.
3. **Given** a protocol already closed, **When** close runs again, **Then** the operation is a no-op.

### User Story 3 - Handover starts the human timer on the existing Weni Chats open (Priority: P1)

`wenichats.Open` is the handover. It cancels the AI timer and starts the human timer from that instant, including while the room is still queued. The human period comes from org config, from 1 hour to 28 days, default 4 days. `room.update` already calls `tickets.Close`; that close also closes the protocol with reason `attendant` or `ticket_closed` and cancels the timer.

**Why this priority**: The human clock does not need a new event from chats-engine. The hooks already live here.

**Independent Test**: Call the Open path for a contact with an open protocol and assert the timer kind is human and the AI deadline is gone. Deliver `room.update` and assert the protocol is closed once.

**Acceptance Scenarios**:

1. **Given** an open AI protocol, **When** `wenichats.Open` creates the room, **Then** the AI timer stops and the human timer starts from that time.
2. **Given** a human timer running, **When** `room.update` closes the flows ticket, **Then** the protocol closes once and the timer is cancelled.
3. **Given** the sector inactivity feature also ends in `room.update`, **When** that callback arrives, **Then** the protocol closes once and a later timer fire does not send a second CSAT.
4. **Given** an internal `on_hold` signal, **When** it is applied, **Then** the human timer pauses and later resumes from the accumulated idle time. The chats room does not emit this signal yet; the test publishes it directly.
5. **Given** a timer scheduler outage, **When** it recovers, **Then** protocols stay open until idle time is recomputed. They are not closed in bulk.

### User Story 4 - An outgoing flow message keeps the turn protocol (Priority: P2)

A flow reply copies the `protocol_id` of the inbound turn. A send against a closed protocol is refused. Mailroom does not resolve a second protocol on the way out.

**Independent Test**: Handle an inbound message bound to protocol P, produce the flow reply, and assert the outbound row carries P. Close P and assert the next outbound send is refused.

**Acceptance Scenarios**:

1. **Given** an inbound message bound to protocol P, **When** the flow replies, **Then** the outbound message stores P.
2. **Given** protocol P is closed, **When** a send targets P, **Then** the send is refused and P stays closed.

### Edge Cases

- `handleMsgEvent` trusts `protocol_id` already stored by Courier. A message that arrives without one uses the same fail-safe as resolve: open a new protocol, then continue.
- Handover racing the AI timer: once Open runs, the AI timer cannot close the protocol afterwards.
- A value of `solved` is not a close. Mailroom does not treat it as one.
- Mass expiry at the deadline processes closures without blocking ingest and without duplicate CSAT.
- Attach moves `msgs_protocol.contact_id` inside the Flows transaction. Mailroom only reads the new owner. It does not write protocol state during attach.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Mailroom MUST expose a resolve operation that returns `protocol_id`, `created`, and `predecessor_id` for one inbound message.
- **FR-002**: Mailroom MUST bind each message it handles to exactly one protocol, except rows written by the Flows stream insert, which may have a null `protocol_id` until Nexus sends one.
- **FR-003**: Mailroom MUST create a follow-up protocol instead of reopening a closed one.
- **FR-004**: Mailroom MUST close an AI-phase protocol on CSAT answer or on AI inactivity, once.
- **FR-005**: Mailroom MUST start the human timer inside `wenichats.Open` and close the protocol inside the existing `tickets.Close` path used by `room.update`.
- **FR-006**: Mailroom MUST pause and resume the human timer from accumulated idle time when an `on_hold` or `resume` signal is applied.
- **FR-007**: Mailroom MUST copy the turn `protocol_id` onto flow replies and MUST refuse a send to a closed protocol.
- **FR-008**: Mailroom MUST read AI and human inactivity hours from org config, with defaults of 1 hour and 96 hours.
- **FR-009**: Mailroom MUST NOT create a second person record, MUST NOT infer identity, and MUST NOT own the attach transaction.

### Key Entities

- **Protocol**: row in `msgs_protocol`. States `open` and `closed`. Optional predecessor. Timer fields owned by this service.
- **Message**: existing `msgs_msg` plus nullable `protocol_id`.
- **Handover**: the existing Weni Chats room open, not a new chats event.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Two resolves for one URN with an open protocol return that same protocol id.
- **SC-002**: A message after close creates a new protocol linked as follow-up in 100% of the scripted cases, and the closed protocol stays closed.
- **SC-003**: Each scripted close route closes the protocol once.
- **SC-004**: A repeated external message id never creates a second protocol binding.
- **SC-005**: After `wenichats.Open`, the human timer is running and the AI timer is not.
- **SC-006**: An `on_hold` interval longer than the human deadline leaves the protocol open until the timer resumes and completes.

## Assumptions

- Flows ships the `msgs_protocol` table and the nullable `msgs_msg.protocol_id` column before Mailroom runs against a shared database.
- `on_hold` has no producer in chats-engine. Tests inject the signal. Production pause waits until chats emits it.
- The sector inactivity clock in chats-engine is a different feature. When it closes a room it already arrives as `room.update`, and that single close is enough.
- Courier calls resolve over HTTP. The field names are fixed here; the path is fixed in the Courier spec.
- Existing AI conversations are not backfilled into protocols.
