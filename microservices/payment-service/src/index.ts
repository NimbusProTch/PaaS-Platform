import express, { Request, Response, NextFunction } from 'express';
import cors from 'cors';
import helmet from 'helmet';
import { PrismaClient } from '@prisma/client';
import Stripe from 'stripe';
import Redis from 'ioredis';
import amqp from 'amqplib';
import * as OpenTelemetry from '@opentelemetry/api';
import { NodeTracerProvider } from '@opentelemetry/sdk-trace-node';
import { registerInstrumentations } from '@opentelemetry/instrumentation';
import { HttpInstrumentation } from '@opentelemetry/instrumentation-http';
import { ExpressInstrumentation } from '@opentelemetry/instrumentation-express';
import { Resource } from '@opentelemetry/resources';
import { SemanticResourceAttributes } from '@opentelemetry/semantic-conventions';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-grpc';
import { BatchSpanProcessor } from '@opentelemetry/sdk-trace-base';
import { PrometheusExporter } from '@opentelemetry/exporter-prometheus';
import { MeterProvider } from '@opentelemetry/sdk-metrics';
import winston from 'winston';
import LokiTransport from 'winston-loki';

// Environment variables
const PORT = process.env.PORT || 8083;
const STRIPE_SECRET_KEY = process.env.STRIPE_SECRET_KEY || 'sk_test_dummy';
const STRIPE_WEBHOOK_SECRET = process.env.STRIPE_WEBHOOK_SECRET || '';
const DATABASE_URL = process.env.DATABASE_URL || 'postgresql://payment_user:payment_pass@localhost:5432/payment_db';
const REDIS_URL = process.env.REDIS_URL || 'redis://localhost:6379';
const AMQP_URL = process.env.AMQP_URL || 'amqp://guest:guest@localhost:5672';
const LOKI_HOST = process.env.LOKI_HOST || 'http://localhost:3100';
const JAEGER_ENDPOINT = process.env.JAEGER_ENDPOINT || 'localhost:4317';

// Initialize logger
const logger = winston.createLogger({
  level: 'info',
  format: winston.format.json(),
  transports: [
    new winston.transports.Console({
      format: winston.format.simple()
    })
  ]
});

// Add Loki transport in production
if (process.env.NODE_ENV === 'production') {
  logger.add(
    new LokiTransport({
      host: LOKI_HOST,
      labels: {
        service: 'payment-service',
        environment: process.env.ENVIRONMENT || 'production'
      }
    })
  );
}

// Initialize OpenTelemetry
const resource = new Resource({
  [SemanticResourceAttributes.SERVICE_NAME]: 'payment-service',
  [SemanticResourceAttributes.SERVICE_VERSION]: '1.0.0',
});

const provider = new NodeTracerProvider({
  resource,
});

const exporter = new OTLPTraceExporter({
  url: `http://${JAEGER_ENDPOINT}`,
});

provider.addSpanProcessor(new BatchSpanProcessor(exporter));
provider.register();

registerInstrumentations({
  instrumentations: [
    new HttpInstrumentation(),
    new ExpressInstrumentation(),
  ],
});

const tracer = OpenTelemetry.trace.getTracer('payment-service');

// Initialize Prometheus metrics
const prometheusExporter = new PrometheusExporter(
  {
    port: 9090,
    endpoint: '/metrics',
  },
  () => {
    logger.info('Prometheus metrics server started on port 9090');
  }
);

const meterProvider = new MeterProvider({
  readers: [prometheusExporter as any],
  resource,
});

const meter = meterProvider.getMeter('payment-service');

// Define metrics
const httpRequestDuration = meter.createHistogram('http_request_duration', {
  description: 'Duration of HTTP requests in seconds',
});

const paymentCounter = meter.createCounter('payments_total', {
  description: 'Total number of payments',
});

const paymentAmount = meter.createHistogram('payment_amount', {
  description: 'Payment amount distribution',
});

// Initialize Stripe
const stripe = new Stripe(STRIPE_SECRET_KEY, {
  apiVersion: '2023-10-16',
});

// Initialize Prisma
const prisma = new PrismaClient();

// Initialize Redis
const redis = new Redis(REDIS_URL);

// RabbitMQ connection
let amqpChannel: amqp.Channel | null = null;

