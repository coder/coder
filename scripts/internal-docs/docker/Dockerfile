FROM golang:1.18-alpine

RUN go install golang.org/x/tools/cmd/godoc@v0.1.10

WORKDIR /go/src/github.com/coder/coder
ADD . .

EXPOSE 6060
CMD godoc -http :6060
