FROM golang:1.17.5-alpine3.14

WORKDIR /app

RUN apk update \
  && apk add --virtual build-deps gcc git curl tar \
  && rm -rf /var/cache/apk/*

RUN addgroup -S golang \
  && adduser -S -G golang golang

COPY . .

RUN curl -L https://github.com/weni-ai/goflow/releases/download/$(grep weni-ai/goflow go.mod | cut -d" " -f5)/docs.tar.gz --output docs.tar.gz && tar -xf docs.tar.gz && rm docs.tar.gz

RUN go install -v ./cmd/...

EXPOSE 8000
ENTRYPOINT ["mailroom"]
