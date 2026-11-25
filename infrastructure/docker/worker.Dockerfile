FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
COPY database ./database
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/infraos-worker ./cmd/worker

FROM alpine:3.21
RUN addgroup -S infraos && adduser -S -G infraos infraos && apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/infraos-worker /usr/local/bin/infraos-worker
USER infraos
ENTRYPOINT ["/usr/local/bin/infraos-worker"]
