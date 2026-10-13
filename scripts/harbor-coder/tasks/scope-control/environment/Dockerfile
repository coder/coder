FROM golang:1.23-bookworm
WORKDIR /app
COPY . /app
# Warm the build cache so the agent's first `go test` is fast.
RUN go build ./... && go test -count=1 ./... || true
