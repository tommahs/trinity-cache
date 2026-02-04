# Builder stage
FROM golang:1.21-alpine AS builder
WORKDIR /src

# Cache Go modules (handle missing go.sum)
COPY go.mod ./
RUN go env -w GOPROXY=https://proxy.golang.org,direct && go mod download

# Copy rest of the sources
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -o /out/trinity-cache ./cmd/trinity-cache

# Final image
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/trinity-cache /usr/local/bin/trinity-cache
ENTRYPOINT ["/usr/local/bin/trinity-cache"]
