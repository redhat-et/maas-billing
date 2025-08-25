import express from 'express';
import cors from 'cors';
import { createServer } from 'http';
import { Server as SocketIOServer } from 'socket.io';
import { io as SocketIOClient } from 'socket.io-client';
import { logger } from './utils/logger';
import metricsRoutes from './routes/metrics';
import policiesRoutes from './routes/policies';

const app: express.Application = express();
const server = createServer(app);
const io = new SocketIOServer(server, {
  cors: {
    origin: process.env.FRONTEND_URL || 'http://localhost:3000',
    methods: ['GET', 'POST'],
    credentials: true
  }
});
const PORT = process.env.PORT || 3001;
const QOS_SERVICE_URL = process.env.QOS_SERVICE_URL || 'http://localhost:3005';

// Middleware
app.use(cors({
  origin: process.env.FRONTEND_URL || 'http://localhost:3000',
  credentials: true
}));
app.use(express.json());
app.use(express.urlencoded({ extended: true }));

// Request logging
app.use((req, res, next) => {
  logger.info(`${req.method} ${req.path}`, {
    query: req.query,
    userAgent: req.get('User-Agent')
  });
  next();
});

// Health check
app.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    timestamp: new Date().toISOString(),
    service: 'maas-backend'
  });
});

// API routes
app.use('/api/v1/metrics', metricsRoutes);
app.use('/api/v1/policies', policiesRoutes);

// QoS proxy endpoints
app.get('/api/v1/qos/metrics', async (req, res) => {
  try {
    const response = await fetch(`${QOS_SERVICE_URL}/metrics`);
    const data = await response.json();
    res.json({ success: true, data });
  } catch (error) {
    logger.error('Failed to fetch QoS metrics:', error);
    res.status(500).json({ success: false, error: 'QoS service unavailable' });
  }
});

app.get('/api/v1/qos/health', async (req, res) => {
  try {
    const response = await fetch(`${QOS_SERVICE_URL}/health`);
    const data = await response.json();
    res.json({ success: true, data });
  } catch (error) {
    logger.error('Failed to fetch QoS health:', error);
    res.status(500).json({ success: false, error: 'QoS service unavailable' });
  }
});

// Models endpoint for compatibility
app.get('/api/v1/models', (req, res) => {
  res.json({
    success: true,
    data: [
      {
        id: 'vllm-simulator',
        name: 'vLLM Simulator',
        provider: 'KServe',
        description: 'Test model for policy enforcement'
      },
      {
        id: 'qwen3-0-6b-instruct',
        name: 'Qwen3 0.6B Instruct',
        provider: 'KServe',
        description: 'Qwen3 model with vLLM runtime'
      }
    ]
  });
});

// Error handling
app.use((err: any, req: express.Request, res: express.Response, next: express.NextFunction) => {
  logger.error('Unhandled error:', err);
  res.status(500).json({
    success: false,
    error: 'Internal server error'
  });
});

// 404 handler
app.use('*', (req, res) => {
  res.status(404).json({
    success: false,
    error: 'Route not found'
  });
});

// QoS Service integration
let qosClient: any = null;

function connectToQoSService() {
  try {
    qosClient = SocketIOClient(QOS_SERVICE_URL);
    
    qosClient.on('connect', () => {
      logger.info('Connected to QoS service', { url: QOS_SERVICE_URL });
    });
    
    qosClient.on('disconnect', () => {
      logger.warn('Disconnected from QoS service');
    });
    
    qosClient.on('connect_error', (error: any) => {
      logger.error('QoS service connection error:', error);
    });
    
    // Forward QoS events to frontend clients
    qosClient.on('queue_update', (data: any) => {
      io.emit('qos_queue_update', data);
    });
    
    qosClient.on('queue_stats', (data: any) => {
      io.emit('qos_queue_stats', data);
    });
    
    qosClient.on('request_queued', (data: any) => {
      io.emit('qos_request_queued', data);
    });
    
    qosClient.on('request_completed', (data: any) => {
      io.emit('qos_request_completed', data);
    });
    
  } catch (error) {
    logger.error('Failed to connect to QoS service:', error);
  }
}

// Socket.IO connection handling
io.on('connection', (socket) => {
  logger.info('Frontend client connected', { 
    socketId: socket.id,
    clientIP: socket.handshake.address 
  });
  
  // Handle QoS monitoring requests
  socket.on('subscribe_qos', () => {
    logger.info('Client subscribed to QoS updates', { socketId: socket.id });
    if (qosClient?.connected) {
      qosClient.emit('get_metrics');
      qosClient.emit('get_detailed_stats');
    }
  });
  
  socket.on('unsubscribe_qos', () => {
    logger.info('Client unsubscribed from QoS updates', { socketId: socket.id });
  });
  
  socket.on('disconnect', () => {
    logger.info('Frontend client disconnected', { socketId: socket.id });
  });
});


// Start server
if (require.main === module) {
  server.listen(PORT, () => {
    logger.info(`Server running on port ${PORT}`);
    logger.info('Available endpoints:');
    logger.info('  GET /health - Health check');
    logger.info('  GET /api/v1/models - Available models');
    logger.info('  GET /api/v1/metrics - General metrics');
    logger.info('  GET /api/v1/metrics/live-requests - Live request data with policy enforcement');
    logger.info('  GET /api/v1/metrics/dashboard - Dashboard statistics');
    logger.info('  GET /api/v1/policies - Get all policies');
    logger.info('  POST /api/v1/policies - Create new policy');
    logger.info('  PUT /api/v1/policies/:id - Update policy');
    logger.info('  DELETE /api/v1/policies/:id - Delete policy');
    logger.info('  GET /api/v1/qos/metrics - QoS metrics proxy');
    logger.info('  GET /api/v1/qos/health - QoS health proxy');
    logger.info('Socket.IO server enabled for real-time updates');
    
    // Connect to QoS service after server starts
    connectToQoSService();
  });
}

export default app;