FROM golang:1.25-alpine AS build
RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o server .

FROM alpine:latest
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /src/server .

EXPOSE 8080
ENTRYPOINT ["./server"]

