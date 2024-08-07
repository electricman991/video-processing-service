# Use the official Golang image to create a build artifact.
FROM golang:1.22 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod ./
COPY go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code into the container
COPY . .

# Set environment variables for static linking
ENV CGO_ENABLED=0 GOOS=linux

# Build the Go app
RUN go build -o video-processing-service

# Use a minimal base image to package the Go application
FROM alpine:3.20

# Install certificates to make HTTPS requests
RUN apk --no-cache add ca-certificates ffmpeg

# Copy the binary from the builder stage
COPY --from=builder /app/video-processing-service /video-processing-service

RUN chmod +x /video-processing-service

EXPOSE 8080

# Command to run the executable
CMD ["/video-processing-service"]
