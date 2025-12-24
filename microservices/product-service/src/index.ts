import express, { Request, Response } from 'express';
import { Pool } from 'pg';
import Redis from 'ioredis';
import amqp from 'amqplib';
import { v4 as uuidv4 } from 'uuid';
import promClient from 'prom-client';
import winston from 'winston';
import LokiTransport from 'winston-loki';
import { trace, context, SpanStatusCode } from '@opentelemetry/api';
import { NodeTracerProvider } from '@opentelemetry/sdk-trace-node';
import { registerInstrumentations } from '@opentelemetry/instrumentation';
import { HttpInstrumentation } from '@opentelemetry/instrumentation-http';
import { ExpressInstrumentation } from '@opentelemetry/instrumentation-express';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-grpc';
import { BatchSpanProcessor } from '@opentelemetry/sdk-trace-base';
import { Resource } from '@opentelemetry/resources';
import { SemanticResourceAttributes } from '@opentelemetry/semantic-conventions';

// Initialize OpenTelemetry
const provider = new NodeTracerProvider({
  resource: new Resource({
    [SemanticResourceAttributes.SERVICE_NAME]: 'product-service',
    [SemanticResourceAttributes.SERVICE_VERSION]: '1.0.0',
  }),
});

const exporter = new OTLPTraceExporter({
  url: process.env.OTEL_EXPORTER_OTLP_ENDPOINT || 'http://localhost:4317',
});

provider.addSpanProcessor(new BatchSpanProcessor(exporter));
provider.register();

registerInstrumentations({
  instrumentations: [
    new HttpInstrumentation(),
    new ExpressInstrumentation(),
  ],
});

const tracer = trace.getTracer('product-service');

// Initialize Loki logging
const logger = winston.createLogger({
  transports: [
    new winston.transports.Console({
      format: winston.format.simple(),
    }),
    new LokiTransport({
      host: process.env.LOKI_HOST || 'http://localhost:3100',
      labels: {
        service: 'product-service',
        environment: process.env.ENVIRONMENT || 'development',
      },
    }),
  ],
});

// Prometheus metrics
const register = new promClient.Registry();
promClient.collectDefaultMetrics({ register });

const httpRequestDuration = new promClient.Histogram({
  name: 'http_request_duration_seconds',
  help: 'Duration of HTTP requests in seconds',
  labelNames: ['method', 'route', 'status'],
  registers: [register],
});

const dbQueryDuration = new promClient.Histogram({
  name: 'db_query_duration_seconds',
  help: 'Duration of database queries in seconds',
  labelNames: ['operation', 'table'],
  registers: [register],
});

// Product interface
interface Product {
  id: string;
  sku: string;
  name: string;
  description: string;
  category: string;
  price: number;
  currency: string;
  stock: number;
  images: string[];
  attributes: Record<string, any>;
  active: boolean;
  created_at: Date;
  updated_at: Date;
}

class ProductService {
  private db: Pool;
  private redis: Redis;
  private rabbitChannel: amqp.Channel | null = null;

  constructor() {
    // PostgreSQL connection
    this.db = new Pool({
      host: process.env.DB_HOST || 'localhost',
      port: parseInt(process.env.DB_PORT || '5432'),
      database: process.env.DB_NAME || 'products_db',
      user: process.env.DB_USER || 'postgres',
      password: process.env.DB_PASSWORD || 'postgres',
    });

    // Redis connection
    this.redis = new Redis({
      host: process.env.REDIS_HOST || 'localhost',
      port: parseInt(process.env.REDIS_PORT || '6379'),
      password: process.env.REDIS_PASSWORD,
    });

    // RabbitMQ connection
    this.connectRabbitMQ();

    // Initialize database
    this.initDB();
  }

  private async connectRabbitMQ() {
    try {
      const connection = await amqp.connect({
        hostname: process.env.RABBITMQ_HOST || 'localhost',
        port: parseInt(process.env.RABBITMQ_PORT || '5672'),
        username: process.env.RABBITMQ_USER || 'admin',
        password: process.env.RABBITMQ_PASSWORD || 'rabbitmq123',
      });

      this.rabbitChannel = await connection.createChannel();
      await this.rabbitChannel.assertExchange('product-events', 'topic', { durable: true });

      logger.info('Connected to RabbitMQ');
    } catch (error) {
      logger.error('Failed to connect to RabbitMQ:', error);
    }
  }

  private async initDB() {
    const createTableQuery = `
      CREATE TABLE IF NOT EXISTS products (
        id VARCHAR(36) PRIMARY KEY,
        sku VARCHAR(100) UNIQUE NOT NULL,
        name VARCHAR(255) NOT NULL,
        description TEXT,
        category VARCHAR(100),
        price DECIMAL(10, 2) NOT NULL,
        currency VARCHAR(3) DEFAULT 'USD',
        stock INTEGER DEFAULT 0,
        images TEXT[],
        attributes JSONB,
        active BOOLEAN DEFAULT true,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
      );

      CREATE INDEX IF NOT EXISTS idx_products_sku ON products(sku);
      CREATE INDEX IF NOT EXISTS idx_products_category ON products(category);
      CREATE INDEX IF NOT EXISTS idx_products_active ON products(active);
    `;

    try {
      await this.db.query(createTableQuery);
      logger.info('Database initialized');
    } catch (error) {
      logger.error('Failed to initialize database:', error);
    }
  }

