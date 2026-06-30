# Build context must be the repo root so the shared/ module is reachable.
# docker build -f worker/Dockerfile .
FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY shared/ ./shared/
COPY worker/ ./worker/

WORKDIR /app/worker
RUN go build -o /worker ./cmd/runner/

FROM alpine:3.21
RUN adduser -D -u 1000 appuser
USER appuser
COPY --from=builder /worker /worker
CMD ["/worker"]
