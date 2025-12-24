# Build stage
FROM node:20-slim AS builder
WORKDIR /app

# Install OpenSSL for Prisma
RUN apt-get update -y && apt-get install -y openssl && rm -rf /var/lib/apt/lists/*

# Install dependencies
COPY ecommerce-platform/package*.json ./
RUN npm install

# Copy source code
COPY ecommerce-platform/ .

# Generate Prisma client and build
RUN npx prisma generate || true
RUN npm run build

# Production stage
FROM node:20-slim
WORKDIR /app

# Install OpenSSL for Prisma
RUN apt-get update -y && apt-get install -y openssl && rm -rf /var/lib/apt/lists/*

ENV NODE_ENV=production

# Copy standalone output
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/public ./public

EXPOSE 3000

CMD ["node", "server.js"]