async function connectRabbitMQ() {
  try {
    const connection = await amqp.connect(AMQP_URL);
    amqpChannel = await connection.createChannel();

    await amqpChannel.assertExchange('payments', 'topic', { durable: true });

    logger.info('Connected to RabbitMQ');
  } catch (error) {
    logger.error('Failed to connect to RabbitMQ:', error);
  }
}

// Express app
const app = express();

// Middleware
app.use(helmet());
app.use(cors());
app.use(express.json());
app.use(express.raw({ type: 'application/json' })); // For Stripe webhook

// Request timing middleware
app.use((req: Request, res: Response, next: NextFunction) => {
  const start = Date.now();

  res.on('finish', () => {
    const duration = (Date.now() - start) / 1000;
    httpRequestDuration.record(duration, {
      method: req.method,
      route: req.route?.path || req.path,
      status_code: res.statusCode,
    });
  });

  next();
});

// Health check
app.get('/health', (req: Request, res: Response) => {
  res.json({ status: 'healthy' });
});

// Ready check
app.get('/ready', async (req: Request, res: Response) => {
  try {
    await prisma.$queryRaw`SELECT 1`;
    await redis.ping();
    res.json({ status: 'ready' });
  } catch (error) {
    res.status(503).json({ status: 'not ready', error: error.message });
  }
});

// Payment endpoints

// Create payment intent
app.post('/api/v1/payments/intent', async (req: Request, res: Response) => {
  const span = tracer.startSpan('create_payment_intent');

  try {
    const { amount, currency, orderId, userId, metadata } = req.body;

    // Validate input
    if (!amount || amount <= 0) {
      return res.status(400).json({ error: 'Invalid amount' });
    }

    // Create Stripe payment intent
    const paymentIntent = await stripe.paymentIntents.create({
      amount: Math.round(amount * 100), // Convert to cents
      currency: currency || 'usd',
      metadata: {
        orderId,
        userId,
        ...metadata
      }
    });

    // Store payment in database
    const payment = await prisma.payment.create({
      data: {
        id: paymentIntent.id,
        orderId,
        userId,
        amount,
        currency: currency || 'usd',
        status: 'pending',
        stripePaymentIntentId: paymentIntent.id,
        metadata: metadata || {}
      }
    });

    // Cache payment data
    await redis.set(
      `payment:${payment.id}`,
      JSON.stringify(payment),
      'EX',
      600 // 10 minutes
    );

    // Send event
    await publishEvent('payment.intent.created', payment);

    // Track metrics
    paymentCounter.add(1, { status: 'pending', method: 'stripe' });
    paymentAmount.record(amount, { currency: currency || 'usd' });

    span.setStatus({ code: OpenTelemetry.SpanStatusCode.OK });

    res.json({
      clientSecret: paymentIntent.client_secret,
      paymentId: payment.id
    });

  } catch (error) {
    logger.error('Failed to create payment intent:', error);
    span.setStatus({
      code: OpenTelemetry.SpanStatusCode.ERROR,
      message: error.message
    });
    res.status(500).json({ error: 'Failed to create payment intent' });
  } finally {
    span.end();
  }
});

// Confirm payment
app.post('/api/v1/payments/:id/confirm', async (req: Request, res: Response) => {
  const span = tracer.startSpan('confirm_payment');

  try {
    const { id } = req.params;
    const { paymentMethodId } = req.body;

    // Get payment from cache or database
    let payment = await getPayment(id);
    if (!payment) {
      return res.status(404).json({ error: 'Payment not found' });
    }

    // Confirm payment with Stripe
    const paymentIntent = await stripe.paymentIntents.confirm(
      payment.stripePaymentIntentId,
      {
        payment_method: paymentMethodId
      }
    );

    // Update payment status
    payment = await prisma.payment.update({
      where: { id },
      data: {
        status: paymentIntent.status === 'succeeded' ? 'completed' : 'failed',
        completedAt: paymentIntent.status === 'succeeded' ? new Date() : null
      }
    });

    // Update cache
    await redis.set(
      `payment:${payment.id}`,
      JSON.stringify(payment),
      'EX',
      600
    );

    // Send event
    await publishEvent(`payment.${payment.status}`, payment);

    // Track metrics
    paymentCounter.add(1, { status: payment.status, method: 'stripe' });

    span.setStatus({ code: OpenTelemetry.SpanStatusCode.OK });

    res.json(payment);

  } catch (error) {
    logger.error('Failed to confirm payment:', error);
    span.setStatus({
      code: OpenTelemetry.SpanStatusCode.ERROR,
      message: error.message
    });
    res.status(500).json({ error: 'Failed to confirm payment' });
  } finally {
    span.end();
  }
});

