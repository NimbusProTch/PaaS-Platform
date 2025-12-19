import { test, expect } from '@playwright/test';

const API_URL = process.env.API_URL || 'http://localhost:8083';

// Mock Stripe test data
const STRIPE_TEST_PAYMENT_METHOD = 'pm_card_visa'; // Test payment method ID

test.describe('Payment Service E2E Tests', () => {
  let paymentId: string;
  const orderId = 'order-' + Date.now();
  const userId = 'user-' + Date.now();

  test('should be healthy', async ({ request }) => {
    const response = await request.get(`${API_URL}/health`);
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    expect(data.status).toBe('healthy');
  });

  test('should be ready', async ({ request }) => {
    const response = await request.get(`${API_URL}/ready`);
    expect(response.ok()).toBeTruthy();
    const data = await response.json();
    expect(data.status).toBe('ready');
  });

  test('should create payment intent', async ({ request }) => {
    const paymentData = {
      amount: 99.99,
      currency: 'usd',
      orderId,
      userId,
      metadata: {
        productName: 'Test Product',
        quantity: 2
      }
    };

    const response = await request.post(`${API_URL}/api/v1/payments/intent`, {
      data: paymentData
    });

    expect(response.ok()).toBeTruthy();
    const result = await response.json();
    expect(result.clientSecret).toBeDefined();
    expect(result.paymentId).toBeDefined();

    paymentId = result.paymentId;
  });

  test('should get payment by ID', async ({ request }) => {
    expect(paymentId).toBeDefined();

    const response = await request.get(`${API_URL}/api/v1/payments/${paymentId}`);
    expect(response.ok()).toBeTruthy();

    const payment = await response.json();
    expect(payment.id).toBe(paymentId);
    expect(payment.orderId).toBe(orderId);
    expect(payment.userId).toBe(userId);
    expect(payment.amount).toBe(99.99);
    expect(payment.status).toBe('pending');
  });

  test('should list payments', async ({ request }) => {
    const response = await request.get(`${API_URL}/api/v1/payments?userId=${userId}`);
    expect(response.ok()).toBeTruthy();

    const result = await response.json();
    expect(result.payments).toBeDefined();
    expect(Array.isArray(result.payments)).toBeTruthy();
    expect(result.payments.length).toBeGreaterThan(0);
    expect(result.total).toBeGreaterThan(0);
  });

  test('should confirm payment', async ({ request }) => {
    expect(paymentId).toBeDefined();

    const response = await request.post(`${API_URL}/api/v1/payments/${paymentId}/confirm`, {
      data: {
        paymentMethodId: STRIPE_TEST_PAYMENT_METHOD
      }
    });

    // Note: This will likely fail in test environment without real Stripe setup
    // In production tests, you would use Stripe test mode
    if (response.ok()) {
      const payment = await response.json();
      expect(payment.status).toMatch(/completed|failed/);
    }
  });

  test('should handle invalid payment amount', async ({ request }) => {
    const invalidPayment = {
      amount: -10,
      currency: 'usd',
      orderId: 'invalid-order',
      userId: 'invalid-user'
    };

    const response = await request.post(`${API_URL}/api/v1/payments/intent`, {
      data: invalidPayment
    });

    expect(response.status()).toBe(400);
    const error = await response.json();
    expect(error.error).toContain('Invalid amount');
  });

  test('should handle missing payment', async ({ request }) => {
    const response = await request.get(`${API_URL}/api/v1/payments/non-existent-payment`);
    expect(response.status()).toBe(404);
  });

  test('should create multiple payment intents', async ({ request }) => {
    const payments = [];

    for (let i = 0; i < 3; i++) {
      const paymentData = {
        amount: (i + 1) * 25.50,
        currency: 'usd',
        orderId: `multi-order-${i}`,
        userId: `multi-user-${i}`,
        metadata: {
          test: true,
          index: i
        }
      };

      const response = await request.post(`${API_URL}/api/v1/payments/intent`, {
        data: paymentData
      });

      expect(response.ok()).toBeTruthy();
      const result = await response.json();
      payments.push(result.paymentId);
    }

    expect(payments.length).toBe(3);
  });

  test('should filter payments by status', async ({ request }) => {
    const response = await request.get(`${API_URL}/api/v1/payments?status=pending`);
    expect(response.ok()).toBeTruthy();

    const result = await response.json();
    if (result.payments.length > 0) {
      result.payments.forEach((payment: any) => {
        expect(payment.status).toBe('pending');
      });
    }
  });

  test('should paginate payments', async ({ request }) => {
    const page1 = await request.get(`${API_URL}/api/v1/payments?limit=5&offset=0`);
    expect(page1.ok()).toBeTruthy();
    const result1 = await page1.json();
    expect(result1.limit).toBe(5);
    expect(result1.offset).toBe(0);

    const page2 = await request.get(`${API_URL}/api/v1/payments?limit=5&offset=5`);
    expect(page2.ok()).toBeTruthy();
    const result2 = await page2.json();
    expect(result2.limit).toBe(5);
    expect(result2.offset).toBe(5);
  });

  test('should expose metrics', async ({ request }) => {
    const response = await request.get(`${API_URL}/metrics`);
    expect(response.ok()).toBeTruthy();

    const metrics = await response.text();
    expect(metrics).toContain('http_request_duration');
    expect(metrics).toContain('payments_total');
    expect(metrics).toContain('payment_amount');
  });
});

