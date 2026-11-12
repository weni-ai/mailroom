# Contract: Zendesk opening text media addresses

Outbound payload only. No new Mailroom HTTP route. The Zendesk Any Channel push (`any_channel/push.json`) keeps its current JSON shape. String values that carry the opening text may change.

## Input

`Open(session, topic, body, assignee, logHTTP)` body:

- plain conversation history, or
- a JSON ticket body whose text fields hold that history.

Media addresses inside that text may be:

- a direct object URL on a known media bucket,
- the same URL plus a temporary signature query,
- a public URL,
- a durable media link already.

## Output

The pushed `external_resources[0]` text (`message` and text `fields`) obeys all of the following:

1. Every known-bucket media address is a durable media link when durable links are configured.
2. A temporary signature on a known-bucket address is not present in the payload.
3. Public URLs and links that are already durable are byte-for-byte unchanged.
4. Characters that are not part of a replaced address are unchanged, including JSON structure.
5. When a durable link cannot be issued, the address follows `presign.DurableURL` (signed address if signing is configured, otherwise the original), and `Open` still returns success when the push succeeds.
6. A body that only reopens an existing ticket by id does not push opening text.
7. `Forward` payloads are unchanged.

## Non-goals

- Local ticket body storage.
- Zendesk `file_urls` and `convertAttachments`.
- Other ticketer types.
- New configuration keys.
