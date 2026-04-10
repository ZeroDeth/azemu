FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /azemu ./cmd/azemu

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /azemu /usr/local/bin/azemu
EXPOSE 4566 4567
ENTRYPOINT ["azemu"]
