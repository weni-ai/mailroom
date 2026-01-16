# Mailroom

[![Build Status](https://github.com/weni-ai/mailroom/workflows/CI/badge.svg)](https://github.com/weni-ai/mailroom/actions?query=workflow%3ACI)
[![codecov](https://codecov.io/gh/weni-ai/mailroom/branch/main/graph/badge.svg)](https://codecov.io/gh/weni-ai/mailroom)

Mailroom is the [RapidPro](https://github.com/rapidpro/rapidpro) worker which does the heavy lifting of
running flow starts, campaigns, messaging, tickets and other background tasks. It connects directly to the
RapidPro database and exchanges messages with [Courier](https://github.com/nyaruka/courier) via Redis.

This repository is the Weni-maintained fork used in production at Weni, based on the original
[`nyaruka/mailroom`](https://github.com/nyaruka/mailroom) project.

## Features

- High‑throughput execution of RapidPro flows and campaigns
- Background processing for messaging, IVR, webhooks, tickets and analytics
- Integration with PostgreSQL, Redis, ElasticSearch and S3
- Metrics and error reporting hooks for Prometheus, Librato and Sentry

## Quick start

You can run Mailroom either from source (Go toolchain) or using Docker.

### Running with Docker

The repository includes a minimal `docker-compose.yml`:

```bash
cd docker
docker compose up -d
```

By default this will build and run the image
`${DOCKER_IMAGE_NAME:-ilhasoft/mailroom}:${DOCKER_IMAGE_TAG:-latest}` and expose Mailroom on port `8000`.
Configure environment variables (see **Configuration** below) either in your Compose file or in your
container orchestration environment.

### Building from source

Prerequisites:

- Go `1.23` or newer
- PostgreSQL (for the RapidPro database)
- Redis
- ElasticSearch (optional, for search indexing)

Clone the repository and build:

```bash
go build github.com/nyaruka/mailroom/cmd/mailroom
```

This produces a `mailroom` binary (usually in `$GOPATH/bin` or the current directory, depending on how you run
`go build`).

## Configuration

Mailroom uses a tiered configuration system, each option taking precedence over the ones above it:

1. Configuration file
2. Environment variables starting with `MAILROOM_`
3. Command line parameters

In most deployments we recommend using only environment variables. Run:

```bash
mailroom --help
```

to see the full list of options and defaults.

### Core RapidPro settings

For use with RapidPro, configure at least:

- `MAILROOM_ADDRESS`: address to bind the web server to (default `"localhost"`)
- `MAILROOM_DOMAIN`: public domain that Mailroom is listening on
- `MAILROOM_AUTH_TOKEN`: token clients must use to authenticate web requests (must match RapidPro settings)
- `MAILROOM_ATTACHMENT_DOMAIN`: domain used for relative attachments in flows
- `MAILROOM_DB`: PostgreSQL URL for the RapidPro database (default
  `postgres://temba:temba@localhost/temba?sslmode=disable`)
- `MAILROOM_REDIS`: Redis URL (default `redis://localhost:6379/15`)
- `MAILROOM_ELASTIC`: ElasticSearch URL (default `http://localhost:9200`)
- `MAILROOM_SMTP_SERVER`: SMTP configuration for sending emails, e.g.
  `smtp://user%40password@server:port/?from=foo%40gmail.com`

### Media and session storage (S3)

For writing message attachments to S3:

- `MAILROOM_S3_REGION`: region for your S3 bucket (e.g. `eu-west-1`)
- `MAILROOM_S3_MEDIA_BUCKET`: S3 bucket name (e.g. `dl-mailroom`)
- `MAILROOM_S3_MEDIA_PREFIX`: prefix for attachment filenames (e.g. `attachments`)
- `MAILROOM_AWS_ACCESS_KEY_ID`: AWS access key ID
- `MAILROOM_AWS_SECRET_ACCESS_KEY`: AWS secret access key

For writing session data to S3:

- `MAILROOM_S3_SESSION_BUCKET`: S3 bucket name for sessions (e.g. `rp-sessions`)
- `MAILROOM_S3_SESSION_PREFIX`: prefix for session filenames (may be empty)

### Monitoring and logging

Recommended settings for error and performance monitoring:

- `MAILROOM_LIBRATO_USERNAME`: username for logging events to Librato
- `MAILROOM_LIBRATO_TOKEN`: token for logging events to Librato
- `MAILROOM_SENTRY_DSN`: DSN used when logging errors to Sentry
- `MAILROOM_LOG_LEVEL`: logging level (default `"error"`, use `"debug"` for more verbosity)

## Development

Create the test database:

```bash
createdb mailroom_test
createuser -P -E -s mailroom_test  # set no password
```

Run the full test suite (serially, as some tests expect isolation):

```bash
go test ./... -p=1
```

Useful additional resources:

- `docs/` – end‑user and operator documentation in multiple languages
- `WENI-CHANGELOG.md` – Weni‑specific changes on top of upstream Mailroom

## Contributing

Issues and pull requests are welcome. Please open them on the
[`weni-ai/mailroom` GitHub repository](https://github.com/weni-ai/mailroom).

Before submitting a PR:

- Ensure `go test ./... -p=1` passes
- Add or update tests where appropriate
- Follow existing code style and project conventions

## License

This current project is licensed under the GNU Affero General Public License v3.0 (AGPL‑3.0). See the `LICENSE` file for
full license text.
