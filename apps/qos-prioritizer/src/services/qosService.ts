import PQueue from 'p-queue';
import axios from 'axios';
import { EventEmitter } from 'events';
import { Server as SocketIOServer } from 'socket.io';
import { logger } from '../utils/logger';

interface QueueRequest {
  id: string;
  tier: 'enterprise' | 'premium' | 'free';
  prompt: string;
  timestamp: number;
  resolve: (response: any) => void;
  reject: (error: Error) => void;
  isAdvancedDemo?: boolean;
  isSimulationDemo?: boolean;
}

interface QoSMetrics {
  totalRequests: number;
  activeRequests: number;
  enterpriseQueue: number;
  premiumQueue: number;
  freeQueue: number;
  avgResponseTime: number;
}

interface DetailedQoSStats {
  timestamp: string;
  queues: {
    enterprise: {
      size: number;
      pending: number;
      concurrency: number;
      isPaused: boolean;
    };
    premium: {
      size: number;
      pending: number;
      concurrency: number;
      isPaused: boolean;
    };
    free: {
      size: number;
      pending: number;
      concurrency: number;
      isPaused: boolean;
    };
  };
  performance: {
    totalProcessed: number;
    processingRate: number;
    avgWaitTime: number;
    avgProcessingTime: number;
  };
  activeRequests: Array<{
    id: string;
    tier: string;
    startTime: number;
    waitTime: number;
  }>;
}

export class QoSService extends EventEmitter {
  private enterpriseQueue: PQueue;
  private premiumQueue: PQueue;
  private freeQueue: PQueue;
  private metrics: QoSMetrics;
  private simulationMode: boolean;
  private modelEndpoint: string;
  private io?: SocketIOServer;
  private activeRequests: Map<string, { id: string; tier: string; startTime: number; waitTime: number }>;
  private performanceHistory: Array<{ timestamp: number; processingTime: number; waitTime: number }>;
  private metricsInterval?: NodeJS.Timeout;
  
  constructor(io?: SocketIOServer) {
    super();
    this.io = io;
    this.simulationMode = process.env.SIMULATION_MODE === 'true';
    this.modelEndpoint = 'http://localhost:8004';
    this.activeRequests = new Map();
    this.performanceHistory = [];
    
    // Create priority queues with different concurrency
    this.enterpriseQueue = new PQueue({ 
      concurrency: 3         // Enterprise gets most resources
    });
    
    this.premiumQueue = new PQueue({ 
      concurrency: 2         // Premium gets medium resources  
    });
    
    this.freeQueue = new PQueue({ 
      concurrency: 1         // Free gets least resources
    });
    
    this.metrics = {
      totalRequests: 0,
      activeRequests: 0,
      enterpriseQueue: 0,
      premiumQueue: 0,
      freeQueue: 0,
      avgResponseTime: 0
    };
    
    this.setupEventHandlers();
    this.startMetricsBroadcast();
    
    logger.info('QoS Service initialized with p-queue', {
      simulationMode: this.simulationMode,
      endpoint: this.modelEndpoint,
      socketIO: !!this.io
    });
  }
  
  /**
   * Process request with proper priority queuing
   */
  async processRequest(
    id: string,
    tier: 'enterprise' | 'premium' | 'free',
    prompt: string,
    userContext: any
  ): Promise<any> {
    const startTime = Date.now();
    
    logger.info('Processing QoS request', {
      requestId: id,
      tier,
      queueSizes: this.getQueueSizes()
    });
    
    this.metrics.totalRequests++;
    
    // Track the request
    this.activeRequests.set(id, {
      id,
      tier,
      startTime,
      waitTime: 0
    });
    
    // Emit request started event
    this.broadcastEvent('request_queued', {
      requestId: id,
      tier,
      queueSizes: this.getQueueSizes(),
      timestamp: new Date().toISOString()
    });
    
    return new Promise((resolve, reject) => {
      const queueRequest: QueueRequest = {
        id,
        tier,
        prompt,
        timestamp: startTime,
        resolve,
        reject,
        isAdvancedDemo: userContext?.isAdvancedDemo || false,
        isSimulationDemo: userContext?.isSimulationDemo || false
      };
      
      // Route to appropriate priority queue
      const queue = this.selectQueue(tier);
      const priority = this.calculatePriority(tier, startTime);
      
      // Add to queue with priority
      queue.add(
        () => this.executeRequest(queueRequest),
        { priority }
      ).then(resolve).catch(reject);
      
      this.updateQueueMetrics();
      this.broadcastQueueUpdate();
    });
  }
  
