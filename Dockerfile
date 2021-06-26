FROM golang:alpine

# Create application directory
RUN mkdir /src
ADD . /src/
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

# Build the application
COPY go.* .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    --mount=type=cache,target=/go/pkg/mod \
    go mod graph | awk '{if ($1 !~ "@") print $2}' | xargs -r go get

RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /app/run .

# Add the execution user
RUN adduser -S -D -H -h /app execuser
USER execuser

# Run the application
CMD ["/app/run"]
