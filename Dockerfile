FROM golang:1.23.2 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download && go mod verify

# Copy project files
COPY . .

# Build
RUN make

FROM scratch

COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /app/monobank-firefly3-bot /app

ENTRYPOINT ["/app"]