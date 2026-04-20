FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/app ./cmd/app

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/bin/app .
COPY resources /app/resources
EXPOSE 8085
CMD ["./app", "-c", "/app/configs/config.yaml"]
