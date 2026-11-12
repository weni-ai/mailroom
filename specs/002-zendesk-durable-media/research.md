# Research: Durable media links on Zendesk ticket opening

## 1. Where the rewrite runs

**Decision**: Rewrite a copy of the opening text inside Zendesk `Open`, after `flows.OpenTicket` stores the original body, and before the plain-text versus JSON split that builds the Any Channel push.

**Rationale**: The agent-facing defect is the text Zendesk receives (`message`, and the string fields copied from a JSON body: subject, description, message, custom field values). The local ticket keeps the original body, so the existing open assertion that `ticket.Body()` equals the flow payload stays true. The `{"id": N}` path still only calls ticket update and does not push opening text. `Forward` and other ticketers are not called.

**Alternatives considered**:

- Rewrite the body passed into `OpenTicket`. Rejected because it changes the stored ticket payload, which this defect does not ask for.
- Walk JSON fields after unmarshal and rewrite each string. Rejected because plain history is not JSON, and a second walk can miss a URL that only exists in the raw text. One rewrite of the outbound copy covers both shapes.
- Rewrite `Forward` text or `file_urls`. Rejected. Later Zendesk messages already convert attachments through the Zendesk file path, and the spec leaves that path alone.

## 2. How addresses are found

**Decision**: A single regular expression finds literal `http://` and `https://` spans in the outbound text. Each span is passed to `presign.DurableURL`. The regex only locates candidates. It does not decide whether a bucket is known.

**Rationale**: History is free text, or those URLs sit inside JSON string values. `DurableURL` already accepts one URL, parses it, and returns a durable link only for the configured media bucket plus `S3SignedBuckets`. That is the same function Rocket.Chat, Freshchat, and the generic ticketer use for attachments. The user confirmed the scan is a regex.

**Alternatives considered**:

- Classify buckets with a regex of host shapes. Rejected. Virtual-hosted, path-style, presigned, and custom-endpoint URLs are already handled by `DurableURL` / `ParseS3URL`. A second pattern would drift from that allow list.
- Split the text on whitespace and parse every token. Rejected. URLs in sentences and JSON pick up trailing punctuation and quotes; a URL pattern with an explicit trailing trim is easier to test.

## 3. What `DurableURL` already guarantees

**Decision**: Call `presign.DurableURL` unchanged. Do not reimplement signing, tokens, or the bucket list.

**Rationale**: With a media-link secret and domain configured, a URL whose bucket is in the allow list becomes `https://{domain}/mr/media/{token}`. Query strings, including temporary signatures, are dropped because the parser keeps bucket, key, and region only. A URL outside the allow list, including an address that is already durable, is returned unchanged. If the signer is missing, the original URL is returned. If the secret or domain is empty, `DurableURL` falls back to `SignURL` (a short-lived signed address when signing works, otherwise the original). None of these paths return an error, so ticket opening cannot fail because a link was not rewritten.

**Alternatives considered**:

- Presign in the Zendesk body and skip durable links. Rejected. Presigned addresses expire, which is the defect, and other ticketers already moved attachment sharing to durable links.
- Add `S3SessionBucket` to the allow list for this feature. Rejected. The session bucket is not on the durable-link allow list unless it is also listed in `S3SignedBuckets`. The spec uses that same set.

## 4. Keeping the surrounding text valid

**Decision**: The match stops at whitespace, quotes, and angle brackets. Trailing prose or JSON punctuation (`. , ; : ! ? ) ] }`) is split off the candidate before `DurableURL` and left in place after a replacement. If `DurableURL` returns the same string, the original span is kept, punctuation included.

**Rationale**: A history sentence and a JSON string both place characters after a URL that are not part of the object key. Replacing only the URL span leaves wording and JSON structure intact. Durable links and presigned fallbacks do not contain `"`, so a replacement inside a JSON string stays quoted. Slashes in `https://` are literal in the flow body; Go JSON does not escape them, so the pattern does not need to accept `https:\/\/`.

**Alternatives considered**:

- Replace the regex match with no trim. Rejected. A URL at the end of a sentence would swallow the period, and `DurableURL` would either miss the object or keep the punctuation when it returns the original.
- Parse and re-serialize JSON. Rejected. Plain bodies are not JSON, and re-serialization would reorder fields and change text that has no media URL.

## 5. Load, logs, and changelog

**Decision**: No new outbound call, queue, or stored state. One linear scan of the opening text per Zendesk open. Peak remains the current Zendesk ticket-open rate. Do not log the opening text or the URLs. Record a `fix:` line in `WENI-CHANGELOG.md` on the release that ships the change.

**Rationale**: The constitution requires a declared peak on a changed load path, forbids message text in logs, and requires a changelog entry for a user-visible ticketer change. This path only edits a string that `Open` already pushes.

**Alternatives considered**:

- Log each rewritten URL. Rejected. The URL can identify a contact's file, and presign errors already log on the existing signer path.
