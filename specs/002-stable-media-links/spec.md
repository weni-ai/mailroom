# Feature Specification: Stable Media Links on Outbound Webhooks

**Feature Branch**: `002-stable-media-links`

**Created**: 2026-11-04

**Status**: Draft

**Input**: User description: "aplicar tratamento para mídia ser um link estável no ponto de saída de webhooks que contenha no valor de alguma chave do body o link para qualquer mídia dos buckets conhecidos."

## Inheritance from Product Spec

- Product Spec: none supplied — this spec was requested directly from an operational finding
- Pinned version: none
- Architecture doc: none
- Inherited binding decisions: a file stored in a known location is handed to an external system as a stable link; a file outside those locations is left unchanged; the stored conversation keeps the original file address
- Scope of this spec: outbound webhooks whose body carries an address of a file in a known location
- Divergences: none

## User Scenarios & Testing *(mandatory)*

### User Story 1 - External systems receive a stable file link from a webhook (Priority: P1)

A flow or another outbound call sends a message body to an external system, such as a CRM. One or more fields of that body contain the address of a file the contact sent or the conversation stored in a known location. Before the call leaves, each of those addresses is replaced by a stable link to the same file. The external system can open that link later and retrieve the file. Addresses that do not point at a known location stay as they were. The rest of the body stays as it was.

**Why this priority**: Clients who capture a document or image and forward it to another system currently receive an address they can no longer use. This is the path that restores that handoff.

**Independent Test**: Send a webhook whose body includes one known-location file address and one unrelated address. The call that arrives at the external system contains a stable link for the known file and the unrelated address unchanged. Opening the stable link returns that same file.

**Acceptance Scenarios**:

1. **Given** a webhook body field whose value is the address of a file in a known location, **When** the call is sent, **Then** that field value is a stable link to the same file
2. **Given** a body field whose value contains a known-location file address inside surrounding text, **When** the call is sent, **Then** only that address is replaced by the stable link and the surrounding text is unchanged
3. **Given** a body with several fields and more than one known-location file address, **When** the call is sent, **Then** every such address is a stable link and every other value is unchanged
4. **Given** a body field whose value is an address outside the known locations, **When** the call is sent, **Then** that value is unchanged
5. **Given** the same known file sent on two separate calls, **When** both calls are sent, **Then** both carry the same stable link
6. **Given** a body with no known-location file address, **When** the call is sent, **Then** the body is unchanged

---

### Edge Cases

- A field value that contains several known-location addresses has each one replaced. Addresses outside the known locations in the same value stay unchanged.
- An address that is already the stable link for that file is left as that same link.
- A short-lived address that still identifies a file in a known location is replaced by the stable link for that same file.
- A field value that is not an address, including an empty value, is left unchanged.
- An address that cannot be read as a file location is left unchanged, and the rest of the call is still sent.
- A body that is the file itself, rather than a field value containing an address, is left unchanged.
- An address that appears only in the call's query string, and not in a body field, is left unchanged.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Before an outbound webhook is sent, the system MUST replace each address of a file in a known location that appears in a body field value with the stable link for that same file.
- **FR-002**: The system MUST apply that replacement at any depth of the body, including a field whose value only contains the address as part of a larger text.
- **FR-003**: The system MUST leave every address that does not identify a file in a known location unchanged.
- **FR-004**: The system MUST leave body field values that contain no known-location file address unchanged, including the surrounding text of a value that also contains a replaced address.
- **FR-005**: The system MUST use one stable link per file. Two outbound calls that refer to the same file MUST carry the same stable link.
- **FR-006**: A recipient that holds only the stable link MUST be able to retrieve that file later, including after the original storage address is no longer publicly usable.
- **FR-007**: The system MUST NOT rewrite the file address stored on the conversation. Only the copy placed on the outbound call changes.
- **FR-008**: The system MUST leave addresses that appear only in the query string, and file bodies that are not an address inside a field value, unchanged.
- **FR-009**: A file outside the known locations on a webhook MUST keep the address it already had.
- **FR-010**: An unreadable address MUST be left unchanged and MUST NOT prevent the rest of the call from being sent.

### Key Entities

- **Known location**: A storage location the product already treats as its own for conversation files. Files outside these locations are not rewritten.
- **File address**: The address currently carried for a conversation file, whether that is the original storage address or a short-lived access address for the same file.
- **Stable link**: An address of one specific file that stays the same for that file, can be handed to an external system, and is sufficient for that system to retrieve the file later.
- **Webhook body field**: A named value in the body of an outbound call. The value may be the file address itself or a text that contains it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In 100% of outbound webhooks whose body contains a known-location file address, the external system receives a stable link for every such file.
- **SC-002**: In 100% of those same webhooks, addresses that do not identify a known-location file arrive unchanged.
- **SC-003**: A recipient retrieves the correct file from the stable link on a later attempt, after the moment the call was sent, in 100% of the files covered by this feature.
- **SC-004**: Two webhook deliveries of the same file present the same stable link in 100% of cases.
- **SC-005**: Conversations stored before and after this change still show the original file address. The rewrite is visible only on the outbound copy.

## Assumptions

- A stable link means the same kind of durable file address already handed to external ticket services: one file, the same address every time, usable by the recipient without access to private storage, and still usable on a later retrieval. It is not a short-lived access address.
- Known locations are the storage locations the product already uses for conversation files. Customer-owned storage, including images a customer hosts for message templates, is outside those locations and is left unchanged.
- "A body field value" includes named values in a structured body and in a form body, at any depth. It does not include the query string, headers, or a body that is the file bytes themselves.
- The address stored on the conversation is not migrated. Past and future messages keep the original address. Only the outbound copy is rewritten.
- An address that cannot be interpreted is skipped. The call still goes out.
- Outbound volume does not increase. The change replaces file addresses on webhook calls that already happen. Peak load stays the current peak of those calls.
- Contact history read through the flows service, files sent by other ticket services, and template media hosted by the customer are out of scope.
- No approved product spec was supplied. This engineering spec records the operational request as its source until a product spec is pinned.
