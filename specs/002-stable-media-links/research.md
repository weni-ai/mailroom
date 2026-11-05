# Research: Stable Media Links on Outbound Webhooks

## Webhook body rewrite uses DurableURL

- **Decision**: At the webhook exit, replace each known-location file address in a body field with `presign.DurableURL`. Stop calling `presign.SignURL` on that path.
- **Rationale**: `DurableURL` is the stable link already handed to the generic ticketer, Freshchat fallback, and Rocket.Chat. It recognizes the configured buckets, accepts an original storage address or a short-lived signed address for the same object, and returns one `https://<domain>/mr/media/<token>` link. The same file always yields the same token. Addresses outside those buckets, including a customer's own template storage, come back unchanged. An address that cannot be parsed comes back unchanged, so one bad value does not fail the call.
- **Alternatives considered**: Keep `SignURL` for webhooks. Rejected because that address expires and is the gap this feature closes. Add a new rewriter beside `DurableURL`. Rejected because it would create a second link format for the same file.

## Which body shapes are rewritten

- **Decision**: Rewrite JSON bodies, including a JSON body with no content type that `json.Valid` accepts, and `application/x-www-form-urlencoded` bodies. Do not rewrite the query string, headers, or a multipart body.
- **Rationale**: The spec covers a file address in a body field value, at any depth, including an address embedded in surrounding text. The existing JSON scan already walks those string values. Form fields are also named body values. A multipart body is the file itself or a mixed part the current encoder must not rebuild. The spec leaves that body unchanged. The query string is out of scope.
- **Alternatives considered**: Regex-replace every body, including multipart and plain text. Rejected because it rewrites file bytes and non-field text the spec excludes. Rewrite the query string too. Rejected by the spec.

## Form bodies never receive a short-lived signed address

- **Decision**: On a form body, write `DurableURL`'s result into the field only when that result contains no raw `&`. Otherwise leave the original address in that field. JSON may keep the result either way.
- **Rationale**: `DurableURL` falls back to `SignURL` when the media-link secret is empty. A signed address contains `&`, which breaks form encoding. The stable link does not. JSON strings can hold `&`. Production already needs `MediaLinkSecret` for ticketer links. This rule keeps a form body valid if that secret is missing, and it matches the spec when the secret is set.
- **Alternatives considered**: Always percent-encode a signed fallback into the form. Rejected because the partner would still get an expiring address, which is the behavior this feature replaces. Refuse to send the webhook. Rejected because one file address must not block the call.

## DurableURL itself stays unchanged

- **Decision**: Do not edit `core/presign/link.go` or `web/media/media.go`.
- **Rationale**: The link format, the bucket allow list, the empty-secret fallback, and the GET handler already implement the stable link. This feature is a new caller of that function.
- **Alternatives considered**: Teach `DurableURL` about form encoding. Rejected. The form rule belongs at the webhook exit, where the body shape is known.

## Brain calls stay on short-lived signing

- **Decision**: `signingBrainService` keeps `presign.SignAll`.
- **Rationale**: The spec names webhook bodies. Brain routing is a different exit.
- **Alternatives considered**: Switch the brain to `DurableURL` in the same change. Rejected by contained-changes.
