# Build stage
FROM golang:1.21-alpine AS builder

# Install git for go modules
RUN apk add --no-cache git

WORKDIR /workspace

# Copy go mod files (context is already infrastructure/platform-operator)
COPY go.mod go.sum ./
RUN go mod download

# Copy operator source code
COPY . ./

# Build the operator
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o manager cmd/manager/main.go

# Final stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the operator binary
COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]