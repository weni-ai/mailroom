FROM golang:1.23-alpine3.20 AS builder

WORKDIR /app

RUN apk update && apk --no-cache add build-base curl gcc

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod/ \
    go mod download -x

RUN curl -L https://github.com/weni-ai/goflow/releases/download/$(grep weni-ai/goflow go.mod | cut -d" " -f5)/docs.tar.gz --output docs.tar.gz \
    && tar -xf docs.tar.gz \
    && rm docs.tar.gz

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod/ \
    go install -v ./cmd/...

FROM alpine:3.20

RUN apk add --no-cache tzdata

ENV APP_USER=app \
    APP_GROUP=app \
    USER_ID=11999 \
    GROUP_ID=11999

RUN addgroup --system --gid ${GROUP_ID} ${APP_GROUP} \
    && adduser --system --disabled-password --home /home/${APP_USER} \
    --uid ${USER_ID} --ingroup ${APP_GROUP} ${APP_USER}

COPY --from=builder --chown=${APP_USER}:${APP_GROUP} /go/bin/ /app/
COPY --from=builder --chown=${APP_USER}:${APP_GROUP} /app/docs /app/docs

WORKDIR /app

USER ${APP_USER}:${APP_GROUP}

EXPOSE 8000
ENTRYPOINT ["./mailroom"]
