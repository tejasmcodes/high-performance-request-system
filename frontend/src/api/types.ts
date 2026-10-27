export interface Algorithm {
  id: string;
  name: string;
}

export interface Scenario {
  id: string;
  name: string;
  description: string;
}

export interface ServerConfig {
  id: string;
  url: string;
  weight: number;
}

export interface BenchmarkRequest {
  algorithm: string;
  scenario: string;
  servers: ServerConfig[];
  duration_seconds: number;
  warmup_seconds: number;
  concurrency: number;
  rate_per_second: number;
}

export interface LatencyMetrics {
  count: number;
  mean_ms: number;
  p50_ms: number;
  p95_ms: number;
  p99_ms: number;
  min_ms: number;
  max_ms: number;
}

export interface WindowMetrics {
  second: number;
  requests: number;
  errors: number;
  latency: LatencyMetrics;
  backend_requests: Record<string, number>;
  backend_cpu?: Record<string, number> | null;
}

export interface BenchmarkResult {
  algorithm: string;
  scenario: string;
  duration_seconds: number;
  concurrency: number;
  rate_per_second: number;
  total_requests: number;
  completed_requests: number;
  failed_requests: number;
  throughput_rps: number;
  error_rate: number;
  latency: LatencyMetrics;
  server_distribution: Record<string, number>;
  windows: WindowMetrics[];
}

export interface ComparisonResultInput {
  algorithm: string;
  throughput_rps?: number;
  avg_latency_ms?: number;
  p95_latency_ms?: number;
  p99_latency_ms?: number;
  error_rate?: number;
}

export interface MetricWinner {
  algorithm: string;
  value: number;
  improvement_percent?: number;
}

export interface ComparisonResponse {
  reference_algorithm: string;
  best_throughput?: MetricWinner;
  lowest_avg_latency?: MetricWinner;
  lowest_p95_latency?: MetricWinner;
  lowest_p99_latency?: MetricWinner;
  lowest_error_rate?: MetricWinner;
}

export interface ApiErrorResponse {
  error?: string;
  message?: string;
}
