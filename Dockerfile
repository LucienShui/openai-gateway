FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /gateway ./cmd/gateway

FROM alpine:latest

RUN apk --no-cache add ca-certificates wget

COPY --from=builder /gateway /gateway

EXPOSE 8000

ENTRYPOINT ["/gateway"]