  // CRUD Operations
  async listProducts(req: Request, res: Response) {
    const span = tracer.startSpan('listProducts');
    const timer = dbQueryDuration.startTimer({ operation: 'select', table: 'products' });

    try {
      const { category, active = true, limit = 100, offset = 0 } = req.query;

      // Try cache first
      const cacheKey = `products:${JSON.stringify(req.query)}`;
      const cached = await this.redis.get(cacheKey);

      if (cached) {
        span.setAttribute('cache.hit', true);
        span.end();
        timer();
        return res.json(JSON.parse(cached));
      }

      let query = 'SELECT * FROM products WHERE 1=1';
      const params: any[] = [];
      let paramCount = 0;

      if (category) {
        query += ` AND category = $${++paramCount}`;
        params.push(category);
      }

      if (active !== undefined) {
        query += ` AND active = $${++paramCount}`;
        params.push(active);
      }

      query += ` ORDER BY created_at DESC LIMIT $${++paramCount} OFFSET $${++paramCount}`;
      params.push(limit, offset);

      const result = await this.db.query(query, params);

      // Cache result
      await this.redis.set(cacheKey, JSON.stringify(result.rows), 'EX', 300);

      span.setStatus({ code: SpanStatusCode.OK });
      logger.info('Listed products', { count: result.rows.length });

      res.json(result.rows);
    } catch (error) {
      span.recordException(error as Error);
      span.setStatus({ code: SpanStatusCode.ERROR });
      logger.error('Failed to list products:', error);
      res.status(500).json({ error: 'Failed to list products' });
    } finally {
      span.end();
      timer();
    }
  }

  async getProduct(req: Request, res: Response) {
    const span = tracer.startSpan('getProduct');
    const { id } = req.params;
    span.setAttribute('product.id', id);

    try {
      // Check cache
      const cacheKey = `product:${id}`;
      const cached = await this.redis.get(cacheKey);

      if (cached) {
        span.setAttribute('cache.hit', true);
        span.end();
        return res.json(JSON.parse(cached));
      }

      const result = await this.db.query('SELECT * FROM products WHERE id = $1', [id]);

      if (result.rows.length === 0) {
        span.setStatus({ code: SpanStatusCode.ERROR, message: 'Product not found' });
        return res.status(404).json({ error: 'Product not found' });
      }

      const product = result.rows[0];

      // Cache result
      await this.redis.set(cacheKey, JSON.stringify(product), 'EX', 600);

      span.setStatus({ code: SpanStatusCode.OK });
      res.json(product);
    } catch (error) {
      span.recordException(error as Error);
      logger.error('Failed to get product:', error);
      res.status(500).json({ error: 'Failed to get product' });
    } finally {
      span.end();
    }
  }

  async createProduct(req: Request, res: Response) {
    const span = tracer.startSpan('createProduct');
    const timer = dbQueryDuration.startTimer({ operation: 'insert', table: 'products' });

    try {
      const product: Product = {
        id: uuidv4(),
        ...req.body,
        created_at: new Date(),
        updated_at: new Date(),
      };

      span.setAttributes({
        'product.id': product.id,
        'product.sku': product.sku,
        'product.name': product.name,
      });

      const query = `
        INSERT INTO products (
          id, sku, name, description, category, price, currency,
          stock, images, attributes, active, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
        RETURNING *
      `;

      const values = [
        product.id,
        product.sku,
        product.name,
        product.description,
        product.category,
        product.price,
        product.currency || 'USD',
        product.stock || 0,
        product.images || [],
        JSON.stringify(product.attributes || {}),
        product.active !== false,
        product.created_at,
        product.updated_at,
      ];

      const result = await this.db.query(query, values);
      const createdProduct = result.rows[0];

      // Publish event
      await this.publishEvent('product.created', createdProduct);

      // Invalidate cache
      await this.redis.del('products:*');

      span.setStatus({ code: SpanStatusCode.OK });
      logger.info('Product created', { productId: product.id, sku: product.sku });

      res.status(201).json(createdProduct);
    } catch (error) {
      span.recordException(error as Error);
      logger.error('Failed to create product:', error);
      res.status(500).json({ error: 'Failed to create product' });
    } finally {
      span.end();
      timer();
    }
  }

