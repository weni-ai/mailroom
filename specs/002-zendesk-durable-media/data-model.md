# Data model: Durable media links on Zendesk ticket opening

No new stored entity, table, or configuration key. The rewrite is a pure function of the opening text and the durable-link configuration that already exists.

## Opening text

The string passed to Zendesk `Open`.

| Field | Meaning |
| --- | --- |
| Stored body | The original string saved on the local ticket. Unchanged. |
| Outbound text | A copy scanned before the push is built. Plain history, or a JSON object whose string fields may contain history. |
| Shape | Plain text when the body does not start with `{`. Otherwise JSON with `message`, `subject`, `description`, `priority`, `tags`, `custom_fields`, and optional `id`. |

**Rule**: Only the outbound copy is rewritten. `{"id": N}` with no push of opening text still updates the existing Zendesk ticket and does not send the text.

## Media address

One `http://` or `https://` span inside the outbound text.

| Field | Meaning |
| --- | --- |
| Candidate | The URL span, with trailing `. , ; : ! ? ) ] }` held aside. |
| Bucket | Parsed only by `presign.DurableURL`. Known when it is the media bucket or a bucket in the signed-bucket list. |
| Outcome | `durable`, `presigned-fallback`, or `unchanged`. |

**Rules**:

- Known bucket and durable links configured → replace the candidate with the durable link. Drop any temporary signature.
- Known bucket and durable links unavailable, signing available → replace with the short-lived signed address (`presigned-fallback`).
- Known bucket and no signer → leave the candidate unchanged.
- Unknown host, public link, or address that is already durable → unchanged.
- Every occurrence is evaluated. Surrounding characters stay.

## Known media bucket

Not a new record. Membership is the allow list already given to `presign.DurableURL`: `S3MediaBucket` plus the comma-separated `S3SignedBuckets`. `S3SessionBucket` is included only when it is also on that list.

## Zendesk external resource

The Any Channel object `Open` already pushes. This feature changes string values inside it, not the object shape.

| Field | Rewrite |
| --- | --- |
| `message` | Yes, when it contains a media address. |
| `fields[].value` | Yes, when the value is text copied from the JSON body (subject, description, message, custom field values). |
| `file_urls` | No. Open does not set this. Forward keeps today's file conversion. |

There is no status transition. An address is rewritten or left as-is at the moment the push body is built.
