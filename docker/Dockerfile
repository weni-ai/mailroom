FROM golang:1.13.3-alpine3.10

WORKDIR /app

RUN apk update \
  && apk add --virtual build-deps gcc git \
  && rm -rf /var/cache/apk/*

RUN addgroup -S golang \
  && adduser -S -G golang golang

COPY . .

RUN go build ./cmd/...
RUN chmod +x mailroom
RUN chown -R golang /app

USER golang

EXPOSE 80
ENTRYPOINT ["./mailroom"]