  /**
   * Execute the actual request (simulation or real LLM)
   */
  private async executeRequest(request: QueueRequest): Promise<any> {
    const startTime = Date.now();
    this.metrics.activeRequests++;
    
    try {
      let response;
      
      // Check demo mode flags
      const isAdvancedDemo = request.isAdvancedDemo || false;
      const isSimulationDemo = request.isSimulationDemo || false;
      
      // Determine processing mode:
      // - isSimulationDemo=true: Force simulation (regardless of service mode)
      // - isAdvancedDemo=true: Force real LLM (regardless of service mode)  
      // - Otherwise: Use service default (simulationMode setting)
      if (isSimulationDemo || (this.simulationMode && !isAdvancedDemo)) {
        response = await this.simulateResponse(request);
      } else {
        response = await this.callLLM(request);
      }
      
      const processingTime = Date.now() - startTime;
      const queueTime = startTime - request.timestamp;
      
      // Store performance data
      this.performanceHistory.push({
        timestamp: Date.now(),
        processingTime,
        waitTime: queueTime
      });
      
      // Keep only last 100 entries
      if (this.performanceHistory.length > 100) {
        this.performanceHistory.shift();
      }
      
      logger.info('Request completed', {
        requestId: request.id,
        tier: request.tier,
        processingTime,
        queueTime,
        simulation: this.simulationMode
      });
      
      // Emit completion event
      this.broadcastEvent('request_completed', {
        requestId: request.id,
        tier: request.tier,
        processingTime,
        queueTime,
        queueSizes: this.getQueueSizes(),
        timestamp: new Date().toISOString()
      });
      
      this.updateMetrics(processingTime);
      this.broadcastQueueUpdate();
      return response;
      
    } finally {
      this.metrics.activeRequests--;
      this.activeRequests.delete(request.id);
    }
  }
  
  /**
   * Simulate LLM response for testing
   */
  private async simulateResponse(request: QueueRequest): Promise<any> {
    // Same processing time for all tiers to demonstrate pure QoS prioritization
    // (Real QoS benefits come from queue management, not faster hardware)
    const delay = 4000;
    
    await new Promise(resolve => setTimeout(resolve, delay));
    
    return {
      id: `sim-${Date.now()}`,
      object: 'chat.completion',
      created: Math.floor(Date.now() / 1000),
      model: 'gpt2-medium-sim',
      choices: [{
        index: 0,
        message: {
          role: 'assistant',
          content: `Simulated ${request.tier.toUpperCase()} response: ${request.prompt.substring(0, 50)}... [Priority processing completed]`
        },
        finish_reason: 'stop'
      }],
      usage: { prompt_tokens: 10, completion_tokens: 20, total_tokens: 30 }
    };
  }
  
  /**
   * Call real LLM model
   */
  private async callLLM(request: QueueRequest): Promise<any> {
    try {
      const response = await axios.post(`${this.modelEndpoint}/v1/chat/completions`, {
        model: 'gpt2-medium',
        messages: [{ role: 'user', content: request.prompt }],
        max_tokens: 50,
        temperature: 0.7
      }, {
        headers: { 'Content-Type': 'application/json' },
        timeout: 180000 // 3 minutes
      });
      
      return response.data;
      
    } catch (error: any) {
      logger.error('LLM request failed', {
        requestId: request.id,
        error: error.message,
        endpoint: this.modelEndpoint
      });
      throw error;
    }
  }
  
  /**
   * Select appropriate queue based on tier
   */
  private selectQueue(tier: string): PQueue {
    switch (tier) {
      case 'enterprise': return this.enterpriseQueue;
      case 'premium': return this.premiumQueue;  
      case 'free': return this.freeQueue;
      default: return this.freeQueue;
    }
  }
  
  /**
   * Calculate priority score (higher = more important)
   */
  private calculatePriority(tier: string, timestamp: number): number {
    const basePriority = {
      enterprise: 100,
      premium: 50,
      free: 10
    }[tier] || 10;
    
    // Add slight aging bonus to prevent starvation
    const age = Date.now() - timestamp;
    const agingBonus = Math.min(age / 1000, 20); // Max 20 point bonus
    
    return basePriority + agingBonus;
  }
  
  /**
   * Update queue size metrics (includes both waiting and processing)
   */
  private updateQueueMetrics(): void {
    // Include both queued (size) and currently processing (pending) requests
    this.metrics.enterpriseQueue = this.enterpriseQueue.size + this.enterpriseQueue.pending;
    this.metrics.premiumQueue = this.premiumQueue.size + this.premiumQueue.pending;
    this.metrics.freeQueue = this.freeQueue.size + this.freeQueue.pending;
  }
  
