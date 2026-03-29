FROM golang:1.25-alpine

WORKDIR /app

COPY . .

RUN go mod tidy && go build -o api .

EXPOSE 8080

CMD ["./api"]

