# Fase di build
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o blockchain .

FROM gcr.io/distroless/base-debian11:nonroot

WORKDIR /app

COPY --from=builder --chown=nonroot:nonroot /app/blockchain .
COPY --from=builder --chown=nonroot:nonroot /app/config ./config

USER nonroot
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s \
  CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["./blockchain"]