  /**
   * Update performance metrics
   */
  private updateMetrics(processingTime: number): void {
    // Simple moving average for response time
    this.metrics.avgResponseTime = 
      (this.metrics.avgResponseTime * 0.9) + (processingTime * 0.1);
  }
  
  /**
   * Get current queue sizes (includes both waiting and processing)
   */
  private getQueueSizes(): Record<string, number> {
    return {
      enterprise: this.enterpriseQueue.size + this.enterpriseQueue.pending,
      premium: this.premiumQueue.size + this.premiumQueue.pending,
      free: this.freeQueue.size + this.freeQueue.pending
    };
  }
  
  /**
   * Get comprehensive metrics
   */
  getMetrics(): QoSMetrics {
    this.updateQueueMetrics();
    return { ...this.metrics };
  }

  /**
   * Get detailed statistics for real-time monitoring
   */
  getDetailedStats(): DetailedQoSStats {
    this.updateQueueMetrics();
    
    const now = Date.now();
    const recentHistory = this.performanceHistory.filter(h => now - h.timestamp < 300000); // Last 5 minutes
    
    const avgWaitTime = recentHistory.length > 0 
      ? recentHistory.reduce((sum, h) => sum + h.waitTime, 0) / recentHistory.length 
      : 0;
      
    const avgProcessingTime = recentHistory.length > 0
      ? recentHistory.reduce((sum, h) => sum + h.processingTime, 0) / recentHistory.length
      : 0;
    
    const processingRate = recentHistory.length / 5; // Requests per minute over 5 minutes
    
    return {
      timestamp: new Date().toISOString(),
      queues: {
        enterprise: {
          size: this.enterpriseQueue.size,
          pending: this.enterpriseQueue.pending,
          concurrency: this.enterpriseQueue.concurrency,
          isPaused: this.enterpriseQueue.isPaused
        },
        premium: {
          size: this.premiumQueue.size,
          pending: this.premiumQueue.pending,
          concurrency: this.premiumQueue.concurrency,
          isPaused: this.premiumQueue.isPaused
        },
        free: {
          size: this.freeQueue.size,
          pending: this.freeQueue.pending,
          concurrency: this.freeQueue.concurrency,
          isPaused: this.freeQueue.isPaused
        }
      },
      performance: {
        totalProcessed: this.metrics.totalRequests,
        processingRate,
        avgWaitTime,
        avgProcessingTime
      },
      activeRequests: Array.from(this.activeRequests.values()).map(req => ({
        ...req,
        waitTime: Date.now() - req.startTime
      }))
    };
  }

  /**
   * Broadcast event to connected Socket.IO clients
   */
  private broadcastEvent(event: string, data: any): void {
    if (this.io) {
      this.io.emit(event, data);
    }
  }

  /**
   * Broadcast queue update to all clients
   */
  private broadcastQueueUpdate(): void {
    if (this.io) {
      this.io.emit('queue_update', this.getMetrics());
    }
  }

  /**
   * Start periodic metrics broadcasting
   */
  private startMetricsBroadcast(): void {
    if (this.io) {
      this.metricsInterval = setInterval(() => {
        this.broadcastEvent('queue_stats', this.getDetailedStats());
        this.broadcastQueueUpdate();
      }, 1000); // Broadcast every second
    }
  }
  
  /**
   * Setup event handlers for monitoring
   */
  private setupEventHandlers(): void {
    // Monitor queue events
    [this.enterpriseQueue, this.premiumQueue, this.freeQueue].forEach((queue, index) => {
      const tier = ['enterprise', 'premium', 'free'][index];
      
      queue.on('active', () => {
        logger.debug(`${tier} queue processing started`, {
          queueSize: queue.size,
          pending: queue.pending
        });
      });
      
      queue.on('idle', () => {
        logger.debug(`${tier} queue idle`, {
          queueSize: queue.size
        });
      });
    });
  }
  
  /**
   * Health check
   */
  async healthCheck(): Promise<{ status: string; metrics: QoSMetrics }> {
    return {
      status: 'healthy',
      metrics: this.getMetrics()
    };
  }
  
  /**
   * Graceful shutdown
   */
  async shutdown(): Promise<void> {
    logger.info('Shutting down QoS Service...');
    
    // Clear metrics interval
    if (this.metricsInterval) {
      clearInterval(this.metricsInterval);
    }
    
    // Wait for all queues to finish
    await Promise.all([
      this.enterpriseQueue.onIdle(),
      this.premiumQueue.onIdle(), 
      this.freeQueue.onIdle()
    ]);
    
    logger.info('QoS Service shutdown complete');
  }
}