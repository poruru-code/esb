# Builder Stage
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/agent ./cmd/agent

# Production Stage
FROM alpine:3.19 AS prod

WORKDIR /app

# Install CA certificates
RUN apk --no-cache add ca-certificates

COPY --from=builder /app/agent /app/agent

ENV PORT=50051

EXPOSE ${PORT}

CMD ["/app/agent"]
