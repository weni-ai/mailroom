# Contract: Outbound Webhook Body

Mailroom is the caller. The external system is the receiver. Only the body changes. Method, URL, query string, and headers are unchanged.

## Request

- **JSON** (`application/json`, `*+json`, or a body that is valid JSON when the content type is empty): every `http` or `https` address inside a string value is passed through `presign.DurableURL`.
- **Form** (`application/x-www-form-urlencoded`): every address inside a field value is passed through `presign.DurableURL`. The result is written back only when it contains no raw `&`. Otherwise that address stays as it was. The field is re-encoded.
- **Other bodies**, including multipart and a body that is the file itself: unchanged.
- **Query string**: unchanged, even when it contains a known-location file address.

## Address results

| Input address | Body result |
| --- | --- |
| Known-location object, secret configured | `https://<domain>/mr/media/<token>` for that object |
| Same object, already the stable link or a short-lived signed address | The same stable link |
| Any other address | The original address |
| Unreadable address | The original address, and the call still goes out |

Two calls that mention the same known object carry the same token.

## Receiver guarantee

The receiver can retrieve the file later with the stable link alone. The stored conversation still has the original address.

## Failure

A body that cannot be read fails the webhook call, as it does today. A single unreadable address does not.
