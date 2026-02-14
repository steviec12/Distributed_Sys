FROM golang:latest

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY *.go ./

RUN go build -o /product-api-server .

EXPOSE 8080

CMD ["/product-api-server"]