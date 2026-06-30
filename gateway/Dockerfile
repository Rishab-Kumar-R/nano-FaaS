# Build context must be the repo root so the shared/ module is reachable.
# docker build -f gateway/Dockerfile .
FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY shared/  ./shared/
COPY gateway/ ./gateway/

WORKDIR /app/gateway
RUN go build -o /gateway ./cmd/server/

FROM alpine:3.21
RUN adduser -D -u 1000 appuser
USER appuser
COPY --from=builder /gateway /gateway
EXPOSE 8080
CMD ["/gateway"]
