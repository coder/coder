FROM golang:1.18-alpine

RUN go install github.com/dwahler/go-tools/cmd/godoc@v0.1.1

WORKDIR /go/src/github.com/coder/coder
ADD . .

EXPOSE 6060
CMD godoc -http :6060
