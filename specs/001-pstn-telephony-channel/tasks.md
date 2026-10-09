# Tasks: PSTN Telephony Channel (Mailroom)

**Input**: Design documents from `/specs/001-pstn-telephony-channel/`  
**Prerequisites**: plan.md, spec.md

## Format: `[ID] [P?] [Story] Description`

---

## Phase 1: Setup

- [x] T001 Create feature spec artifacts in `specs/001-pstn-telephony-channel/`
- [x] T002 Set active feature in `.specify/feature.json`

---

## Phase 2: User Story 1 - TPH always streams (P1)

- [x] T003 [US1] Add `ChannelTypeTelephony` (`TPH`) in `core/models/channels.go`
- [x] T004 [US1] Set `stream_support` true for TPH in `channelStreamSupport` / `requestToRouter`
- [x] T005 [US1] Keep `config.version >= 2` for non-TPH channels
- [x] T006 [US1] Add `TestRequestToRouterStreamSupportForTPH`
- [x] T007 [US1] Record the contract change in `WENI-CHANGELOG.md`

---

## Dependencies

- Courier `TPH` handler and Flows `TPH` channel type on `feat/telephony-channel`
- Nexus `GRPC_ENABLED_PROJECTS` still required at deploy time (out of Mailroom)

## Validation

```bash
go test ./core/tasks/handler/ -run 'TestRequestToRouterStreamSupport' -p=1 -count=1
```
