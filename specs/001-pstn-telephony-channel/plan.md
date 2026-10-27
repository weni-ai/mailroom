# Implementation Plan: PSTN Telephony Channel (Mailroom)

**Branch**: `feat/telephony-channel` | **Date**: 2026-10-09 | **Spec**: [spec.md](./spec.md)

**Input**: Engineering spec `specs/001-pstn-telephony-channel/spec.md`

## Summary

Advertise Nexus gRPC streaming for PSTN (`TPH`) on the existing Brain router POST. TPH always sends `stream_support: true`. Other channel types keep `config.version >= 2` (Weni Web Chat).

## Technical Context

**Language/Version**: Go 1.23.5 (CI `go-version`)  
**Primary Dependencies**: existing `core/tasks/handler` Brain routing (`requestToRouter`)  
**Storage**: N/A (reads `channels_channel.channel_type` / config already loaded on org assets)  
**Testing**: `go test ./core/tasks/handler/ -run 'TestRequestToRouterStreamSupport' -p=1` against `mailroom_test` + Redis  
**Target Platform**: Mailroom worker  
**Project Type**: background worker  
**Performance Goals**: no change to Brain POST latency; same path as WWC  
**Constraints**: do not change WWC/WhatsApp streaming semantics  
**Scale/Scope**: existing inbound Brain path; peak = current Mailroom inbound-to-Nexus volume, not a new queue

## Constitution Check

- I. PR to `main` with review + CI  
- II. no secrets  
- VI. `stream_support` contract unchanged except TPH always true  
- X. tests capture the HTTP body sent to Nexus  
- XI. named `ChannelTypeTelephony` and `minWWCStreamVersion`  
- XIII. inherits Product Spec 004 BD-001 / BD-004 / BD-010  
- XVI. `WENI-CHANGELOG.md`

## Project Structure

```
core/models/channels.go          # ChannelTypeTelephony = TPH
core/tasks/handler/worker.go     # channelStreamSupport
core/tasks/handler/worker_test.go
specs/001-pstn-telephony-channel/
WENI-CHANGELOG.md
```

## Implementation

1. Add `ChannelTypeTelephony`.
2. `channelStreamSupport`: TPH → true; else `Atoi(config.version) >= 2`.
3. Tests for TPH with empty / v1 / non-numeric config.
4. Changelog 1.104.0.
