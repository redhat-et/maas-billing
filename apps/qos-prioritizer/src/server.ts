import express from 'express';
import cors from 'cors';
import { createServer } from 'http';
import { Server as SocketIOServer } from 'socket.io';
import { QoSService } from './services/qosService';
import { logger } from './utils/logger';

const app = express();
const server = createServer(app);
const io = new SocketIOServer(server, {
  cors: {
    origin: ["http://localhost:3000", "http://localhost:3001"],
    methods: ["GET", "POST"]
  }
});
const PORT = process.env.PORT || 3005;

// Middleware
app.use(cors());
app.use(express.json());

// Request logging middleware
app.use((req, res, next) => {
  logger.info('HTTP Request', {
    method: req.method,
    path: req.path,
    ip: req.ip,
    userAgent: req.get('User-Agent')
  });
  next();
});

// Initialize QoS service with Socket.IO
const qosService = new QoSService(io);

/**
 * Extract user context from Authorino headers
 */
function extractUserContext(headers: Record<string, string>): { tier: 'enterprise' | 'premium' | 'free'; userId: string } {
  try {
    const authIdentity = headers['x-auth-identity'];
    if (!authIdentity) {
      return { tier: 'free', userId: 'anonymous' };
    }
    
    const identity = JSON.parse(authIdentity);
    const groups = identity.metadata?.annotations?.['kuadrant.io/groups'] || '';
    const userId = identity.metadata?.annotations?.['secret.kuadrant.io/user-id'] || 'unknown';
    
    // Determine tier from groups
    const tier = groups.toLowerCase().includes('enterprise') ? 'enterprise' :
                 groups.toLowerCase().includes('premium') ? 'premium' : 'free';
    
    return { tier, userId };
  } catch (error) {
    logger.warn('Failed to parse user context, defaulting to free tier', { error });
    return { tier: 'free', userId: 'unknown' };
  }
}

/**
 * Health check endpoint
 */
app.get('/health', async (req, res) => {
  try {
    const health = await qosService.healthCheck();
    res.json(health);
  } catch (error) {
    res.status(500).json({ status: 'error', error: error instanceof Error ? error.message : String(error) });
  }
});

/**
 * QoS metrics endpoint
 */
app.get('/metrics', (req, res) => {
  const metrics = qosService.getMetrics();
  res.json(metrics);
});

/**
 * OpenAI-compatible chat completions endpoint with QoS
 */
app.post('/v1/chat/completions', async (req, res) => {
  const startTime = Date.now();
  const requestId = `req-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
  
  try {
    // Extract user context
    const { tier, userId } = extractUserContext(req.headers as Record<string, string>);
    
    // Extract prompt from OpenAI format
    const { messages, model = 'gpt2-medium' } = req.body;
    const prompt = messages?.[messages.length - 1]?.content || 'Hello';
    
    logger.info('Processing OpenAI chat completion request', {
      requestId,
      model,
      tier,
      userId
    });
    
    // Check demo mode: 'advanced' forces real LLM, 'simulation' forces simulation
    const demoMode = req.headers['x-demo-mode'] as string;
    const isAdvancedDemo = demoMode === 'advanced';
    const isSimulationDemo = demoMode === 'simulation';
    
    // Process through QoS system
    const response = await qosService.processRequest(requestId, tier, prompt, { userId, tier, isAdvancedDemo, isSimulationDemo });
    
    const totalTime = Date.now() - startTime;
    logger.info('Request completed successfully', {
      requestId,
      tier,
      totalTime,
      model
    });
    
    res.json(response);
    
  } catch (error: unknown) {
    const totalTime = Date.now() - startTime;
    logger.error('Request failed', {
      requestId,
      error: error instanceof Error ? error.message : String(error),
      totalTime
    });
    
    res.status(500).json({
      error: {
        message: error instanceof Error ? error.message : String(error),
        type: 'qos_error',
        code: 'processing_failed'
      }
    });
  }
});

/**
 * Graceful shutdown
 */
process.on('SIGTERM', async () => {
  logger.info('SIGTERM received, shutting down gracefully...');
  await qosService.shutdown();
  process.exit(0);
});

process.on('SIGINT', async () => {
  logger.info('SIGINT received, shutting down gracefully...');
  await qosService.shutdown();
  process.exit(0);
});

// Socket.IO connection handling
io.on('connection', (socket) => {
  logger.info('Client connected to QoS service', {
    socketId: socket.id,
    clientIP: socket.handshake.address
  });

  // Send current metrics immediately on connection
  socket.emit('queue_update', qosService.getMetrics());
  socket.emit('queue_stats', qosService.getDetailedStats());

  socket.on('disconnect', () => {
    logger.info('Client disconnected from QoS service', {
      socketId: socket.id
    });
  });

  // Handle client requests for metrics
  socket.on('get_metrics', () => {
    socket.emit('queue_update', qosService.getMetrics());
  });

  socket.on('get_detailed_stats', () => {
    socket.emit('queue_stats', qosService.getDetailedStats());
  });
});

// Start server
server.listen(PORT, () => {
  logger.info('QoS Prioritizer service started', {
    port: PORT,
    environment: process.env.NODE_ENV || 'development',
    simulationMode: process.env.SIMULATION_MODE === 'true',
    logLevel: 'info',
    socketIO: 'enabled'
  });
});

export default app;