  async updateProduct(req: Request, res: Response) {
    const span = tracer.startSpan('updateProduct');
    const timer = dbQueryDuration.startTimer({ operation: 'update', table: 'products' });
    const { id } = req.params;

    try {
      span.setAttribute('product.id', id);

      const updates = { ...req.body, updated_at: new Date() };
      delete updates.id; // Don't update ID

      const setClause = Object.keys(updates)
        .map((key, index) => `${key} = $${index + 2}`)
        .join(', ');

      const query = `UPDATE products SET ${setClause} WHERE id = $1 RETURNING *`;
      const values = [id, ...Object.values(updates)];

      const result = await this.db.query(query, values);

      if (result.rows.length === 0) {
        return res.status(404).json({ error: 'Product not found' });
      }

      const updatedProduct = result.rows[0];

      // Publish event
      await this.publishEvent('product.updated', updatedProduct);

      // Invalidate cache
      await this.redis.del(`product:${id}`, 'products:*');

      span.setStatus({ code: SpanStatusCode.OK });
      logger.info('Product updated', { productId: id });

      res.json(updatedProduct);
    } catch (error) {
      span.recordException(error as Error);
      logger.error('Failed to update product:', error);
      res.status(500).json({ error: 'Failed to update product' });
    } finally {
      span.end();
      timer();
    }
  }

  async deleteProduct(req: Request, res: Response) {
    const span = tracer.startSpan('deleteProduct');
    const timer = dbQueryDuration.startTimer({ operation: 'delete', table: 'products' });
    const { id } = req.params;

    try {
      span.setAttribute('product.id', id);

      const result = await this.db.query(
        'DELETE FROM products WHERE id = $1 RETURNING id, sku',
        [id]
      );

      if (result.rows.length === 0) {
        return res.status(404).json({ error: 'Product not found' });
      }

      // Publish event
      await this.publishEvent('product.deleted', { id, sku: result.rows[0].sku });

      // Invalidate cache
      await this.redis.del(`product:${id}`, 'products:*');

      span.setStatus({ code: SpanStatusCode.OK });
      logger.info('Product deleted', { productId: id });

      res.json({ message: 'Product deleted successfully' });
    } catch (error) {
      span.recordException(error as Error);
      logger.error('Failed to delete product:', error);
      res.status(500).json({ error: 'Failed to delete product' });
    } finally {
      span.end();
      timer();
    }
  }

  // Search products with Elasticsearch integration
  async searchProducts(req: Request, res: Response) {
    const span = tracer.startSpan('searchProducts');
    const { q, category, minPrice, maxPrice } = req.query;

    try {
      let query = 'SELECT * FROM products WHERE active = true';
      const params: any[] = [];
      let paramCount = 0;

      if (q) {
        query += ` AND (name ILIKE $${++paramCount} OR description ILIKE $${paramCount})`;
        params.push(`%${q}%`);
      }

      if (category) {
        query += ` AND category = $${++paramCount}`;
        params.push(category);
      }

      if (minPrice) {
        query += ` AND price >= $${++paramCount}`;
        params.push(minPrice);
      }

      if (maxPrice) {
        query += ` AND price <= $${++paramCount}`;
        params.push(maxPrice);
      }

      const result = await this.db.query(query, params);

      span.setStatus({ code: SpanStatusCode.OK });
      logger.info('Products searched', { query: q, results: result.rows.length });

      res.json(result.rows);
    } catch (error) {
      span.recordException(error as Error);
      logger.error('Failed to search products:', error);
      res.status(500).json({ error: 'Failed to search products' });
    } finally {
      span.end();
    }
  }

  private async publishEvent(eventType: string, data: any) {
    if (!this.rabbitChannel) return;

    try {
      const message = Buffer.from(JSON.stringify({
        type: eventType,
        data,
        timestamp: new Date().toISOString(),
        service: 'product-service',
      }));

      await this.rabbitChannel.publish(
        'product-events',
        eventType,
        message,
        { persistent: true }
      );

      logger.info('Event published', { type: eventType });
    } catch (error) {
      logger.error('Failed to publish event:', error);
    }
  }
}

// Express app setup
const app = express();
app.use(express.json());

// Middleware for metrics
app.use((req, res, next) => {
  const timer = httpRequestDuration.startTimer({
    method: req.method,
    route: req.path,
  });

  res.on('finish', () => {
    timer({ status: res.statusCode.toString() });
  });

  next();
});

// Initialize service
const productService = new ProductService();

// Routes
app.get('/health', (req, res) => {
  res.json({ status: 'healthy', timestamp: new Date().toISOString() });
});

app.get('/ready', async (req, res) => {
  try {
    await productService['db'].query('SELECT 1');
    await productService['redis'].ping();
    res.json({ status: 'ready' });
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : 'Unknown error';
    res.status(503).json({ status: 'not ready', error: errorMessage });
  }
});

app.get('/metrics', async (req, res) => {
  res.set('Content-Type', register.contentType);
  res.end(await register.metrics());
});

// Product routes
const router = express.Router();
router.get('/products', productService.listProducts.bind(productService));
router.get('/products/search', productService.searchProducts.bind(productService));
router.get('/products/:id', productService.getProduct.bind(productService));
router.post('/products', productService.createProduct.bind(productService));
router.put('/products/:id', productService.updateProduct.bind(productService));
router.delete('/products/:id', productService.deleteProduct.bind(productService));

app.use('/api/v1', router);

// Start server
const PORT = process.env.PORT || 8080;
app.listen(PORT, () => {
  logger.info(`Product Service listening on port ${PORT}`);
});