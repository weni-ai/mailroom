# Implementation Plan: Stable Media Links on Outbound Webhooks

**Branch**: `002-stable-media-links` | **Date**: 2026-11-04 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-stable-media-links/spec.md`

## Summary

Outbound webhooks replace each known-location file address found in a body field with the stable media link already produced by `presign.DurableURL`. Stored messages stay unchanged. Query strings, file bodies, and addresses outside the known locations stay unchanged.

## Technical Context

**Language/Version**: Go 1.23.5 (`.github/workflows/ci.yml`)

**Primary Dependencies**: `core/presign` (`DurableURL`), goflow webhook service factory in `core/goflow`

**Storage**: No schema change. Conversation files stay in the existing S3 media bucket. Messages keep the original attachment address in PostgreSQL.

**Testing**: `go test` with `testify` and `testsuite.UseDurableLinks`. Webhook coverage in `core/goflow`.

**Target Platform**: Linux Mailroom worker

**Project Type**: Backend worker and internal HTTP service

**Performance Goals**: No new outbound call class. A webhook rewrites a body it already reads, bounded by `WebhooksMaxBodyBytes`.

**Constraints**: Reuse `presign.DurableURL` for webhooks. Do not add a second link format. A form body must not gain a raw `&` from a short-lived signed address. An unreadable address must not fail the webhook call.

**Scale/Scope**: Peak stays the current peak of flow webhooks. One code path: `core/goflow/signing.go`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. Version Control and Review**: Pass. Work lands through a pull request. This command does not push to `main`.
- **II. Security and Secrets**: Pass. No new secret. The existing `MediaLinkSecret` signs the stable link. Tests use the fake secret from `testsuite.UseDurableLinks`.
- **III. Never Trust the Client**: Pass. The rewrite only runs on bodies Mailroom is about to send. The media handler already rejects a bad token. No new inbound trust.
- **IV. Observability**: Pass. No new log of message text, URNs, or phone numbers. Existing HTTP traces stay the partner-call record.
- **V. Diagnosable Errors**: Pass. A body that cannot be read fails the webhook call. The error does not attach message text, URNs, or phone numbers.
- **VI. Versioned Contracts**: Pass. Webhook bodies for known-location files change from a short-lived signed address to the stable link. That change is recorded in `WENI-CHANGELOG.md`.
- **VII. Fail Gracefully**: Pass. A bad address is left as-is and the call still goes out.
- **VIII. Bounded Retry**: Pass. The webhook call keeps the existing retry policy. This feature does not add a retry path.
- **IX. Scalability**: Pass. No process-local state. Peak declared above. No new cron.
- **X. Tests Exercise Flows**: Pass. The webhook call gets a success path and a failure path. External HTTP is mocked.
- **XI. Explicit Over Clever**: Pass. No new shared abstraction.
- **XII. Contained Changes**: Pass. Brain calls, ticketers, and the media handler stay as they are.
- **XIII. Specification Traceability**: Pass with the gap already recorded in the spec. No product spec was supplied. This plan does not redefine the spec's what.
- **XIV. No Silent Divergence**: Pass. Reusing `DurableURL` on the webhook exit implements the spec.
- **XV. Commit Messages**: Pass. Not exercised by this command. Implementation commits follow Conventional Commits.
- **XVI. Changelog**: Pass for this phase. Implementation must add a `WENI-CHANGELOG.md` entry because webhook body addresses change for partners.

Post-design re-check: the same gates hold. The webhook body contract names the address change. No gate failed.

## Project Structure

### Documentation (this feature)

```text
specs/002-stable-media-links/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── webhook-body.md
└── tasks.md              # /speckit-tasks, not this command
```

### Source Code (repository root)

```text
core/goflow/
├── signing.go            # rewrite known-location addresses before the webhook is sent
└── signing_test.go

core/presign/
└── link.go               # DurableURL, reused unchanged

web/media/media.go        # existing GET /mr/media/{token}, unchanged
```

**Structure Decision**: Mailroom is a single Go module. This feature edits the webhook exit in `core/goflow`. `core/presign/link.go` is a dependency, not a change.

## Complexity Tracking

None. No constitution gate required a justification.
