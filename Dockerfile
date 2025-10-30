# --- Stage 1: The Builder ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy the dependency files first
COPY go.mod go.sum ./
# This is the cache-busting command
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the rest of your source code
COPY . .

# Build the Go app.
RUN CGO_ENABLED=0 go build -o /app/market-sim .

# --- Stage 2: The Final Image ---
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy the binary
COPY --from=builder /app/market-sim .

# Copy the index.html file
COPY --from=builder /app/index.html .

EXPOSE 8080
CMD ["./market-sim"]