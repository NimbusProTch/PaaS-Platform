# Build stage
FROM golang:1.21-alpine AS builder

# Install git for go modules
RUN apk add --no-cache git curl

WORKDIR /workspace

# Copy go mod files (context is already infrastructure/platform-operator)
COPY go.mod go.sum ./
RUN go mod download

# Copy operator source code
COPY . ./

# Build the operator
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o manager cmd/manager/main.go

# Download Helm
RUN curl -fsSL https://get.helm.sh/helm-v3.13.3-linux-amd64.tar.gz | tar -xz && \
    mv linux-amd64/helm /usr/local/bin/helm && \
    chmod +x /usr/local/bin/helm

# Final stage
FROM gcr.io/distroless/base:nonroot

WORKDIR /

# Copy the operator binary
COPY --from=builder /workspace/manager .
# Copy helm binary
COPY --from=builder /usr/local/bin/helm /usr/local/bin/helm

USER 65532:65532

ENTRYPOINT ["/manager"]