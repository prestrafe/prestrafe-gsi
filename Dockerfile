FROM golang:alpine

# Create application directory
RUN mkdir /app
ADD . /app/
WORKDIR /app

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o run .

# Add the execution user
RUN adduser -S -D -H -h /app execuser
USER execuser

# Run the application
CMD ["./run"]
