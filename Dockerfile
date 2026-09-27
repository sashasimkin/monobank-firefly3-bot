FROM golang:1.23.2 AS builder

# Install certificates
RUN apt-get update && apt-get install -y ca-certificates

WORKDIR /app

COPY go.mod go.sum .

RUN go mod download && go mod verify

# Copy project files
COPY . .

# Build
RUN make

FROM scratch

LABEL org.opencontainers.image.source="https://github.com/sashasimkin/monobank-firefly3-bot"

WORKDIR /app

COPY --from=builder /app/monobank-firefly3-bot /app/monobank-firefly3-bot
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder --chown=1000:1000 /app/logs /app/logs

USER 1000:1000

ENTRYPOINT ["/app/monobank-firefly3-bot"]
