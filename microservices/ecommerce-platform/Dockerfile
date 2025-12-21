# Build stage
FROM node:20-alpine AS builder
WORKDIR /app

# Install dependencies
COPY ecommerce-platform/package*.json ./
RUN npm install

# Copy source code
COPY ecommerce-platform/ .

# Generate Prisma client and build
RUN npx prisma generate || true
RUN npm run build

# Production stage
FROM node:20-alpine
WORKDIR /app

ENV NODE_ENV=production

# Copy standalone output
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/public ./public

EXPOSE 3000

CMD ["node", "server.js"]