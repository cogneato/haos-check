# HAOS Network Readiness Checker - Docker Version
# This runs the checks from inside a container to simulate HAOS networking

FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o haos-check .

FROM alpine:3.19

# Add ca-certificates for HTTPS and bind-tools for DNS debugging
RUN apk --no-cache add ca-certificates bind-tools

COPY --from=builder /app/haos-check /usr/local/bin/haos-check

# Run as non-root (similar to how Supervisor runs)
RUN adduser -D -u 1000 checker
USER checker

ENTRYPOINT ["/usr/local/bin/haos-check"]