// Get payment
app.get('/api/v1/payments/:id', async (req: Request, res: Response) => {
  const span = tracer.startSpan('get_payment');

  try {
    const { id } = req.params;

    const payment = await getPayment(id);
    if (!payment) {
      return res.status(404).json({ error: 'Payment not found' });
    }

    span.setStatus({ code: OpenTelemetry.SpanStatusCode.OK });
    res.json(payment);

  } catch (error) {
    logger.error('Failed to get payment:', error);
    span.setStatus({
      code: OpenTelemetry.SpanStatusCode.ERROR,
      message: error.message
    });
    res.status(500).json({ error: 'Failed to get payment' });
  } finally {
    span.end();
  }
});

// List payments
app.get('/api/v1/payments', async (req: Request, res: Response) => {
  const span = tracer.startSpan('list_payments');

  try {
    const { userId, orderId, status, limit = 20, offset = 0 } = req.query;

    const where: any = {};
    if (userId) where.userId = userId;
    if (orderId) where.orderId = orderId;
    if (status) where.status = status;

    const payments = await prisma.payment.findMany({
      where,
      take: Number(limit),
      skip: Number(offset),
      orderBy: { createdAt: 'desc' }
    });

    const total = await prisma.payment.count({ where });

    span.setStatus({ code: OpenTelemetry.SpanStatusCode.OK });

    res.json({
      payments,
      total,
      limit: Number(limit),
      offset: Number(offset)
    });

  } catch (error) {
    logger.error('Failed to list payments:', error);
    span.setStatus({
      code: OpenTelemetry.SpanStatusCode.ERROR,
      message: error.message
    });
    res.status(500).json({ error: 'Failed to list payments' });
  } finally {
    span.end();
  }
});

// Refund payment
app.post('/api/v1/payments/:id/refund', async (req: Request, res: Response) => {
  const span = tracer.startSpan('refund_payment');

  try {
    const { id } = req.params;
    const { amount, reason } = req.body;

    // Get payment
    const payment = await getPayment(id);
    if (!payment) {
      return res.status(404).json({ error: 'Payment not found' });
    }

    if (payment.status !== 'completed') {
      return res.status(400).json({ error: 'Payment cannot be refunded' });
    }

    // Create refund in Stripe
    const refund = await stripe.refunds.create({
      payment_intent: payment.stripePaymentIntentId,
      amount: amount ? Math.round(amount * 100) : undefined,
      reason: reason || 'requested_by_customer'
    });

    // Create refund record
    const refundRecord = await prisma.refund.create({
      data: {
        id: refund.id,
        paymentId: payment.id,
        amount: refund.amount / 100,
        currency: refund.currency,
        status: refund.status,
        reason: reason || 'requested_by_customer',
        stripeRefundId: refund.id
      }
    });

    // Update payment status
    await prisma.payment.update({
      where: { id },
      data: {
        status: 'refunded',
        refundedAt: new Date()
      }
    });

    // Send event
    await publishEvent('payment.refunded', {
      payment,
      refund: refundRecord
    });

    // Track metrics
    paymentCounter.add(1, { status: 'refunded', method: 'stripe' });

    span.setStatus({ code: OpenTelemetry.SpanStatusCode.OK });

    res.json(refundRecord);

  } catch (error) {
    logger.error('Failed to refund payment:', error);
    span.setStatus({
      code: OpenTelemetry.SpanStatusCode.ERROR,
      message: error.message
    });
    res.status(500).json({ error: 'Failed to refund payment' });
  } finally {
    span.end();
  }
});

