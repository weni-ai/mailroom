<!--
Sync Impact Report:
- Version change: unversioned template → 1.0.0 (first generation)
- Modified principles: none (all placeholders replaced)
- Added principles:
  I. Version Control and Review
  II. Security and Secrets
  III. Never Trust the Client
  IV. Observability
  V. Diagnosable Errors
  VI. Versioned Contracts
  VII. Fail Gracefully and Predictably
  VIII. Bounded Retry Over REST
  IX. Scalability and Peak Load
  X. Tests Exercise Flows
  XI. Explicit Over Clever
  XII. Contained Changes
  XIII. Specification Traceability
  XIV. No Silent Divergence
  XV. Commit Messages
  XVI. Changelog Maintenance
- Added sections: Technology and Runtime Constraints, Development Workflow and Quality Gates, Governance
- Removed sections: none
- Templates requiring updates:
  ⚠ .specify/templates/spec-template.md — does not yet open with the mandatory
    "## Inheritance from Product Spec" section (Principle XIII); until it does,
    /speckit.specify output MUST have that section added manually.
  ✅ .specify/templates/plan-template.md — generic "Constitution Check" gate is compatible.
  ✅ .specify/templates/tasks-template.md — no constitution-specific references.
- Follow-up TODOs:
  TODO(BRANCH_PROTECTION): confirm that branch protection on weni-ai/mailroom `main`
    blocks direct pushes and requires 1 approval + green CI (Principle I).
  TODO(PRODUCT_SPEC_REPO): record the URL of the product-spec repository that
    engineering specs in `specs/` inherit from (Principles XIII, XIV).
  TODO(ACCOUNT_ID_MAPPING): confirm which Mailroom field represents the "account
    identifier" for error reports (Principle V); org ID / project UUID are assumed
    for the project identifier.
  TODO(CHANGELOG_FORMAT): decide whether WENI-CHANGELOG.md migrates to Keep a
    Changelog categories (Principle XVI).
  TODO(VULN_SCAN): CI has no dependency vulnerability check yet (e.g. govulncheck)
    (Principle II).

Provenance:
- Source: weni-ai/vtex-cx-engineering-constitutions (main)
- Files: base-constitution.md, backend/base-constitution.md
- Domains: backend
- Project layer: weni-ai/mailroom (Go fork of nyaruka/mailroom)
-->

# Mailroom Constitution

Mailroom is the Weni-maintained fork of the RapidPro worker
(`github.com/nyaruka/mailroom`). It executes flows, campaigns, messaging, IVR,
tickets, and other background tasks, exposes an internal HTTP API under `web/`,
and integrates with PostgreSQL, Redis, RabbitMQ, SQS, ElasticSearch, and S3.
This constitution binds every change to this repository.

## Core Principles

### I. Version Control and Review

- All code MUST enter `main` through a pull request on
  `github.com/weni-ai/mailroom`.
- A merge MUST require at least one approved review and a green run of the `CI`
  workflow (`.github/workflows/ci.yml`).
- Direct pushes to `main` MUST be blocked via GitHub branch protection, not by
  convention.

**Rationale:** The policy is only real when the platform enforces it. Peer review
and a protected `main` keep history auditable and stop unreviewed changes from
reaching the production worker that every Weni project depends on.

### II. Security and Secrets

- Secrets MUST never be committed. This includes `MAILROOM_AUTH_TOKEN`,
  `MAILROOM_DB` / `MAILROOM_REDIS` credentials, `MAILROOM_SENTRY_DSN`, AWS keys,
  SMTP credentials, and channel or ticketer tokens. Test fixtures (`testsuite/`,
  `mailroom_test.dump`, `weni_dump.sql`) MUST contain only fake credentials.
- Secrets MUST come from an external secrets manager and be injected at runtime as
  `MAILROOM_*` environment variables read by `runtime.Config`. Defaults in
  `runtime/config.go` MUST NOT hold real credentials.
- Access MUST follow least privilege. Cloud access SHOULD use workload identity
  (e.g. IRSA for S3) rather than static access keys.
- Dependencies MUST come only from trusted sources: the Go module proxy and the
  forks pinned by `replace` directives in `go.mod` (`weni-ai/gocommon`,
  `weni-ai/goflow`). Dependencies MUST be checked for known vulnerabilities.

**Rationale:** Leaked credentials and untrusted dependencies are among the most
common and most damaging breaches. Mailroom holds database and messaging-channel
credentials for every org, so prevention is far cheaper than remediation.

### III. Never Trust the Client

- Everything that reaches Mailroom from outside MUST be treated as potentially
  malicious, incomplete, or incorrect until validated. That covers HTTP requests to
  `web/` endpoints, webhook and ticketer callbacks, and payloads consumed from
  Redis queues, RabbitMQ, and SQS.
