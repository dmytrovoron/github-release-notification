FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/github-release-notification ./cmd

FROM alpine:latest

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app

COPY --from=builder /out/github-release-notification /app/github-release-notification
COPY migrations /app/migrations

USER app

EXPOSE 8080

ENV SCHEME=http
ENV HOST=0.0.0.0
ENV PORT=8080

ENTRYPOINT ["/app/github-release-notification"]