// Stripe webhook
app.post('/api/v1/webhooks/stripe', express.raw({ type: 'application/json' }), async (req: Request, res: Response) => {
  const span = tracer.startSpan('stripe_webhook');

  try {
    const sig = req.headers['stripe-signature'] as string;

    if (!STRIPE_WEBHOOK_SECRET) {
      logger.warn('Stripe webhook secret not configured');
      return res.status(400).json({ error: 'Webhook not configured' });
    }

    // Verify webhook signature
    const event = stripe.webhooks.constructEvent(
      req.body,
      sig,
      STRIPE_WEBHOOK_SECRET
    );

    logger.info(`Received Stripe webhook: ${event.type}`);

    // Handle different event types
    switch (event.type) {
      case 'payment_intent.succeeded':
        await handlePaymentSucceeded(event.data.object as Stripe.PaymentIntent);
        break;

      case 'payment_intent.payment_failed':
        await handlePaymentFailed(event.data.object as Stripe.PaymentIntent);
        break;

      case 'charge.refunded':
        await handleChargeRefunded(event.data.object as Stripe.Charge);
        break;

      default:
        logger.info(`Unhandled event type: ${event.type}`);
    }

    span.setStatus({ code: OpenTelemetry.SpanStatusCode.OK });
    res.json({ received: true });

  } catch (error) {
    logger.error('Webhook error:', error);
    span.setStatus({
      code: OpenTelemetry.SpanStatusCode.ERROR,
      message: error.message
    });
    res.status(400).json({ error: 'Webhook error' });
  } finally {
    span.end();
  }
});

// Helper functions
async function getPayment(id: string) {
  // Try cache first
  const cached = await redis.get(`payment:${id}`);
  if (cached) {
    return JSON.parse(cached);
  }

  // Get from database
  const payment = await prisma.payment.findUnique({
    where: { id }
  });

  if (payment) {
    // Update cache
    await redis.set(
      `payment:${id}`,
      JSON.stringify(payment),
      'EX',
      600
    );
  }

  return payment;
}

async function publishEvent(eventType: string, data: any) {
  if (!amqpChannel) return;

  try {
    await amqpChannel.publish(
      'payments',
      eventType,
      Buffer.from(JSON.stringify({
        eventType,
        timestamp: new Date(),
        data
      })),
      { persistent: true }
    );

    logger.info(`Published event: ${eventType}`);
  } catch (error) {
    logger.error(`Failed to publish event ${eventType}:`, error);
  }
}

async function handlePaymentSucceeded(paymentIntent: Stripe.PaymentIntent) {
  await prisma.payment.update({
    where: { stripePaymentIntentId: paymentIntent.id },
    data: {
      status: 'completed',
      completedAt: new Date()
    }
  });

  await publishEvent('payment.completed', { paymentIntentId: paymentIntent.id });
}

async function handlePaymentFailed(paymentIntent: Stripe.PaymentIntent) {
  await prisma.payment.update({
    where: { stripePaymentIntentId: paymentIntent.id },
    data: {
      status: 'failed',
      failedAt: new Date()
    }
  });

  await publishEvent('payment.failed', { paymentIntentId: paymentIntent.id });
}

async function handleChargeRefunded(charge: Stripe.Charge) {
  const payment = await prisma.payment.findFirst({
    where: { stripePaymentIntentId: charge.payment_intent as string }
  });

  if (payment) {
    await prisma.payment.update({
      where: { id: payment.id },
      data: {
        status: 'refunded',
        refundedAt: new Date()
      }
    });

    await publishEvent('payment.refunded', { paymentId: payment.id });
  }
}

// Error handler
app.use((err: Error, req: Request, res: Response, next: NextFunction) => {
  logger.error('Unhandled error:', err);
  res.status(500).json({ error: 'Internal server error' });
});

// Start server
async function start() {
  try {
    // Connect to RabbitMQ
    await connectRabbitMQ();

    // Start server
    app.listen(PORT, () => {
      logger.info(`Payment service started on port ${PORT}`);
    });

  } catch (error) {
    logger.error('Failed to start server:', error);
    process.exit(1);
  }
}

// Graceful shutdown
process.on('SIGTERM', async () => {
  logger.info('SIGTERM received, shutting down gracefully');

  await prisma.$disconnect();
  await redis.quit();
  if (amqpChannel) await amqpChannel.close();

  process.exit(0);
});

start();