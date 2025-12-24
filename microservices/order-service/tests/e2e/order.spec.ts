import { test, expect } from '@playwright/test';

const API_URL = process.env.API_URL || 'http://localhost:8082';

test.describe('Order Service E2E Tests', () => {
  let orderId: string;
  const userId = 'test-user-' + Date.now();

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

  test('should create an order', async ({ request }) => {
    const order = {
      items: [
        {
          product_id: 'PROD-001',
          product_name: 'Test Product 1',
          product_sku: 'SKU-001',
          quantity: 2,
          price: 29.99
        },
        {
          product_id: 'PROD-002',
          product_name: 'Test Product 2',
          product_sku: 'SKU-002',
          quantity: 1,
          price: 49.99
        }
      ],
      shipping_address: {
        street: '123 Main St',
        city: 'New York',
        state: 'NY',
        country: 'USA',
        postal_code: '10001',
        phone: '+1-555-1234'
      },
      billing_address: {
        street: '123 Main St',
        city: 'New York',
        state: 'NY',
        country: 'USA',
        postal_code: '10001',
        phone: '+1-555-1234'
      },
      payment_method: 'credit_card',
      payment_id: 'pay_' + Date.now(),
      notes: 'Please deliver between 9 AM and 5 PM'
    };

    const response = await request.post(`${API_URL}/api/v1/orders`, {
      data: order,
      headers: {
        'X-User-ID': userId
      }
    });

    expect(response.ok()).toBeTruthy();
    const created = await response.json();
    expect(created.id).toBeDefined();
    expect(created.user_id).toBe(userId);
    expect(created.status).toBe('pending');
    expect(created.total_price).toBe(109.97); // (2 * 29.99) + 49.99

    orderId = created.id;
  });

  test('should get an order by ID', async ({ request }) => {
    expect(orderId).toBeDefined();

    const response = await request.get(`${API_URL}/api/v1/orders/${orderId}`);
    expect(response.ok()).toBeTruthy();

    const order = await response.json();
    expect(order.id).toBe(orderId);
    expect(order.user_id).toBe(userId);
  });

  test('should get user orders', async ({ request }) => {
    const response = await request.get(`${API_URL}/api/v1/users/${userId}/orders`);
    expect(response.ok()).toBeTruthy();

    const data = await response.json();
    expect(data.orders).toBeDefined();
    expect(Array.isArray(data.orders)).toBeTruthy();
    expect(data.orders.length).toBeGreaterThan(0);
    expect(data.total).toBeGreaterThan(0);
  });

  test('should update an order', async ({ request }) => {
    expect(orderId).toBeDefined();

    const updates = {
      items: [
        {
          product_id: 'PROD-001',
          product_name: 'Test Product 1',
          product_sku: 'SKU-001',
          quantity: 3, // Updated quantity
          price: 29.99
        }
      ],
      shipping_address: {
        street: '456 Oak Ave', // Updated address
        city: 'Los Angeles',
        state: 'CA',
        country: 'USA',
        postal_code: '90001',
        phone: '+1-555-5678'
      },
      billing_address: {
        street: '456 Oak Ave',
        city: 'Los Angeles',
        state: 'CA',
        country: 'USA',
        postal_code: '90001',
        phone: '+1-555-5678'
      }
    };

    const response = await request.put(`${API_URL}/api/v1/orders/${orderId}`, {
      data: updates
    });

    expect(response.ok()).toBeTruthy();
    const updated = await response.json();
    expect(updated.total_price).toBe(89.97); // 3 * 29.99
  });

  test('should update order status', async ({ request }) => {
    expect(orderId).toBeDefined();

    // Update to confirmed
    let response = await request.patch(`${API_URL}/api/v1/orders/${orderId}/status`, {
      data: { status: 'confirmed' }
    });
    expect(response.ok()).toBeTruthy();

    // Verify status was updated
    response = await request.get(`${API_URL}/api/v1/orders/${orderId}`);
    let order = await response.json();
    expect(order.status).toBe('confirmed');

    // Update to processing
    response = await request.patch(`${API_URL}/api/v1/orders/${orderId}/status`, {
      data: { status: 'processing' }
    });
    expect(response.ok()).toBeTruthy();

    // Update to shipped
    response = await request.patch(`${API_URL}/api/v1/orders/${orderId}/status`, {
      data: { status: 'shipped' }
    });
    expect(response.ok()).toBeTruthy();

    // Verify final status
    response = await request.get(`${API_URL}/api/v1/orders/${orderId}`);
    order = await response.json();
    expect(order.status).toBe('shipped');
  });

  test('should calculate order total', async ({ request }) => {
    const order = {
      items: [
        { product_id: 'P1', quantity: 2, price: 10.50 },
        { product_id: 'P2', quantity: 3, price: 5.25 },
        { product_id: 'P3', quantity: 1, price: 100.00 }
      ]
    };

    const response = await request.post(`${API_URL}/api/v1/orders/calculate`, {
      data: order
    });

    expect(response.ok()).toBeTruthy();
    const result = await response.json();
    expect(result.total).toBe(136.75); // (2*10.50) + (3*5.25) + (1*100)
    expect(result.items).toBe(3);
  });

  test('should handle invalid order data', async ({ request }) => {
    const invalidOrder = {
      // Missing required fields
      items: []
    };

    const response = await request.post(`${API_URL}/api/v1/orders`, {
      data: invalidOrder,
      headers: {
        'X-User-ID': userId
      }
    });

    expect(response.status()).toBe(500);
  });

  test('should handle invalid status transition', async ({ request }) => {
    // Create a new order
    const order = {
      items: [{ product_id: 'P1', quantity: 1, price: 10 }],
      shipping_address: { street: '123 Main', city: 'NY', country: 'USA' },
      billing_address: { street: '123 Main', city: 'NY', country: 'USA' }
    };

    const createResponse = await request.post(`${API_URL}/api/v1/orders`, {
      data: order,
      headers: { 'X-User-ID': userId }
    });
    const created = await createResponse.json();

    // Try invalid transition from pending to delivered
    const response = await request.patch(`${API_URL}/api/v1/orders/${created.id}/status`, {
      data: { status: 'delivered' }
    });

    expect(response.status()).toBe(500);
  });

  test('should cancel an order', async ({ request }) => {
    // Create a new order
    const order = {
      items: [{ product_id: 'P1', quantity: 1, price: 10 }],
      shipping_address: { street: '123 Main', city: 'NY', country: 'USA' },
      billing_address: { street: '123 Main', city: 'NY', country: 'USA' }
    };

    const createResponse = await request.post(`${API_URL}/api/v1/orders`, {
      data: order,
      headers: { 'X-User-ID': userId }
    });
    const created = await createResponse.json();

    // Cancel the order
    const response = await request.delete(`${API_URL}/api/v1/orders/${created.id}`);
    expect(response.ok()).toBeTruthy();

    // Verify order was cancelled
    const getResponse = await request.get(`${API_URL}/api/v1/orders/${created.id}`);
    const cancelled = await getResponse.json();
    expect(cancelled.status).toBe('cancelled');
  });

  test('should expose metrics', async ({ request }) => {
    const response = await request.get(`${API_URL}/metrics`);
    expect(response.ok()).toBeTruthy();

    const metrics = await response.text();
    expect(metrics).toContain('http_request_duration_seconds');
    expect(metrics).toContain('db_query_duration_seconds');
    expect(metrics).toContain('orders_total');
  });
});

