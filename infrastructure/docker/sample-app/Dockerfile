FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY infrastructure/docker/sample-app/main.go ./main.go
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sample-app ./main.go

FROM alpine:3.21
RUN addgroup -S sample && adduser -S -G sample sample
COPY --from=builder /out/sample-app /usr/local/bin/sample-app
USER sample
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/sample-app"]
