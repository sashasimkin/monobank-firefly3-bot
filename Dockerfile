FROM golang:1.23.2 AS builder

RUN apt-get update && apt-get install -y ca-certificates

WORKDIR /app

COPY go.mod go.sum .

RUN go mod download && go mod verify

# Copy project files
COPY . .

# Build
RUN make

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs
COPY --from=builder /app/monobank-firefly3-bot /app

ENTRYPOINT ["/app"]