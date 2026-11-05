# Data Model: Stable Media Links on Outbound Webhooks

No new stored entity and no schema change. The rewrite exists only on the outbound copy.

## Known location

A bucket already configured for stable links: the media bucket plus `S3SignedBuckets`. `presign.DurableURL` decides membership. This feature does not grow or shrink that list.

## File address

The address stored on a message attachment, or a short-lived signed form of that same object.

- Validation: if `DurableURL` cannot read it as a known object, the outbound copy keeps the original text.
- Not updated in PostgreSQL. `msgs_msg.attachments` stays the original address.

## Stable link

The string `DurableURL` returns for a known object: `https://<domain>/mr/media/<token>`.

- One file, one link, across webhook deliveries.
- A link that is already stable stays that link.
- A short-lived address of the same object becomes that link.
- The receiver retrieves the file with the existing `GET /mr/media/{token}` handler. This feature does not change that handler.

## Webhook body field

A named value in a JSON body or in an `application/x-www-form-urlencoded` body.

- A value may be the address itself or text that contains one or more addresses.
- Every known-location address in the value is replaced. Surrounding text and unknown addresses stay.
- Query string, headers, and multipart bodies are not fields for this rewrite.
- State: original body in memory, then the rewritten body on `Request.Body` and `GetBody`, then the call. Nothing is persisted.

## Relationships

- The stable link refers to one object in a known location. The media handler resolves it back to that object.
