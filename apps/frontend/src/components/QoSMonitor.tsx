import React, { useState, useEffect, useRef } from 'react';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Grid,
  LinearProgress,
  Chip,
  Alert,
  Switch,
  FormControlLabel,
  Button,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
} from '@mui/material';
import {
  Speed as QoSIcon,
  Refresh,
  Wifi as ConnectedIcon,
  WifiOff as DisconnectedIcon,
} from '@mui/icons-material';
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';
import { io, Socket } from 'socket.io-client';

interface QoSMetrics {
  totalRequests: number;
  activeRequests: number;
  enterpriseQueue: number;
  premiumQueue: number;
  freeQueue: number;
  avgResponseTime: number;
}

interface QueueStats {
  size: number;
  pending: number;
  concurrency: number;
  isPaused: boolean;
}

interface DetailedStats {
  timestamp: string;
  queues: {
    enterprise: QueueStats;
    premium: QueueStats;
    free: QueueStats;
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

interface RequestEvent {
  requestId: string;
  tier: string;
  timestamp: string;
  type?: string;
  queueSizes?: any;
  processingTime?: number;
  queueTime?: number;
}

export default function QoSMonitor() {
  const [isConnected, setIsConnected] = useState(false);
  const [metrics, setMetrics] = useState<QoSMetrics | null>(null);
  const [detailedStats, setDetailedStats] = useState<DetailedStats | null>(null);
  const [realtimeEnabled, setRealtimeEnabled] = useState(true);
  const [queueHistory, setQueueHistory] = useState<any[]>([]);
  const [recentEvents, setRecentEvents] = useState<RequestEvent[]>([]);
  const [error, setError] = useState<string | null>(null);
  
  const socketRef = useRef<Socket | null>(null);
  const maxHistoryItems = 30;
  const maxEvents = 20;

  useEffect(() => {
    if (realtimeEnabled) {
      connectSocket();
    } else {
      disconnectSocket();
    }

    return () => {
      disconnectSocket();
    };
  }, [realtimeEnabled]);

  const connectSocket = () => {
    try {
      socketRef.current = io('http://localhost:3001', {
        transports: ['websocket'],
        forceNew: true,
      });

      const socket = socketRef.current;

      socket.on('connect', () => {
        setIsConnected(true);
        setError(null);
        // Subscribe to QoS updates
        socket.emit('subscribe_qos');
      });

      socket.on('disconnect', () => {
        setIsConnected(false);
      });

      socket.on('connect_error', (error) => {
        setError(`Connection error: ${error.message}`);
        setIsConnected(false);
      });

      // QoS event handlers
      socket.on('qos_queue_update', (data: QoSMetrics) => {
        setMetrics(data);
        
        // Add to queue history for charts
        const historyItem = {
          timestamp: new Date().toLocaleTimeString(),
          enterprise: data.enterpriseQueue,
          premium: data.premiumQueue,
          free: data.freeQueue,
          total: data.enterpriseQueue + data.premiumQueue + data.freeQueue,
        };
        
        setQueueHistory(prev => {
          const newHistory = [...prev, historyItem];
          return newHistory.slice(-maxHistoryItems);
        });
      });

      socket.on('qos_queue_stats', (data: DetailedStats) => {
        setDetailedStats(data);
      });

      socket.on('qos_request_queued', (data: RequestEvent) => {
        setRecentEvents(prev => {
          const newEvents = [{ ...data, type: 'queued' }, ...prev];
          return newEvents.slice(0, maxEvents);
        });
      });

      socket.on('qos_request_completed', (data: RequestEvent) => {
        setRecentEvents(prev => {
          const newEvents = [{ ...data, type: 'completed' }, ...prev];
          return newEvents.slice(0, maxEvents);
        });
      });

    } catch (error) {
      setError(`Failed to connect: ${error}`);
    }
  };

  const disconnectSocket = () => {
    if (socketRef.current) {
      socketRef.current.emit('unsubscribe_qos');
      socketRef.current.disconnect();
      socketRef.current = null;
    }
    setIsConnected(false);
  };

  const refreshData = async () => {
    try {
      const response = await fetch('/api/v1/qos/metrics');
      const data = await response.json();
      if (data.success) {
        setMetrics(data.data);
      }
    } catch (error) {
      setError(`Failed to fetch data: ${error}`);
    }
  };

  const getTierColor = (tier: string) => {
    switch (tier) {
      case 'enterprise': return '#FFD700'; // Gold
      case 'premium': return '#C0C0C0';    // Silver
      case 'free': return '#CD7F32';       // Bronze
      default: return '#999';
    }
  };

  const formatWaitTime = (waitTime: number) => {
    if (waitTime < 1000) return `${waitTime}ms`;
    return `${(waitTime / 1000).toFixed(1)}s`;
  };

  return (
    <Box>
      {/* Header */}
      <Box sx={{ mb: 3, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <QoSIcon sx={{ mr: 1, color: 'primary.main' }} />
          <Typography variant="h4" sx={{ fontWeight: 600 }}>
            QoS Monitor
          </Typography>
          <Chip
            icon={isConnected ? <ConnectedIcon /> : <DisconnectedIcon />}
            label={isConnected ? 'Connected' : 'Disconnected'}
            color={isConnected ? 'success' : 'error'}
            size="small"
            sx={{ ml: 2 }}
          />
        </Box>
        
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <FormControlLabel
            control={
              <Switch
                checked={realtimeEnabled}
                onChange={(e) => setRealtimeEnabled(e.target.checked)}
                color="primary"
              />
            }
            label="Real-time Updates"
          />
          <Button
            variant="outlined"
            startIcon={<Refresh />}
            onClick={refreshData}
            disabled={realtimeEnabled}
          >
            Refresh
          </Button>
        </Box>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {error}
        </Alert>
      )}

      {/* Metrics Overview */}
      <Grid container spacing={3} sx={{ mb: 3 }}>
        <Grid item xs={12} md={3}>
          <Card>
            <CardContent>
              <Typography variant="h6" color="primary">Total Requests</Typography>
              <Typography variant="h4">{metrics?.totalRequests || 0}</Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} md={3}>
          <Card>
            <CardContent>
              <Typography variant="h6" color="success.main">Active</Typography>
              <Typography variant="h4">{metrics?.activeRequests || 0}</Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} md={3}>
          <Card>
            <CardContent>
              <Typography variant="h6" color="warning.main">Total Queued</Typography>
              <Typography variant="h4">
                {(metrics?.enterpriseQueue || 0) + (metrics?.premiumQueue || 0) + (metrics?.freeQueue || 0)}
              </Typography>
            </CardContent>
          </Card>
        </Grid>
        <Grid item xs={12} md={3}>
          <Card>
            <CardContent>
              <Typography variant="h6" color="info.main">Avg Response</Typography>
              <Typography variant="h4">
                {metrics?.avgResponseTime ? `${Math.round(metrics.avgResponseTime)}ms` : '0ms'}
              </Typography>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      {/* Queue Status Cards */}
      <Grid container spacing={3} sx={{ mb: 3 }}>
        {['enterprise', 'premium', 'free'].map((tier) => {
          const queueSize = metrics?.[`${tier}Queue` as keyof QoSMetrics] as number || 0;
          const queueStats = detailedStats?.queues?.[tier as keyof typeof detailedStats.queues];
          
          return (
            <Grid item xs={12} md={4} key={tier}>
              <Card sx={{ borderLeft: `4px solid ${getTierColor(tier)}` }}>
                <CardContent>
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                    <Typography variant="h6" sx={{ textTransform: 'capitalize', color: getTierColor(tier) }}>
                      {tier} Queue
                    </Typography>
                    <Chip 
                      label={queueStats?.isPaused ? 'Paused' : 'Active'}
                      color={queueStats?.isPaused ? 'error' : 'success'}
                      size="small"
                    />
                  </Box>
                  
                  <Typography variant="h3" sx={{ mb: 1 }}>
                    {queueSize}
                  </Typography>
                  
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    requests in queue
                  </Typography>
                  
                  {queueStats && (
                    <Box>
                      <Typography variant="body2">
                        Processing: {queueStats.pending} / {queueStats.concurrency}
                      </Typography>
                      <LinearProgress
                        variant="determinate"
                        value={(queueStats.pending / queueStats.concurrency) * 100}
                        sx={{ mt: 1, height: 6, borderRadius: 3 }}
                      />
                    </Box>
                  )}
                </CardContent>
              </Card>
            </Grid>
          );
        })}
      </Grid>

      {/* Charts */}
      <Grid container spacing={3} sx={{ mb: 3 }}>
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Typography variant="h6" sx={{ mb: 2 }}>Queue Sizes Over Time</Typography>
              <ResponsiveContainer width="100%" height={300}>
                <LineChart data={queueHistory}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="timestamp" />
                  <YAxis />
                  <RechartsTooltip />
                  <Legend />
                  <Line type="monotone" dataKey="enterprise" stroke={getTierColor('enterprise')} strokeWidth={2} />
                  <Line type="monotone" dataKey="premium" stroke={getTierColor('premium')} strokeWidth={2} />
                  <Line type="monotone" dataKey="free" stroke={getTierColor('free')} strokeWidth={2} />
                </LineChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        </Grid>
        
        <Grid item xs={12} md={6}>
          <Card>
            <CardContent>
              <Typography variant="h6" sx={{ mb: 2 }}>Current Queue Distribution</Typography>
              <ResponsiveContainer width="100%" height={300}>
                <BarChart data={[
                  { name: 'Enterprise', value: metrics?.enterpriseQueue || 0, fill: getTierColor('enterprise') },
                  { name: 'Premium', value: metrics?.premiumQueue || 0, fill: getTierColor('premium') },
                  { name: 'Free', value: metrics?.freeQueue || 0, fill: getTierColor('free') },
                ]}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="name" />
                  <YAxis />
                  <RechartsTooltip />
                  <Bar dataKey="value" />
                </BarChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      {/* Performance Stats */}
      {detailedStats && (
        <Grid container spacing={3} sx={{ mb: 3 }}>
          <Grid item xs={12} md={6}>
            <Card>
              <CardContent>
                <Typography variant="h6" sx={{ mb: 2 }}>Performance Statistics</Typography>
                <Grid container spacing={2}>
                  <Grid item xs={6}>
                    <Typography variant="body2" color="text.secondary">Processing Rate</Typography>
                    <Typography variant="h6">{detailedStats.performance.processingRate.toFixed(1)} req/min</Typography>
                  </Grid>
                  <Grid item xs={6}>
                    <Typography variant="body2" color="text.secondary">Avg Wait Time</Typography>
                    <Typography variant="h6">{formatWaitTime(detailedStats.performance.avgWaitTime)}</Typography>
                  </Grid>
                  <Grid item xs={6}>
                    <Typography variant="body2" color="text.secondary">Avg Processing Time</Typography>
                    <Typography variant="h6">{formatWaitTime(detailedStats.performance.avgProcessingTime)}</Typography>
                  </Grid>
                  <Grid item xs={6}>
                    <Typography variant="body2" color="text.secondary">Total Processed</Typography>
                    <Typography variant="h6">{detailedStats.performance.totalProcessed}</Typography>
                  </Grid>
                </Grid>
              </CardContent>
            </Card>
          </Grid>
          
          <Grid item xs={12} md={6}>
            <Card>
              <CardContent>
                <Typography variant="h6" sx={{ mb: 2 }}>Active Requests</Typography>
                {detailedStats.activeRequests.length === 0 ? (
                  <Typography variant="body2" color="text.secondary">No active requests</Typography>
                ) : (
                  <TableContainer>
                    <Table size="small">
                      <TableHead>
                        <TableRow>
                          <TableCell>Request ID</TableCell>
                          <TableCell>Tier</TableCell>
                          <TableCell>Wait Time</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {detailedStats.activeRequests.slice(0, 5).map((request) => (
                          <TableRow key={request.id}>
                            <TableCell sx={{ fontFamily: 'monospace' }}>
                              {request.id.substring(0, 8)}...
                            </TableCell>
                            <TableCell>
                              <Chip 
                                label={request.tier}
                                size="small"
                                sx={{ backgroundColor: getTierColor(request.tier), color: 'white' }}
                              />
                            </TableCell>
                            <TableCell>{formatWaitTime(request.waitTime)}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </TableContainer>
                )}
              </CardContent>
            </Card>
          </Grid>
        </Grid>
      )}

      {/* Recent Events */}
      <Card>
        <CardContent>
          <Typography variant="h6" sx={{ mb: 2 }}>Recent Events</Typography>
          {recentEvents.length === 0 ? (
            <Typography variant="body2" color="text.secondary">No recent events</Typography>
          ) : (
            <TableContainer>
              <Table>
                <TableHead>
                  <TableRow>
                    <TableCell>Time</TableCell>
                    <TableCell>Event</TableCell>
                    <TableCell>Request ID</TableCell>
                    <TableCell>Tier</TableCell>
                    <TableCell>Details</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {recentEvents.map((event, index) => (
                    <TableRow key={index}>
                      <TableCell>{new Date(event.timestamp).toLocaleTimeString()}</TableCell>
                      <TableCell>
                        <Chip 
                          label={event.type || 'unknown'}
                          color={event.type === 'completed' ? 'success' : 'warning'}
                          size="small"
                        />
                      </TableCell>
                      <TableCell sx={{ fontFamily: 'monospace' }}>
                        {event.requestId.substring(0, 12)}...
                      </TableCell>
                      <TableCell>
                        <Chip 
                          label={event.tier}
                          size="small"
                          sx={{ backgroundColor: getTierColor(event.tier), color: 'white' }}
                        />
                      </TableCell>
                      <TableCell>
                        {event.processingTime && `Processing: ${event.processingTime}ms`}
                        {event.queueTime && ` | Queue: ${event.queueTime}ms`}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}