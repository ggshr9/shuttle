# Stage 1: Build
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
COPY quicfork/ quicfork/
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /shuttled ./cmd/shuttled

# Stage 2: Runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -s /sbin/nologin shuttle

COPY --from=builder /shuttled /usr/local/bin/shuttled

USER shuttle
EXPOSE 443/tcp 443/udp

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD ["/usr/local/bin/shuttled", "version"]

ENTRYPOINT ["/usr/local/bin/shuttled", "run", "-c", "/etc/shuttle/server.yaml"]
