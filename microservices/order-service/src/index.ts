import express, { Request, Response } from 'express';

const app = express();
const PORT = process.env.PORT || 8080;

// Health check endpoint
app.get('/health', (req: Request, res: Response) => {
  res.status(200).json({ status: 'healthy' });
});

// Ready check endpoint
app.get('/ready', (req: Request, res: Response) => {
  res.status(200).json({ status: 'ready' });
});

// Main endpoint
app.get('/', (req: Request, res: Response) => {
  res.json({
    service: 'order-service',
    version: '1.0.1',
    timestamp: new Date().toISOString()
  });
});

// Orders endpoints
app.get('/orders', (req: Request, res: Response) => {
  res.json([
    { id: 1, product: 'Product 1', quantity: 2, status: 'pending' },
    { id: 2, product: 'Product 2', quantity: 1, status: 'shipped' }
  ]);
});

app.listen(PORT, () => {
  console.log(`Order service is running on port ${PORT}`);
});