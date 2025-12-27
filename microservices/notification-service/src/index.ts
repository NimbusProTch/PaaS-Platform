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
    service: 'notification-service',
    version: '1.0.0',
    timestamp: new Date().toISOString()
  });
});

// Notifications endpoints
app.get('/notifications', (req: Request, res: Response) => {
  res.json([
    { id: 1, type: 'email', message: 'Order confirmed' },
    { id: 2, type: 'sms', message: 'Payment received' }
  ]);
});

app.listen(PORT, () => {
  console.log(`Notification service is running on port ${PORT}`);
});
