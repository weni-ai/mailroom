# Feature Specification: Durable media links on Zendesk ticket opening

**Feature Branch**: `002-zendesk-durable-media`

**Created**: 2026-11-11

**Status**: Draft

**Input**: User description: "Na abertura de ticket do Zendesk, alguns ticketers enviam o histórico da conversa no texto do ticket, e esse texto às vezes inclui o link de mídia do armazenamento. Quando o texto tiver links de mídia de buckets conhecidos, esses links devem ser substituídos por links de mídia duráveis, do mesmo tipo já usado quando outro canal de ticket compartilha um arquivo."

## Inheritance from Product Spec

- Product Spec: none — no tagged product spec exists; this specification is the originating description of the defect
- Pinned version: none
- Architecture doc: none
- Inherited binding decisions: none
- Scope of this spec: Zendesk ticket opening text only — known-bucket media addresses become durable links before the ticket content is delivered
- Divergences: none

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agent can open history media from a new Zendesk ticket (Priority: P1)

A contact has exchanged messages that include files stored by the platform. A ticketer is configured to place that conversation history in the text used to open a Zendesk ticket. The agent who receives the ticket can open each of those files from a durable media link, including after a direct storage address would have stopped working.

**Why this priority**: This is the defect. Agents currently receive storage addresses that expire or are not meant to be shared, so files in the opening history become unreachable.

**Independent Test**: Open a Zendesk ticket whose opening text contains a conversation history with one or more files stored in a known media bucket. Confirm each of those addresses in the delivered ticket text is a durable media link and that the file opens from it.

**Acceptance Scenarios**:

1. **Given** an opening text that includes a direct address of a file in a known media bucket, **When** the Zendesk ticket is opened, **Then** the delivered text contains a durable media link for that file instead of the direct storage address.
2. **Given** an opening text that includes several files from known media buckets, **When** the Zendesk ticket is opened, **Then** every one of those addresses is a durable media link.
3. **Given** an opening text whose known-bucket address already carries a temporary signature, **When** the Zendesk ticket is opened, **Then** the delivered text contains a durable media link and does not keep the temporary signature.
4. **Given** a durable media link in the delivered ticket, **When** an agent opens it after a direct storage address for the same file would have expired, **Then** the file still opens.

---

### User Story 2 - History stays intact around the replaced links (Priority: P2)

The opening text often mixes conversation wording, subjects, extra fields, links to the public internet, and files the platform stores. Only the known-bucket media addresses change. The agent still reads the same history.

**Why this priority**: Replacing the wrong text, or rewriting links the platform does not own, would corrupt the ticket or send agents to the wrong place.

**Independent Test**: Open a Zendesk ticket whose text contains history wording, a public internet link, an address that is already a durable media link, and one known-bucket file address. Confirm only the known-bucket file address changes.

**Acceptance Scenarios**:

1. **Given** opening text that mixes history wording with one known-bucket file address, **When** the Zendesk ticket is opened, **Then** the wording around that address is unchanged and only the address is replaced.
2. **Given** a public internet link in the opening text, **When** the Zendesk ticket is opened, **Then** that link is unchanged.
3. **Given** an address that is already a durable media link, **When** the Zendesk ticket is opened, **Then** that address is unchanged.
4. **Given** opening text that is either a plain history or a structured ticket (subject, message, description, and extra text fields), **When** a known-bucket media address appears in any of that text, **Then** it is replaced in the text that is delivered.
5. **Given** a later message on an existing Zendesk ticket, or a ticket opened on another channel, **When** that action runs, **Then** file sharing stays as it is today.

---

### User Story 3 - Ticket opening still succeeds when a durable link cannot be issued (Priority: P3)

Durable links are the normal result. When the platform cannot issue one, the contact is not left without a ticket, and the agent still receives the same kind of file address already used for attachments in that situation.

**Why this priority**: Opening the ticket matters more than the link upgrade. A link rewrite must not become a new reason for ticket creation to fail.

**Independent Test**: Open a Zendesk ticket whose text contains a known-bucket file address while durable links are unavailable. Confirm the ticket is still created and the address follows the same fallback already used for shared attachments.

**Acceptance Scenarios**:

