# Build stage
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

# Declare build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH

# Install git for go modules
RUN apk add --no-cache git curl

WORKDIR /workspace

# Copy go mod files (context is already infrastructure/platform-operator)
COPY go.mod go.sum ./
RUN go mod download

# Copy operator source code
COPY . ./

# Build the operator for the target architecture
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -installsuffix cgo -o manager cmd/manager/main.go

# Download Helm for the target architecture
RUN HELM_ARCH=${TARGETARCH} && \
    if [ "$TARGETARCH" = "arm64" ]; then HELM_ARCH="arm64"; fi && \
    curl -fsSL https://get.helm.sh/helm-v3.13.3-linux-${HELM_ARCH}.tar.gz | tar -xz && \
    mv linux-${HELM_ARCH}/helm /usr/local/bin/helm && \
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