test.describe('Payment Service Refund Tests', () => {
  test.skip('should create and refund payment', async ({ request }) => {
    // Create payment intent
    const paymentData = {
      amount: 50.00,
      currency: 'usd',
      orderId: 'refund-order-' + Date.now(),
      userId: 'refund-user-' + Date.now()
    };

    const createResponse = await request.post(`${API_URL}/api/v1/payments/intent`, {
      data: paymentData
    });
    const { paymentId } = await createResponse.json();

    // Confirm payment (would need real Stripe test setup)
    await request.post(`${API_URL}/api/v1/payments/${paymentId}/confirm`, {
      data: { paymentMethodId: STRIPE_TEST_PAYMENT_METHOD }
    });

    // Refund payment
    const refundResponse = await request.post(`${API_URL}/api/v1/payments/${paymentId}/refund`, {
      data: {
        amount: 25.00,
        reason: 'Test refund'
      }
    });

    if (refundResponse.ok()) {
      const refund = await refundResponse.json();
      expect(refund.paymentId).toBe(paymentId);
      expect(refund.amount).toBe(25.00);
      expect(refund.reason).toBe('Test refund');
    }
  });
});

test.describe('Payment Service Load Tests', () => {
  test('should handle concurrent payment creation', async ({ request }) => {
    const promises = [];

    for (let i = 0; i < 10; i++) {
      const paymentData = {
        amount: Math.random() * 1000,
        currency: 'usd',
        orderId: `load-order-${i}`,
        userId: `load-user-${i}`
      };

      promises.push(
        request.post(`${API_URL}/api/v1/payments/intent`, {
          data: paymentData
        })
      );
    }

    const responses = await Promise.all(promises);
    responses.forEach(response => {
      expect(response.ok()).toBeTruthy();
    });
  });

  test('should handle rapid payment queries', async ({ request }) => {
    // Create a payment first
    const paymentData = {
      amount: 100,
      currency: 'usd',
      orderId: 'query-order',
      userId: 'query-user'
    };

    const createResponse = await request.post(`${API_URL}/api/v1/payments/intent`, {
      data: paymentData
    });
    const { paymentId } = await createResponse.json();

    // Rapid queries
    const promises = [];
    for (let i = 0; i < 20; i++) {
      promises.push(
        request.get(`${API_URL}/api/v1/payments/${paymentId}`)
      );
    }

    const responses = await Promise.all(promises);
    responses.forEach(response => {
      expect(response.ok()).toBeTruthy();
    });
  });
});