- HTTP request bodies MUST be decoded with a size limit and validated for type,
  format, range, and business rules before use (for example
  `utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes)` with
  `validate` struct tags), and then checked against business rules in the handler.
- Authorization MUST be enforced on the server for every request (for example
  `web.RequireAuthToken` or the org/user checks of the handler). It MUST also be
  scoped to the org the request targets, whatever checks the caller already ran.

**Rationale:** Clients and upstream services run outside Mailroom's control and
can be modified or bypassed. Only server-side validation prevents injection,
cross-org data access, and corruption of the shared RapidPro database.

### IV. Observability

- Logs MUST be structured (`logrus` with fields such as `org_id`, `contact_uuid`,
  `session_uuid`, `task_type`) and MUST NOT contain secrets, auth tokens, or
  sensitive personal data such as message text, URNs, or phone numbers.
- Errors MUST be traceable across components through correlation identifiers:
  the HTTP request ID (`middleware.RequestID`), task/queue identifiers, and
  flow session/run UUIDs.
- Metrics MUST go through the existing hooks (Prometheus/Librato) rather than
  ad-hoc logging.

**Rationale:** Structured, privacy-safe telemetry makes incidents diagnosable
without creating new data-exposure risks. This matters most in a multi-tenant
worker processing end-user messages.

### V. Diagnosable Errors

- Every error that reaches Sentry (via the `logrus_sentry` hook on error, fatal,
  and panic levels) MUST carry enough context to be located and filtered without
  reproducing it. At minimum that means the project identifier (org ID / project
  UUID), the account identifier, the user or contact identifier, and the
  correlation identifier of the request or task.
- These identifiers MUST be opaque (IDs/UUIDs).
- Names, e-mail addresses, phone numbers, URNs, message content, and government
  identifiers MUST NOT be attached to an error report under any circumstance.

**Rationale:** An error without context can be counted but not investigated.
Opaque identifiers give the filtering an investigation needs and keep reports free
of personal data, as Principle IV requires.

### VI. Versioned Contracts

- Every public interface MUST be versioned following SemVer. Mailroom's public
  interfaces are:
  - the HTTP API under `web/` consumed by RapidPro/Weni services;
  - task and message payloads exchanged via Redis (including with Courier),
    RabbitMQ, and SQS;
  - the shared RapidPro database schema Mailroom reads and writes;
  - configuration keys (`MAILROOM_*`).
- Changes MUST be backward compatible or ship with an announced deprecation path.
  Silent breaking changes MUST NOT be introduced.
- Payload changes MUST tolerate old and new formats during rolling deploys.

**Rationale:** RapidPro, Courier, Weni services, and running Mailroom instances of
different versions all depend on these contracts at the same time. Explicit
versioning and deprecation let them adapt without outages.

### VII. Fail Gracefully and Predictably

- Every call to an external dependency (PostgreSQL, Redis, RabbitMQ, SQS, S3,
  ElasticSearch, webhooks, ticketers, classifiers, Weni services) MUST have an
  explicit timeout or a `context.Context` deadline. Such calls MUST NOT block
  indefinitely.
- Failures MUST be handled explicitly. They MUST be returned as wrapped errors
  (`github.com/pkg/errors`) and surfaced as consistent error responses through the
  `web/` error helpers. They MUST NOT surface as unhandled panics or leaked
  internals such as SQL, stack traces, or credentials.
- A failing task MUST NOT crash the worker or block other orgs' queues.

**Rationale:** Failure is a certainty, not an edge case. Handling it explicitly
keeps a partial outage of one dependency or one org contained instead of stalling
the whole worker or exposing internals to callers.

### VIII. Bounded Retry Over REST

- When data is propagated to another service over REST, a failed call MUST be
  retried rather than dropped.
- A retry MUST be attempted only on failures that could succeed on another
  attempt: connection errors, timeouts, HTTP 5xx, or HTTP 429. It MUST NOT be
  attempted on a 4xx that reflects a defect in the request.
- Retries MUST only apply to operations that are idempotent or protected by a
  deduplication key (e.g. message/ticket UUIDs). Operations that are neither MUST
  be made idempotent rather than left without retry.
- Every retry policy MUST define a maximum number of attempts and a backoff
  strategy, preferably via `httpx.Retries` from `gocommon`. Limits MUST be named
  constants or `runtime.Config` values (e.g. `WebhooksMaxRetries`). Unbounded retry
  MUST NOT be used.
- When attempts are exhausted, the failure MUST be logged and MUST remain
  recoverable: persisted as a failed or errored record, recorded in HTTP logs, or
  requeued. It MUST NOT be silently discarded.

