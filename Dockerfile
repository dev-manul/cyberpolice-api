FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./cmd/app

FROM alpine:3.20

WORKDIR /app

RUN adduser -D -H -s /sbin/nologin appuser

COPY --from=builder /out/app /app/app
COPY GeoLite2-City.mmdb /app/GeoLite2-City.mmdb
COPY .env.example /app/.env.example

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/app"]
