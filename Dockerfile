# ---- Stage 1: Build ----
# Use an official Go image as the base image for the build environment.
# Using the alpine variant helps keep the intermediate image size smaller.
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container.
WORKDIR /app

# Copy the go.mod and go.sum files first. This allows Docker to cache
# the dependency layer, speeding up subsequent builds if dependencies haven't changed.
COPY go.mod go.sum ./

# Download the Go module dependencies.
RUN go mod download

# Copy the rest of your application's source code into the container.
COPY . .

# Build the Go application.
# CGO_ENABLED=0 creates a statically linked binary, which is necessary for
# running in a minimal container like distroless.
# -ldflags="-w -s" strips debug information, reducing the binary size.
RUN CGO_ENABLED=0 go build -o /app/server -ldflags="-w -s" .


# ---- Stage 2: Run ----
# Use a minimal "distroless" base image for the final container.
# This image contains only your application and its runtime dependencies,
# making it more secure and smaller than a full OS image.
FROM gcr.io/distroless/static-debian11

# Set the working directory.
WORKDIR /app

# Copy the compiled binary from the 'builder' stage.
COPY --from=builder /app/server .

# Expose the port that the application will listen on.
# Cloud Run will use this to route traffic to your container.
# Ensure this matches the PORT environment variable your app uses (default is 8080).
EXPOSE 8080

# The command to run when the container starts.
# This executes your compiled Go application.
CMD ["/app/server"]
