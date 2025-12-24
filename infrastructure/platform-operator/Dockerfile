# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the operator
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -o manager cmd/manager/main.go

# Final stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the operator binary
COPY --from=builder /app/manager .

# Copy embedded charts from project root
# These charts will be available at /charts and /platform-charts in the container
COPY --from=builder /app/../../charts /charts
COPY --from=builder /app/../../platform-charts /platform-charts

USER 65532:65532

ENTRYPOINT ["/manager"]