# syntax=docker/dockerfile:1.7

# -------- Builder --------
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Install certificates in builder (so we can copy them later)
RUN apk add --no-cache ca-certificates

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build fully static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -trimpath \
      -ldflags="-s -w -buildid=" \
      -o /trinity-cache \
      ./cmd/trinity-cache

# -------- Final --------
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /trinity-cache /trinity-cache

# Run as distroless user
USER 65532:65532


ENTRYPOINT ["/trinity-cache"]
