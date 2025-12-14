FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o generator cmd/generator/main.go cmd/generator/pipeline.go

FROM alpine:3.18
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/generator /app/generator

# Create Kratix directories
RUN mkdir -p /kratix/output

ENTRYPOINT ["/app/generator"]