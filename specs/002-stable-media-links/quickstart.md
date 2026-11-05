# Quickstart: Stable Media Links on Outbound Webhooks

Validate the webhook exit. Contract: [webhook-body.md](./contracts/webhook-body.md). The link rules are in [data-model.md](./data-model.md).

## Prerequisites

- Go 1.23.5
- The package tests mock HTTP. They do not call S3.
- Stable-link cases enable the test signer and link config with `testsuite.UseDurableLinks` (domain, secret, and the media bucket).

## Webhook body

```bash
go test ./core/goflow/ -count=1 -run 'TestSigningWebhook'
```

Expected:

- A JSON body field that holds a known-bucket file address leaves as `https://<test domain>/mr/media/<token>`, and the body is still valid JSON.
- A second address on another host is unchanged. Text around an address is unchanged.
- The same object, sent as an original address or as an expired signed address, produces one token.
- A form body field receives that stable link and remains a valid form. A form field is not given an address that contains `&`.
- An address that appears only in the request query string is unchanged.
- With no signer, the body is unchanged.

## Changelog

After the code change, `WENI-CHANGELOG.md` notes that webhook bodies carry the stable media link for known-location files.
