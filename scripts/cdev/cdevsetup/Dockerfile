FROM golang:1.25.6-alpine

WORKDIR /app

# Copy go.mod and go.sum for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY main.go ./

# Build the binary
RUN go build -o /cdevsetup main.go

ENTRYPOINT ["/cdevsetup", "remain"]
