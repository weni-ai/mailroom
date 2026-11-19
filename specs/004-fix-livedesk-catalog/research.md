# Research: Live Desk catalog reaches the customer

## 1. Where the customer-facing wording is stored

**Decision**: Keep plain `msgs_msg.text` empty on a Weni Chats catalog reply. Store the agent wording in `metadata.body` and, when the agent sent a title, store that title in `metadata.header`. Leave `header_text` and `header_type` in place.

**Rationale**: Delivery sends `msgs_msg.text` as one or more plain messages, then builds the catalog from `metadata.body` and `metadata.header`. The reported failure is an empty body plus a plain text message that succeeded. The Manager catalog path already clears plain text and fills `body` and `header` (`newOutgoingMsgWpp` when products or `send_catalog` are set and the body is non-empty). Commit `c43c51f` applies that split in the ticket callback. The Chats room already stores the agent text before this callback, so clearing the outgoing plain text does not hide the wording from the agent.

**Alternatives considered**:

- Leave the wording in `msgs_msg.text` and teach delivery to fall back to it. Rejected. That changes every channel and every flow, and the spec keeps delivery as it is.
- Fill `metadata.body` and also keep `msgs_msg.text`. Rejected. The customer would still receive a separate plain message before the catalog.

## 2. Empty and oversized body

**Decision**: One helper in the Weni Chats package, `catalogReplyBody(agentText, title string) (string, error)`, with `catalogBodyMaxRunes = 1024`.

Rules, in order:

1. Trim the agent text. Media-link rewriting has already run on that string.
2. Trim the title. A whitespace-only title counts as no title.
3. If the agent text is empty, use the title.
4. If the result is still empty, return an error. The callback responds 400 `catalog reply requires body text` and does not queue a message.
5. If the result has more than 1024 Unicode characters, keep the first 1024. Do not add an ellipsis and do not send the rest as a second message.

The title stored in `metadata.header` and in the broadcast header is the trimmed title, not the truncated body. This feature does not add a length policy for titles or footers.

**Rationale**: The channel rejects an interactive catalog whose body is missing or longer than 1024 characters. Counting runes matches that character limit; a byte cut would split accented characters and could still exceed the limit. The limit is a named constant because it is a channel rule, not an arbitrary literal. Falling back to the title satisfies the spec without inventing a stock sentence. Rejecting when both are empty avoids a 200 that delivery then fails with a 400 from the channel.

**Alternatives considered**:

- Truncate by bytes. Rejected. The channel limit is characters, and agent text is often Portuguese.
- Append an ellipsis inside the 1024 characters. Rejected. The spec says the body contains the first 1024 characters.
- Split the overflow into a follow-up plain message. Rejected. That recreates the double delivery this defect removes.
- Use a fixed sentence when both text and title are empty. Rejected. The spec refuses invented copy.

## 3. Whole-shop catalog with no product rows

**Decision**: In `CreateBroadcastMessages`, treat `send_catalog` as content. The empty check becomes: skip the contact only when plain text, attachments, product rows, and `send_catalog` are all absent. The Weni Chats callback, after a successful helper result, returns 400 `catalog reply was not created` if `SendReplyWithCatalog` returns no message.

**Rationale**: The callback clears plain text on purpose. Today the builder then returns no message when there are also no product rows, and the callback still responds `handled`. A whole-shop catalog is `send_catalog` without product rows. Counting `send_catalog` as content creates that message. The nil check covers the remaining cases (inactive contact, no sendable channel) so the callback cannot report success when nothing was queued. Text and media replies do not go through this nil check.

Flow catalog builders are not modified. Existing broadcast tests that send a catalog with plain text or with product rows still create the same number of messages. The new messages are only broadcasts that already set `send_catalog` and were previously dropped because they had neither plain text nor product rows.

**Alternatives considered**:

- Pass a placeholder plain text so the current guard keeps the message. Rejected. Delivery would send that placeholder as a separate text message.
- Change the guard for every carousel flag as well. Rejected. A carousel flag without products is not the whole-shop catalog, and Desk Copilot carousel payloads include product rows. A catalog callback that still produces no row fails with 400 instead of `handled`.
- Only special-case the ticket sender and leave the shared guard. Rejected. The guard is what drops the message. The ticket sender cannot both clear plain text and force a row without this condition.

## 4. What stays on the existing builders

**Decision**: Do not change `newOutgoingMsgCatalog`, `newOutgoingMsgWpp`, button-label defaulting, or carousel rendering. Keep writing `product_carousel` when the callback says `carousel: true`. Keep writing `action`, `footer`, `products`, `chats_msg_uuid`, and `response_to_external_id` when the callback sends them.

**Rationale**: Flow catalogs already reach the customer as one catalog message. The spec forbids changing that path. The channel delivers this attendance reply as a product list; carousel rendering is a delivery gap outside this repository's change. The Desk Copilot does not send a button label, and a product list does not require one. Footer, button label, product rows, the Chats message id, and the reply-to id are already copied onto the outgoing metadata and must survive the body change. Agent text is not evaluated as a flow expression on this path: the ticket broadcast is created as an already-evaluated template, so moving the wording into `metadata.body` does not start or stop expression evaluation.

**Alternatives considered**:

- Default the button label to the Manager wording ("Ver opções"). Rejected. The spec says not to invent a label.
- Stop persisting `product_carousel`. Rejected. That flag is part of the current payload, and this change does not redefine it.

## 5. Load, logs, and changelog

**Decision**: No new outbound call, queue, or config. Peak remains the current rate of catalog replies during human attendance. Error strings do not include the agent text, the title, a URN, or a phone number. Add a `fix:` line to `WENI-CHANGELOG.md` on the release that ships the change.

**Rationale**: The constitution requires a declared peak on a changed load path, forbids message content in logs and error reports, and requires a changelog entry for a user-visible messaging change.

**Alternatives considered**:

- Log the body length and the title whenever a catalog is truncated. Rejected. The title is message content. A count of catalog replies is not required for this fix.
