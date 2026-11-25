FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
COPY database ./database
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/infraos-api ./cmd/api

FROM alpine:3.21
RUN addgroup -S infraos && adduser -S -G infraos infraos && apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=builder /out/infraos-api /usr/local/bin/infraos-api
RUN mkdir -p /app/data /app/backups && chown -R infraos:infraos /app
USER infraos
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/infraos-api"]
