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
    service: 'user-service',
    version: '1.0.0',
    timestamp: new Date().toISOString()
  });
});

// Users endpoints
app.get('/users', (req: Request, res: Response) => {
  res.json([
    { id: 1, name: 'User 1', email: 'user1@example.com' },
    { id: 2, name: 'User 2', email: 'user2@example.com' },
    { id: 3, name: 'User 3', email: 'user3@example.com' }
  ]);
});

app.listen(PORT, () => {
  console.log(`User service is running on port ${PORT}`);
});