FROM golang

WORKDIR /go/src/app
COPY . .

RUN go get -d -v github.com/go-telegram-bot-api/telegram-bot-api && \
    go get -d -v github.com/jlaffaye/ftp && \
    go install && \
    go clean && \
    cp config.json /go/bin/ && \
    rm -rf ./*

CMD ["/go/bin/app"]
