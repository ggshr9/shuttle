# Shuttle sandbox container
FROM alpine:3.19

# Install dependencies
RUN apk add --no-cache \
    ca-certificates \
    iptables \
    iproute2 \
    curl \
    bind-tools \
    bash \
    procps

# Create shuttle user
RUN adduser -D -H shuttle

# Copy binaries
COPY sandbox/shuttle /usr/local/bin/
COPY sandbox/shuttled /usr/local/bin/
RUN chmod +x /usr/local/bin/shuttle /usr/local/bin/shuttled

# Copy config (will be overwritten by volume mount)
COPY sandbox/configs/ /etc/shuttle/

# Health check - verify shuttled process is running
HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=3 \
    CMD pgrep -x shuttled || exit 1

# Default command (overridden by docker-compose)
CMD ["shuttled", "run", "-c", "/etc/shuttle/server.yaml"]
