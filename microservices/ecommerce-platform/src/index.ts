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
    service: 'ecommerce-platform',
    version: '1.0.1',
    timestamp: new Date().toISOString()
  });
});

// Platform endpoints
app.get('/api/status', (req: Request, res: Response) => {
  res.json({
    platform: 'ecommerce-platform',
    services: ['product', 'user', 'order', 'payment', 'notification'],
    status: 'operational'
  });
});

app.listen(PORT, () => {
  console.log(`Ecommerce platform is running on port ${PORT}`);
});