test.describe('Order Service Load Tests', () => {
  test('should handle concurrent order creation', async ({ request }) => {
    const promises = [];

    // Create 10 orders concurrently
    for (let i = 0; i < 10; i++) {
      const order = {
        items: [
          {
            product_id: `LOAD-PROD-${i}`,
            product_name: `Load Test Product ${i}`,
            quantity: Math.floor(Math.random() * 5) + 1,
            price: Math.random() * 100
          }
        ],
        shipping_address: {
          street: `${i} Test St`,
          city: 'Test City',
          country: 'USA'
        },
        billing_address: {
          street: `${i} Test St`,
          city: 'Test City',
          country: 'USA'
        }
      };

      promises.push(
        request.post(`${API_URL}/api/v1/orders`, {
          data: order,
          headers: {
            'X-User-ID': `load-test-user-${i}`
          }
        })
      );
    }

    const responses = await Promise.all(promises);
    responses.forEach(response => {
      expect(response.ok()).toBeTruthy();
    });
  });

  test('should handle rapid status updates', async ({ request }) => {
    // Create an order first
    const order = {
      items: [{ product_id: 'P1', quantity: 1, price: 10 }],
      shipping_address: { street: '123 Main', city: 'NY', country: 'USA' },
      billing_address: { street: '123 Main', city: 'NY', country: 'USA' }
    };

    const createResponse = await request.post(`${API_URL}/api/v1/orders`, {
      data: order,
      headers: { 'X-User-ID': 'rapid-test-user' }
    });
    const created = await createResponse.json();

    // Rapid status updates
    const statusUpdates = ['confirmed', 'processing', 'shipped'];

    for (const status of statusUpdates) {
      const response = await request.patch(`${API_URL}/api/v1/orders/${created.id}/status`, {
        data: { status }
      });
      expect(response.ok()).toBeTruthy();
    }
  });
});