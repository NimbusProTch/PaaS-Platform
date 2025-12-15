FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go-platform-generator/go.mod go-platform-generator/go.sum ./
RUN go mod download

COPY go-platform-generator/ .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o generator cmd/main.go

FROM alpine:3.18
RUN apk --no-cache add ca-certificates git openssh

WORKDIR /app
COPY --from=builder /app/generator /app/generator

# Copy platform templates (for template-based generation)
COPY platform-templates /platform-templates

# Create Kratix directories
RUN mkdir -p /kratix/output

ENTRYPOINT ["/app/generator"]