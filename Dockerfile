# Start from the latest Go image
FROM golang:latest

# Set working directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY *.go ./

# Build the application
RUN go build -o /docker-gs-ping

# Expose port 8080
EXPOSE 8080

# Run the application
CMD ["/docker-gs-ping"]