# Stage 1: Builder
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -v -o /app/service ./cmd/service

# Stage 2: Final image
FROM alpine:3.19

WORKDIR /app

# Copy the static binary from the builder stage
COPY --from=builder /app/service /app/service

# Copy migrations files
COPY ./migrations ./migrations

# Expose port if we were running an HTTP server (good practice to keep)
# EXPOSE 8080

# Command to run the application
CMD ["/app/service"]