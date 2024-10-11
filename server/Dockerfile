# Start from the official Go base image
FROM golang:1.23-alpine

RUN apk update && apk add --no-cache \
    file-dev \
    imagemagick \
    ghostscript

# Set the working directory inside the container
WORKDIR /app

# Copy the Go source code into the container
COPY . .

# Build the Go app
RUN go build -o myapp

# Expose port 8080 to the outside world
EXPOSE 3333

# Run the Go app
CMD ["./myapp"]