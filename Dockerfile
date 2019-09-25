FROM golang:1.12

WORKDIR /app
COPY . .

RUN curl https://codeload.github.com/nyaruka/goflow/tar.gz/$(grep goflow go.mod | cut -d" " -f2) | tar --wildcards --strip=1 -zx "*/docs/*"
RUN go build ./cmd/...
RUN chmod +x mailroom

EXPOSE 80
ENTRYPOINT ["./mailroom"]