1. **Given** durable media links cannot be issued and a known-bucket file address is in the opening text, **When** the Zendesk ticket is opened, **Then** the ticket is still created and that address follows the existing attachment fallback.
2. **Given** an opening that does not send opening text, **When** the ticket action runs, **Then** behavior is unchanged.
3. **Given** opening text with no media addresses, **When** the Zendesk ticket is opened, **Then** the text is unchanged and the ticket is created as it is today.

---

### Edge Cases

- The same file address appears more than once in the opening text; each occurrence is replaced.
- A known-bucket address sits inside a sentence or a structured text field; only the address is replaced.
- The opening text contains a mix of known-bucket addresses, public links, and addresses that are already durable.
- The known-bucket address is a temporary signed storage address; the signature is not delivered.
- The opening text is empty or has no media addresses; nothing is rewritten.
- The address looks like storage but is not in a known media bucket; it is left unchanged.
- The action reopens an existing ticket and does not send opening text; nothing is rewritten.
- A durable link cannot be issued; ticket opening still succeeds and that address uses the existing attachment fallback.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When a Zendesk ticket is opened, every media address in the opening text that points at a known media bucket MUST be replaced with a durable media link before that text is delivered.
- **FR-002**: The replacement MUST cover the whole opening text, whether it is plain conversation history or structured ticket content that includes a subject, message, description, or extra text fields.
- **FR-003**: When the opening text contains more than one known-bucket media address, every occurrence MUST be replaced.
- **FR-004**: Addresses that do not point at a known media bucket MUST stay unchanged. This includes public internet links and addresses that are already durable media links.
- **FR-005**: A known-bucket address that already carries a temporary signature MUST be recognized as that stored file and replaced with a durable media link. The temporary signature MUST NOT be delivered.
- **FR-006**: Text that is not part of a replaced address MUST remain unchanged.
- **FR-007**: If a durable media link cannot be issued, ticket opening MUST still succeed, and that address MUST follow the same fallback already used when a shared attachment cannot be given a durable link.
- **FR-008**: The rewrite MUST apply only to text sent while opening a Zendesk ticket. Later messages on that ticket, and ticket opening on any other channel, MUST keep their current behavior.
- **FR-009**: A Zendesk ticket action that does not send opening text MUST keep its current behavior.

### Key Entities

- **Opening text**: The content handed over when a Zendesk ticket is created. It may be a plain conversation history or structured ticket content, and it may include file addresses.
- **Known media bucket**: A storage location the platform already treats as its own when it shares a stored file on other ticket channels. The primary media store and any additional stores configured for that same sharing qualify. A store used only for internal session records does not qualify unless it is also configured for that sharing.
- **Direct storage address**: An address that points straight at a file in storage, with or without a temporary signature. It is not what the agent should keep.
- **Durable media link**: A stable address for the same stored file that keeps opening after a direct storage address would expire. It is the same kind of address already used when another ticket channel shares a stored file.
- **Zendesk ticket**: The ticket created for an agent from the opening text.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In Zendesk tickets opened with conversation history that includes files from known media buckets, 100% of those files are represented by durable media links in the delivered opening text.
- **SC-002**: An agent can open each of those files from the ticket on the first attempt after a direct storage address for the same file would have expired, without asking the contact to send the file again.
- **SC-003**: For openings whose text has no known-bucket media, the delivered text is unchanged and the share of openings that succeed stays the same as before this change.
- **SC-004**: In openings that mix history wording, public links, and known-bucket files, reviewers find no change other than the known-bucket addresses.

## Assumptions

- The defect is limited to the text used to open a Zendesk ticket. Some ticketers place conversation history in that text, and that history can include file addresses from platform storage. Later Zendesk messages, and other ticket channels, already share files by their own rules and are out of scope.
- "The same kind of durable link already used when another ticket channel shares a file" is the required result. Known media buckets are exactly the storage locations already eligible for that kind of link: the primary media store plus any additional stores configured for the same sharing. The internal session store is excluded unless it is also on that list.
- Replacement changes only the address. Structured opening text remains valid, and history wording stays in place.
- When durable links are unavailable, the existing attachment fallback applies: a short-lived signed address when signing is available, otherwise the original address. Ticket opening is not failed because of the rewrite.
- An opening that only updates an existing ticket and does not send the opening text is unchanged.
- This does not add tickets, messages, or outbound conversations. Volume stays the volume of Zendesk ticket openings that already happen today.
- No tagged product spec exists for this defect. This document is the originating specification.