**Rationale:** Propagation between services usually fails for transient reasons,
so retrying keeps services converging. But resending a request rejected on its
merits only adds load, and retrying non-idempotent operations duplicates effects
such as messages or tickets. Bounds keep retry from amplifying an outage. A
recoverable exhausted state keeps data from disappearing between two services
that each believe they succeeded.

### IX. Scalability and Peak Load

- Mailroom instances MUST be stateless so they can scale horizontally. State that
  outlives a single request or task MUST live in PostgreSQL, Redis, S3, or a
  queue, never only in process memory or on local disk.
- In-process caches (e.g. org assets in `core/models/assets.go`) are allowed only
  as rebuildable copies of data whose source of truth is external.
- Cron-style jobs MUST coordinate across instances through Redis locks.
- The expected peak load (messages/s, flow starts, concurrent sessions, or queue
  depth) MUST be declared in the engineering spec of any feature that adds or
  changes a load path, stated as peak and not as average.

**Rationale:** Capacity is a design input, not something to discover during an
incident; sizing for average traffic fails exactly at campaign or seasonal peaks.
Statelessness is what lets adding instances answer load at all.

### X. Tests Exercise Flows

- Every flow MUST have at least one test covering the complete use case from input
  to resulting effect. Examples: an HTTP request to a `web/` endpoint, a queued
  task, or a flow event handled by `core/handlers`, asserted against the
  resulting DB rows, queued messages, or HTTP calls. Such tests SHOULD use
  `testsuite` against the `mailroom_test` database, plus Redis and RabbitMQ.
- Isolated method tests are allowed and SHOULD cover edge cases and input
  variations, but MUST NOT be a flow's only coverage.
- Every flow MUST cover its success path and its failure paths. An error path no
  test exercises MUST NOT be considered covered.
- External HTTP dependencies MUST be mocked (e.g. `httpx` mocks), never called
  for real in tests.

**Rationale:** A suite of isolated method tests can be green while the
composition of handlers, hooks, and models is broken. Failure paths are the least
exercised in development and the most expensive in production.

### XI. Explicit Over Clever

- What code does MUST be evident where it happens. Hidden side effects and
  implicit control flow MUST NOT be introduced to save lines.
- Any literal carrying meaning (a threshold, limit, timeout, retry count, batch
  size, or queue name) MUST be a named constant or a `runtime.Config` field, not an
  inline value. Literals that carry no meaning beyond their value (an index of
  `0`, an increment of `1`) are exempt.
- Comments MUST explain why: the constraint, the trade-off, or the non-obvious
  reason. A comment restating what the code does signals the code SHOULD be
  rewritten.

**Rationale:** Code is read far more often than it is written, usually by someone
without the original context. An unexplained literal is a decision nobody can
review. Why-comments preserve what code cannot carry without going stale.

### XII. Contained Changes

- A change MUST be limited to the context it was asked to address. Refactoring,
  renaming, reformatting, or behaviour adjustments outside that context MUST NOT
  ride along; each belongs to its own change.
- This is especially binding for code inherited from upstream
  `nyaruka/mailroom`, where unrelated edits make future upstream merges harder.
- Principle XV (atomic commits) governs how a change is split internally. An
  in-scope change MAY span several commits.

**Rationale:** A change beyond its stated scope is a change nobody reviewed on
purpose. It hides the intended fix in unrelated edits and turns a revert into a
choice between losing the fix and keeping an unrelated regression.

### XIII. Specification Traceability

- Every engineering spec (`specs/<feature>/spec.md`) MUST derive from exactly one
  approved product spec. It MUST reference that spec through an immutable, pinned
  version (commit or tag); a mutable URL or ID alone MUST NOT be used.
- The product spec MUST exist and be tagged before its engineering spec is
  created.
- An engineering spec MUST NOT redefine the inherited "what": problem, scope,
  success criteria, and binding decisions.
- A technical architecture document SHOULD be produced for non-trivial features.
  When it exists, it MUST be linked and pinned by commit or tag; its absence MUST
  NOT block the engineering spec.
- Every engineering spec MUST open with this section, in exactly this format:

```
## Inheritance from Product Spec
- Product Spec: <title> — <URL>
- Pinned version: <commit/tag>
- Architecture doc: <none | URL + commit/tag>
- Inherited binding decisions: <short list>
- Scope of this spec: <slice implemented by this repo>
- Divergences: <none | link to amendment>
```

**Rationale:** Traceability from product intent to execution keeps decisions
auditable. Pinning guarantees every team implements the same version of a feature.
Requiring the product spec prevents work without an agreed problem; keeping the
architecture doc optional avoids ceremony for trivial designs. A single format
keeps the link machine-checkable across repositories.

