FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/agent ./cmd/agent
COPY pkg ./pkg
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/infraos-agent ./cmd/agent

FROM alpine:3.21
RUN addgroup -S infraos && adduser -S -G infraos infraos && apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/infraos-agent /usr/local/bin/infraos-agent
RUN mkdir -p /var/lib/infraos-agent && chown -R infraos:infraos /var/lib/infraos-agent
USER infraos
ENTRYPOINT ["/usr/local/bin/infraos-agent"]
