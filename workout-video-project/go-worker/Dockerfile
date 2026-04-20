FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates ffmpeg \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/worker /app/worker

USER app

ENTRYPOINT ["/app/worker"]
