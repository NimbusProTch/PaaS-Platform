# Build stage
FROM golang:1.21-alpine AS builder

# Install git for go modules
RUN apk add --no-cache git

WORKDIR /workspace

# Copy go mod files
COPY infrastructure/platform-operator/go.mod infrastructure/platform-operator/go.sum ./
RUN go mod download

# Copy operator source code
COPY infrastructure/platform-operator/ ./

# Build the operator
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o manager cmd/manager/main.go

# Final stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the operator binary
COPY --from=builder /workspace/manager .

# Copy charts from project root (build context is project root)
# Charts contain both microservice and platform templates
COPY charts/ /charts/

USER 65532:65532

ENTRYPOINT ["/manager"]