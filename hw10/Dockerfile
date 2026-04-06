FROM golang:1.25 AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /hw10-node .

FROM scratch

COPY --from=build /hw10-node /hw10-node

EXPOSE 8080

ENTRYPOINT ["/hw10-node"]
