FROM golang:alpine

# Create application directory
RUN mkdir /src
ADD . /src/
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Build the application

RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /app/run .

# Add the execution user
RUN adduser -S -D -H -h /app execuser
USER execuser

# Run the application
CMD ["/app/run"]
