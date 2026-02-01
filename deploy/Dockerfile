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
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /shuttled /shuttled

EXPOSE 443/tcp 443/udp

ENTRYPOINT ["/shuttled", "run", "-c", "/etc/shuttle/server.yaml"]