### XIV. No Silent Divergence

- When a technical need contradicts something inherited from the product spec
  (scope, success criteria, or a binding decision), it MUST NOT be implemented
  silently in code.
- The divergence MUST be raised as an amendment in the product repository and
  recorded in the `Divergences` field of the engineering spec, linking to it.
- Once the amendment is approved and tagged, the engineering spec's
  `Pinned version` MUST be updated to that tag.
- A technical difference that contradicts nothing inherited is an implementation
  decision and MUST live in the engineering spec.

**Rationale:** In a federated model where the product spec is the single source
of truth, a silent code deviation lets intent and implementation drift with no
audit trail.

### XV. Commit Messages

- Commits MUST follow Conventional Commits: `<type>: <description>`. Allowed
  types are `feat`, `fix`, `docs`, `refactor`, `test`, `chore`.
- The description MUST be imperative, specific, and no longer than 50 characters.
- Commits MUST be atomic: one logical change per commit.

**Rationale:** Conventional commits enable automated changelogs and semantic
versioning; atomic commits simplify bisecting, reverting, and reviewing.

### XVI. Changelog Maintenance

- Public libraries MUST maintain a changelog in Keep a Changelog format.
  Mailroom is a deployed service, not a public library, so that format is not
  mandated here.
- Every user-facing or contract-affecting change MUST still be recorded in
  `WENI-CHANGELOG.md` under the release version that ships it.
- `CHANGELOG.md` holds upstream history and MUST NOT be used for Weni changes.
- Version bumps (git tags released by goreleaser) MUST follow SemVer, with
  breaking contract changes (Principle VI) bumping MAJOR.

**Rationale:** A maintained changelog tells RapidPro/Weni operators what each
deploy changes. SemVer alignment sets predictable upgrade expectations.

## Technology and Runtime Constraints

- **Language/toolchain:** Go, version pinned by `go-version` in
  `.github/workflows/ci.yml` (currently 1.23.5). Module path stays
  `github.com/nyaruka/mailroom` for upstream compatibility.
- **Forked dependencies:** `gocommon` and `goflow` resolve to `weni-ai` forks via
  `replace` in `go.mod`. Bumping them is a contract-relevant change (Principle VI)
  and MUST be noted in `WENI-CHANGELOG.md`.
- **Layout:** `cmd/mailroom` (entrypoint), `runtime/` (config and clients), `web/`
  (HTTP API), `core/` (models, handlers, hooks, tasks, queues, msgio, runner),
  `services/` (ticketers, IVR, external services), `testsuite/` (test helpers and
  fixtures).
- **Configuration:** only through `runtime.Config` (file < `MAILROOM_*` env vars <
  CLI flags). New tunables MUST be added there with a `help` tag and a safe
  default.
- **Datastores:** PostgreSQL schema is owned by RapidPro. Mailroom MUST NOT
  introduce schema changes that RapidPro migrations do not define, and test
  fixtures in `mailroom_test.dump` MUST be kept in sync.

## Development Workflow and Quality Gates

- The full suite MUST pass with `go test ./... -p=1` (serial: tests share the
  `mailroom_test` database, Redis, and RabbitMQ) locally and in CI before merge.
- Code MUST be `gofmt`-formatted and pass `go vet`.
- New or changed behaviour MUST ship with flow tests per Principle X. Coverage is
  reported to Codecov and SHOULD NOT decrease.
- Reviewers MUST check Principles II, III, VI, and VIII explicitly for any change
  touching `web/`, queue payloads, or outbound HTTP integrations.
- Releases are cut by pushing a SemVer git tag. The `release` CI job publishes via
  goreleaser only after tests pass.

## Governance

- This constitution supersedes conflicting local practice. When it conflicts with
  the upstream sources (`weni-ai/vtex-cx-engineering-constitutions`), the
  engineering root prevails over the backend domain, and the backend domain
  prevails over this project layer.
- **Amendments** MUST be made by pull request, reviewed under Principle I, and
  regenerated or reconciled with the upstream sources via `setup-engineering`.
  Each amendment MUST update the Sync Impact Report and the version line below.
- **Versioning** of this document follows SemVer:
  - MAJOR: removing or redefining a principle;
  - MINOR: adding a principle or section, or materially expanding guidance;
  - PATCH: clarifications and wording fixes.
- **Compliance:** every `/speckit.plan` MUST include a Constitution Check against
  these principles. Violations MUST be justified in the plan's Complexity
  Tracking table. `/speckit.analyze` treats any conflict with a MUST as CRITICAL.
  Pull request reviews MUST verify compliance.

**Version**: 1.0.0 | **Ratified**: 2026-10-01 | **Last Amended**: 2026-10-01
