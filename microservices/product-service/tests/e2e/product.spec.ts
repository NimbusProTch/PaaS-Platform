import { test, expect } from '@playwright/test';

const API_URL = process.env.API_URL || 'http://localhost:8080';

test.describe('Product Service E2E Tests', () => {
  let productId: string;

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

  test('should create a product', async ({ request }) => {
    const product = {
      sku: `TEST-${Date.now()}`,
      name: 'Test Product',
      description: 'This is a test product',
      category: 'Electronics',
      price: 99.99,
      currency: 'USD',
      stock: 100,
      images: ['image1.jpg', 'image2.jpg'],
      attributes: {
        color: 'Black',
        size: 'Medium'
      }
    };

    const response = await request.post(`${API_URL}/api/v1/products`, {
      data: product
    });

    expect(response.ok()).toBeTruthy();
    const created = await response.json();
    expect(created.id).toBeDefined();
    expect(created.sku).toBe(product.sku);
    expect(created.name).toBe(product.name);
    expect(created.price).toBe(product.price);

    productId = created.id;
  });

  test('should get a product by ID', async ({ request }) => {
    expect(productId).toBeDefined();

    const response = await request.get(`${API_URL}/api/v1/products/${productId}`);
    expect(response.ok()).toBeTruthy();

    const product = await response.json();
    expect(product.id).toBe(productId);
  });

  test('should list all products', async ({ request }) => {
    const response = await request.get(`${API_URL}/api/v1/products`);
    expect(response.ok()).toBeTruthy();

    const products = await response.json();
    expect(Array.isArray(products)).toBeTruthy();
    expect(products.length).toBeGreaterThan(0);
  });

  test('should search products', async ({ request }) => {
    const response = await request.get(`${API_URL}/api/v1/products/search?q=Test&category=Electronics`);
    expect(response.ok()).toBeTruthy();

    const products = await response.json();
    expect(Array.isArray(products)).toBeTruthy();
  });

  test('should update a product', async ({ request }) => {
    expect(productId).toBeDefined();

    const updates = {
      name: 'Updated Test Product',
      price: 149.99,
      stock: 50
    };

    const response = await request.put(`${API_URL}/api/v1/products/${productId}`, {
      data: updates
    });

    expect(response.ok()).toBeTruthy();
  });

  test('should delete a product', async ({ request }) => {
    expect(productId).toBeDefined();

    const response = await request.delete(`${API_URL}/api/v1/products/${productId}`);
    expect(response.ok()).toBeTruthy();

    // Verify deletion
    const getResponse = await request.get(`${API_URL}/api/v1/products/${productId}`);
    expect(getResponse.status()).toBe(404);
  });

  test('should return 404 for non-existent product', async ({ request }) => {
    const response = await request.get(`${API_URL}/api/v1/products/non-existent-id`);
    expect(response.status()).toBe(404);
  });

  test('should handle invalid product data', async ({ request }) => {
    const invalidProduct = {
      // Missing required fields
      description: 'Invalid product'
    };

    const response = await request.post(`${API_URL}/api/v1/products`, {
      data: invalidProduct
    });

    expect(response.status()).toBe(400);
  });

  test('should expose metrics', async ({ request }) => {
    const response = await request.get(`${API_URL}/metrics`);
    expect(response.ok()).toBeTruthy();

    const metrics = await response.text();
    expect(metrics).toContain('http_request_duration_seconds');
    expect(metrics).toContain('db_query_duration_seconds');
  });
});

test.describe('Product Service Load Tests', () => {
  test('should handle concurrent requests', async ({ request }) => {
    const promises = [];

    // Create 10 concurrent requests
    for (let i = 0; i < 10; i++) {
      promises.push(
        request.get(`${API_URL}/api/v1/products`)
      );
    }

    const responses = await Promise.all(promises);
    responses.forEach(response => {
      expect(response.ok()).toBeTruthy();
    });
  });

  test('should handle rapid product creation', async ({ request }) => {
    const promises = [];

    // Create 5 products rapidly
    for (let i = 0; i < 5; i++) {
      const product = {
        sku: `LOAD-TEST-${Date.now()}-${i}`,
        name: `Load Test Product ${i}`,
        description: 'Load test product',
        category: 'Test',
        price: Math.random() * 100,
        stock: Math.floor(Math.random() * 100)
      };

      promises.push(
        request.post(`${API_URL}/api/v1/products`, {
          data: product
        })
      );
    }

    const responses = await Promise.all(promises);
    responses.forEach(response => {
      expect(response.ok()).toBeTruthy();
    });
  });
});