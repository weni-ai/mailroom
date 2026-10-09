# Engineering Spec: PSTN Telephony Channel (Mailroom)

**Feature Branch**: `feat/telephony-channel`  
**Created**: 2026-10-09  
**Status**: Draft  
**Input**: Always send `stream_support=true` to Nexus for channel type `TPH`, so telephony voice mode uses gRPC streaming without reusing Weni Web Chat `config.version`.

## Inheritance from Product Spec

- **Product Spec**: Voice Mode for Telephony — `vtex-cx-engine-specs/specs/004-voice-mode-telephony/spec.md`
- **Pinned version**: `004-voice-mode-telephony`
- **Inherited binding decisions**: BD-001 (reuse Flows/Courier/Mailroom → Nexus), BD-004 (Nexus→gateway agent response MUST be gRPC streamed deltas), BD-010 (PSTN is dedicated channel type `TPH` with `tel:` URN)
- **Scope of this spec**: Mailroom-only — how `requestToRouter` advertises streaming capability on `POST {RouterBaseURL}/messages`. No new inbound receive path, no Courier handler, no claim UI.
- **Divergences**: none. Product spec required streamed agent audio; Mailroom previously gated `stream_support` only on WWC `config.version >= 2`, which TPH does not carry.

## User Scenarios & Testing

### User Story 1 - TPH inbound always requests Nexus streaming (Priority: P1)

When Brain routing forwards a committed caller transcript on a `TPH` channel, Mailroom sets `stream_support: true` on the Nexus payload so the agent reply goes through gRPC (TTS into the call) instead of Courier REST `/send`.

**Why this priority**: Telephony without streaming cannot speak. Relying on WWC `version: 2` in channel config is a leaky workaround.

**Independent Test**: Call `requestToRouter` with a `TPH` channel that has empty config; assert the captured POST body has `stream_support: true` and `channel_type: "TPH"`.

**Acceptance Scenarios**:

1. **Given** a `TPH` channel with empty config, **When** Mailroom POSTs to Nexus, **Then** `stream_support` is `true`
2. **Given** a `TPH` channel with `config.version` 1 or a non-numeric WhatsApp-style version, **When** Mailroom POSTs to Nexus, **Then** `stream_support` is still `true`
3. **Given** a non-TPH channel without `config.version >= 2`, **When** Mailroom POSTs to Nexus, **Then** `stream_support` stays `false`
4. **Given** a non-TPH channel with `config.version >= 2`, **When** Mailroom POSTs to Nexus, **Then** `stream_support` stays `true` (WWC unchanged)

### Edge Cases

- Missing `config.version` on TPH MUST NOT disable streaming
- WhatsApp `config.version` strings such as `v2.35.2` MUST NOT enable streaming unless the channel type is `TPH`
- TPH MUST NOT require a Flows claim field to opt into streaming

## Requirements

- **FR-001**: Mailroom MUST set `stream_support` to `true` on `POST /messages` when `channel.Type()` is `TPH`
- **FR-002**: Mailroom MUST keep the existing `config.version >= 2` rule for every other channel type
- **FR-003**: Mailroom MUST send `channel_type` as the real channel type (`TPH` for telephony)

## Success Criteria

- **SC-001**: `TestRequestToRouterStreamSupportForTPH` passes
- **SC-002**: `TestRequestToRouterStreamSupportByVersion` still passes for non-TPH channels

## Assumptions

- Courier `TPH` handler and Flows channel type already exist on `feat/telephony-channel`
- Nexus still requires `GRPC_ENABLED_PROJECTS` (or `*`) and a connected gateway session to play audio; this spec only flips the Mailroom advertisement flag
- Peak load: same Brain `POST /messages` path already used for WWC; TPH adds no new Mailroom QPS class beyond existing inbound handling
