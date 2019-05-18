FROM golang:1.12 AS builder

WORKDIR $GOPATH/src/eis_search
COPY . .

RUN go get -d -v github.com/go-telegram-bot-api/telegram-bot-api && \
    go get -d -v github.com/jlaffaye/ftp && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o /eis_search && \
    go clean && \
    rm -rf ./*

FROM alpine:3.9 AS certs

RUN apk add -U --no-cache ca-certificates

FROM scratch

COPY --from=builder /eis_search /eis_search
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/eis_search"]
