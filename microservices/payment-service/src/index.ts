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
    service: 'payment-service',
    version: '1.0.1',
    timestamp: new Date().toISOString()
  });
});

// Payments endpoints
app.get('/payments', (req: Request, res: Response) => {
  res.json([
    { id: 1, amount: 100, status: 'completed' },
    { id: 2, amount: 200, status: 'pending' }
  ]);
});

app.listen(PORT, () => {
  console.log(`Payment service is running on port ${PORT}`);
});
