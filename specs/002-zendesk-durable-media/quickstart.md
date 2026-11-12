# Quickstart: Durable media links on Zendesk ticket opening

Validates [zendesk-open-media.md](./contracts/zendesk-open-media.md) against a mocked Zendesk push. No live Zendesk or S3 calls.

## Prerequisites

- Go 1.23.5
- Local `mailroom_test` Postgres, Redis, and RabbitMQ, the same dependencies `services/tickets/zendesk` tests already use via `testsuite.Get()`

## Run

From the repository root:

```sh
go test ./services/tickets/zendesk/ -count=1 -p=1
```

The package test is the flow check: `Open` builds the push, the mock records the request, and the assertion reads that body.

Targeted runs once the tests exist:

```sh
go test ./services/tickets/zendesk/ -count=1 -p=1 -run 'TestOpenRewritesBucketMedia|TestRewriteMediaLinks'
```

## Scenarios

Setup for durable cases: `testsuite.UseDurableLinks` with a test domain, secret, and bucket name `media-bucket`. Compare pushed text to `presign.DurableURL` of the fixture URL.

| Scenario | Body | Expected push |
| --- | --- | --- |
| Plain history | Sentence plus `https://media-bucket.s3.sa-east-1.amazonaws.com/media/photo.jpg` | Durable link for that object. Surrounding words unchanged. Raw bucket host absent. |
| Several files | Two known-bucket URLs, one repeated | Each occurrence is the durable link for that object. |
| Temporary signature | Same object URL with an `X-Amz-Signature` query | Durable link. The query is absent. |
| Mixed text | Known-bucket URL, `https://example.com/a.jpg`, and an address that is already `presign.DurableURL` | Only the known-bucket URL changes. |
| JSON ticket | URL inside `message`, `description`, and a custom field value | Those fields in the push use the durable link. JSON field names stay. |
| Unknown bucket | Host that looks like object storage but is not in the allow list | URL unchanged. |
| No media | Current cookie fixture `fieldTicket` | Push matches today's open snapshot. `ticket.Body()` is still the original string. |
| Reopen by id | `{"id":10}` | Update-ticket call only. No opening text in a push. |
| Fallback | Known-bucket URL with `UseAttachmentSigner` and no media-link secret | Push contains a signed address, `Open` succeeds. |
| No signer | Known-bucket URL and no presign config | Original URL pushed, `Open` succeeds. |
| Forward | Existing forward fixture | Unchanged request snapshot. |

## Failure check

`Open` must not return an error whose cause is the rewrite. Push failures stay the existing Zendesk errors.
