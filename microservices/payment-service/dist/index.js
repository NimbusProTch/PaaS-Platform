"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const express_1 = __importDefault(require("express"));
const cors_1 = __importDefault(require("cors"));
const helmet_1 = __importDefault(require("helmet"));
const client_1 = require("@prisma/client");
const stripe_1 = __importDefault(require("stripe"));
const ioredis_1 = __importDefault(require("ioredis"));
const amqplib_1 = __importDefault(require("amqplib"));
const OpenTelemetry = __importStar(require("@opentelemetry/api"));
const sdk_trace_node_1 = require("@opentelemetry/sdk-trace-node");
const instrumentation_1 = require("@opentelemetry/instrumentation");
const instrumentation_http_1 = require("@opentelemetry/instrumentation-http");
const instrumentation_express_1 = require("@opentelemetry/instrumentation-express");
const resources_1 = require("@opentelemetry/resources");
const semantic_conventions_1 = require("@opentelemetry/semantic-conventions");
const exporter_trace_otlp_grpc_1 = require("@opentelemetry/exporter-trace-otlp-grpc");
const sdk_trace_base_1 = require("@opentelemetry/sdk-trace-base");
const exporter_prometheus_1 = require("@opentelemetry/exporter-prometheus");
const sdk_metrics_1 = require("@opentelemetry/sdk-metrics");
const winston_1 = __importDefault(require("winston"));
const winston_loki_1 = __importDefault(require("winston-loki"));
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
const logger = winston_1.default.createLogger({
    level: 'info',
    format: winston_1.default.format.json(),
    transports: [
        new winston_1.default.transports.Console({
            format: winston_1.default.format.simple()
        })
    ]
});
// Add Loki transport in production
if (process.env.NODE_ENV === 'production') {
    logger.add(new winston_loki_1.default({
        host: LOKI_HOST,
        labels: {
            service: 'payment-service',
            environment: process.env.ENVIRONMENT || 'production'
        }
    }));
}
// Initialize OpenTelemetry
const resource = new resources_1.Resource({
    [semantic_conventions_1.SemanticResourceAttributes.SERVICE_NAME]: 'payment-service',
    [semantic_conventions_1.SemanticResourceAttributes.SERVICE_VERSION]: '1.0.0',
});
const provider = new sdk_trace_node_1.NodeTracerProvider({
    resource,
});
const exporter = new exporter_trace_otlp_grpc_1.OTLPTraceExporter({
    url: `http://${JAEGER_ENDPOINT}`,
});
provider.addSpanProcessor(new sdk_trace_base_1.BatchSpanProcessor(exporter));
provider.register();
(0, instrumentation_1.registerInstrumentations)({
    instrumentations: [
        new instrumentation_http_1.HttpInstrumentation(),
        new instrumentation_express_1.ExpressInstrumentation(),
    ],
});
const tracer = OpenTelemetry.trace.getTracer('payment-service');
// Initialize Prometheus metrics
const prometheusExporter = new exporter_prometheus_1.PrometheusExporter({
    port: 9090,
    endpoint: '/metrics',
}, () => {
    logger.info('Prometheus metrics server started on port 9090');
});
const meterProvider = new sdk_metrics_1.MeterProvider({
    readers: [prometheusExporter],
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
const stripe = new stripe_1.default(STRIPE_SECRET_KEY, {
    apiVersion: '2023-10-16',
});
// Initialize Prisma
const prisma = new client_1.PrismaClient();
// Initialize Redis
const redis = new ioredis_1.default(REDIS_URL);
// RabbitMQ connection
let amqpChannel = null;
async function connectRabbitMQ() {
    try {
        const connection = await amqplib_1.default.connect(AMQP_URL);
        amqpChannel = await connection.createChannel();
        await amqpChannel.assertExchange('payments', 'topic', { durable: true });
        logger.info('Connected to RabbitMQ');
    }
    catch (error) {
        logger.error('Failed to connect to RabbitMQ:', error);
    }
}
// Express app
const app = (0, express_1.default)();
// Middleware
app.use((0, helmet_1.default)());
app.use((0, cors_1.default)());
app.use(express_1.default.json());
app.use(express_1.default.raw({ type: 'application/json' })); // For Stripe webhook
// Request timing middleware
app.use((req, res, next) => {
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
app.get('/health', (req, res) => {
    res.json({ status: 'healthy' });
});
// Ready check
app.get('/ready', async (req, res) => {
    try {
        await prisma.$queryRaw `SELECT 1`;
        await redis.ping();
        res.json({ status: 'ready' });
    }
    catch (error) {
        res.status(503).json({ status: 'not ready', error: error.message });
    }
});
// Payment endpoints
// Create payment intent
app.post('/api/v1/payments/intent', async (req, res) => {
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
        await redis.set(`payment:${payment.id}`, JSON.stringify(payment), 'EX', 600 // 10 minutes
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
    }
    catch (error) {
        logger.error('Failed to create payment intent:', error);
        span.setStatus({
            code: OpenTelemetry.SpanStatusCode.ERROR,
            message: error.message
        });
        res.status(500).json({ error: 'Failed to create payment intent' });
    }
    finally {
        span.end();
    }
});
// Confirm payment
app.post('/api/v1/payments/:id/confirm', async (req, res) => {
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
        const paymentIntent = await stripe.paymentIntents.confirm(payment.stripePaymentIntentId, {
            payment_method: paymentMethodId
        });
        // Update payment status
        payment = await prisma.payment.update({
            where: { id },
            data: {
                status: paymentIntent.status === 'succeeded' ? 'completed' : 'failed',
                completedAt: paymentIntent.status === 'succeeded' ? new Date() : null
            }
        });
        // Update cache
        await redis.set(`payment:${payment.id}`, JSON.stringify(payment), 'EX', 600);
        // Send event
        await publishEvent(`payment.${payment.status}`, payment);
        // Track metrics
        paymentCounter.add(1, { status: payment.status, method: 'stripe' });
        span.setStatus({ code: OpenTelemetry.SpanStatusCode.OK });
        res.json(payment);
    }
    catch (error) {
        logger.error('Failed to confirm payment:', error);
        span.setStatus({
            code: OpenTelemetry.SpanStatusCode.ERROR,
            message: error.message
        });
        res.status(500).json({ error: 'Failed to confirm payment' });
    }
    finally {
        span.end();
    }
});
// Get payment
app.get('/api/v1/payments/:id', async (req, res) => {
    const span = tracer.startSpan('get_payment');
    try {
        const { id } = req.params;
        const payment = await getPayment(id);
        if (!payment) {
            return res.status(404).json({ error: 'Payment not found' });
        }
        span.setStatus({ code: OpenTelemetry.SpanStatusCode.OK });
        res.json(payment);
    }
    catch (error) {
        logger.error('Failed to get payment:', error);
        span.setStatus({
            code: OpenTelemetry.SpanStatusCode.ERROR,
            message: error.message
        });
        res.status(500).json({ error: 'Failed to get payment' });
    }
    finally {
        span.end();
    }
});
// List payments
app.get('/api/v1/payments', async (req, res) => {
    const span = tracer.startSpan('list_payments');
    try {
        const { userId, orderId, status, limit = 20, offset = 0 } = req.query;
        const where = {};
        if (userId)
            where.userId = userId;
        if (orderId)
            where.orderId = orderId;
        if (status)
            where.status = status;
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
    }
    catch (error) {
        logger.error('Failed to list payments:', error);
        span.setStatus({
            code: OpenTelemetry.SpanStatusCode.ERROR,
            message: error.message
        });
        res.status(500).json({ error: 'Failed to list payments' });
    }
    finally {
        span.end();
    }
});
// Refund payment
app.post('/api/v1/payments/:id/refund', async (req, res) => {
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
    }
    catch (error) {
        logger.error('Failed to refund payment:', error);
        span.setStatus({
            code: OpenTelemetry.SpanStatusCode.ERROR,
            message: error.message
        });
        res.status(500).json({ error: 'Failed to refund payment' });
    }
    finally {
        span.end();
    }
});
// Stripe webhook
app.post('/api/v1/webhooks/stripe', express_1.default.raw({ type: 'application/json' }), async (req, res) => {
    const span = tracer.startSpan('stripe_webhook');
    try {
        const sig = req.headers['stripe-signature'];
        if (!STRIPE_WEBHOOK_SECRET) {
            logger.warn('Stripe webhook secret not configured');
            return res.status(400).json({ error: 'Webhook not configured' });
        }
        // Verify webhook signature
        const event = stripe.webhooks.constructEvent(req.body, sig, STRIPE_WEBHOOK_SECRET);
        logger.info(`Received Stripe webhook: ${event.type}`);
        // Handle different event types
        switch (event.type) {
            case 'payment_intent.succeeded':
                await handlePaymentSucceeded(event.data.object);
                break;
            case 'payment_intent.payment_failed':
                await handlePaymentFailed(event.data.object);
                break;
            case 'charge.refunded':
                await handleChargeRefunded(event.data.object);
                break;
            default:
                logger.info(`Unhandled event type: ${event.type}`);
        }
        span.setStatus({ code: OpenTelemetry.SpanStatusCode.OK });
        res.json({ received: true });
    }
    catch (error) {
        logger.error('Webhook error:', error);
        span.setStatus({
            code: OpenTelemetry.SpanStatusCode.ERROR,
            message: error.message
        });
        res.status(400).json({ error: 'Webhook error' });
    }
    finally {
        span.end();
    }
});
// Helper functions
async function getPayment(id) {
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
        await redis.set(`payment:${id}`, JSON.stringify(payment), 'EX', 600);
    }
    return payment;
}
async function publishEvent(eventType, data) {
    if (!amqpChannel)
        return;
    try {
        await amqpChannel.publish('payments', eventType, Buffer.from(JSON.stringify({
            eventType,
            timestamp: new Date(),
            data
        })), { persistent: true });
        logger.info(`Published event: ${eventType}`);
    }
    catch (error) {
        logger.error(`Failed to publish event ${eventType}:`, error);
    }
}
async function handlePaymentSucceeded(paymentIntent) {
    await prisma.payment.update({
        where: { stripePaymentIntentId: paymentIntent.id },
        data: {
            status: 'completed',
            completedAt: new Date()
        }
    });
    await publishEvent('payment.completed', { paymentIntentId: paymentIntent.id });
}
async function handlePaymentFailed(paymentIntent) {
    await prisma.payment.update({
        where: { stripePaymentIntentId: paymentIntent.id },
        data: {
            status: 'failed',
            failedAt: new Date()
        }
    });
    await publishEvent('payment.failed', { paymentIntentId: paymentIntent.id });
}
async function handleChargeRefunded(charge) {
    const payment = await prisma.payment.findFirst({
        where: { stripePaymentIntentId: charge.payment_intent }
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
app.use((err, req, res, next) => {
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
    }
    catch (error) {
        logger.error('Failed to start server:', error);
        process.exit(1);
    }
}
// Graceful shutdown
process.on('SIGTERM', async () => {
    logger.info('SIGTERM received, shutting down gracefully');
    await prisma.$disconnect();
    await redis.quit();
    if (amqpChannel)
        await amqpChannel.close();
    process.exit(0);
});
start();
//# sourceMappingURL=index.js.map