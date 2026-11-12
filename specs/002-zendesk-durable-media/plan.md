# Implementation Plan: Durable media links on Zendesk ticket opening

**Branch**: `002-zendesk-durable-media` | **Date**: 2026-11-11 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-zendesk-durable-media/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

When Zendesk `Open` sends the ticket text, scan that text for `http` and `https` URLs and pass each one through `presign.DurableURL`. Known-bucket files become the same durable media links other ticketers already send. Public links, wording, the stored ticket body, later Zendesk messages, and other channels stay as they are. The scan is a regex. The bucket decision is not.

## Technical Context

**Language/Version**: Go 1.23.5 (CI `go-version`)

**Primary Dependencies**: `services/tickets/zendesk` `Open`; `core/presign.DurableURL` (allow list already built from `S3MediaBucket` and `S3SignedBuckets`)

**Storage**: N/A. No schema and no new `MAILROOM_*` key. The local ticket keeps the original body.

**Testing**: `go test ./services/tickets/zendesk/ -count=1 -p=1` with `testsuite` (Postgres `mailroom_test`, Redis, RabbitMQ) and `httpx` mocks. Durable cases use `testsuite.UseDurableLinks`.

**Target Platform**: Mailroom worker (Linux)

**Project Type**: background worker, Zendesk ticketer integration

**Performance Goals**: No added Zendesk or S3 call. One linear scan of the opening text per open. Push latency stays dominated by the existing Any Channel request.

**Constraints**: Do not log opening text or media URLs. Do not fail `Open` when a link cannot be rewritten. Do not change `Forward`, `convertAttachments`, or other ticketers. Replacement must leave JSON and surrounding words intact.

**Scale/Scope**: Peak is the current Zendesk ticket-open rate. The text is one conversation history already included in that push, not a new queue or a new request class.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Result |
| --- | --- | --- |
| I. Version Control and Review | Change reaches `main` only by PR with review and green CI | Pass. Plan does not bypass review. |
| II. Security and Secrets | No new secret material; tests use fake HMAC and fake AWS keys | Pass. Reuse `MediaLinkSecret` and `UseDurableLinks`. |
| III. Never Trust the Client | Opening text is data, not a control path | Pass. Regex finds spans; `DurableURL` parses them. No new HTTP entrypoint. |
| IV. Observability | Do not log message text or file URLs | Pass. The rewrite adds no log. |
| V. Diagnosable Errors | No new error from the rewrite | Pass. `DurableURL` does not fail the caller. Push errors stay the existing wrapped Zendesk errors. |
| VI. Versioned Contracts | Any Channel JSON shape unchanged | Pass. Only URL substrings inside existing string fields change. See [contracts/zendesk-open-media.md](./contracts/zendesk-open-media.md). |
| VII. Fail Gracefully | Rewrite cannot block ticket creation | Pass. Fallback is the current `DurableURL` behavior. |
| VIII. Bounded Retry | No new REST call | Pass. The existing push retry policy is untouched. |
| IX. Scalability and Peak Load | Peak declared; no in-process state that must survive the call | Pass. Peak = current Zendesk opens. One scan of the body that is already sent. |
| X. Tests Exercise Flows | `Open` asserted through the mocked push body, plus edge cases on the helper | Pass. See [quickstart.md](./quickstart.md). |
| XI. Explicit Over Clever | Named regex; reuse `DurableURL` | Pass. |
| XII. Contained Changes | Zendesk `Open` outbound text only | Pass. |
| XIII. Specification Traceability | Engineering spec derives from a tagged product spec | Justified exception. See Complexity Tracking. |
| XIV. No Silent Divergence | Nothing inherited to contradict | Pass. The spec records that it is the originating description. |
| XV. Commit Messages | Conventional Commits when the work is committed | Pass for this design. No commit in this command. |
| XVI. Changelog | User-visible ticketer change recorded in `WENI-CHANGELOG.md` | Pass. Implementation adds a `fix:` bullet on the release that ships it. |

**Post-design re-check**: Same results. Phase 1 adds no endpoint, schema, config key, or second bucket list.

## Project Structure

### Documentation (this feature)

```text
specs/002-zendesk-durable-media/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── zendesk-open-media.md
└── tasks.md             # /speckit-tasks, not created here
```

### Source Code (repository root)

```text
services/tickets/zendesk/service.go            # Open uses a rewritten copy of the body
services/tickets/zendesk/media_links.go        # regex scan + presign.DurableURL
services/tickets/zendesk/media_links_test.go   # span edge cases
services/tickets/zendesk/service_test.go       # Open push assertions
WENI-CHANGELOG.md
```

**Structure Decision**: The helper stays in the Zendesk package. `core/presign` already knows known buckets and durable links; this feature only finds URLs inside one ticketer's opening text.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
| --- | --- | --- |
| XIII. No tagged product spec | The constitution's product-spec repository is still an open TODO, and this defect was specified directly from the Zendesk opening-text bug. `spec.md` records the absence in Inheritance from Product Spec. | Blocking the design on a product-spec tag would leave agents with expiring storage links. There is no inherited "what" to restate or silently change. |

## Design notes

Decisions are in [research.md](./research.md). Entities are in [data-model.md](./data-model.md).

`Open` keeps `flows.OpenTicket(..., body, ...)`. It rewrites a copy, then runs the existing plain-text and JSON branches on that copy. `presign.DurableURL` returns the original URL for unknown hosts and for links that are already durable, returns a signed URL when the media-link secret is empty, and returns the original URL when no signer is configured. The helper therefore has no error return.
