# syntax=docker/dockerfile:1

# --- Build stage ---
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-X main.Version=docker" -o /azemu ./cmd/azemu

# --- Runtime stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget

# Cert bundle directory; bind-mounted from host via docker-compose.
RUN mkdir -p /azemu
VOLUME /azemu

ENV AZEMU_CERT_PATH=/azemu/cert-bundle.pem
ENV AZEMU_METADATA_HOST=127.0.0.1:4567

COPY --from=build /azemu /usr/local/bin/azemu

EXPOSE 4566 4567 4568
ENTRYPOINT ["azemu"]
