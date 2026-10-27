import type {
  Algorithm,
  ApiErrorResponse,
  BenchmarkRequest,
  BenchmarkResult,
  ComparisonResponse,
  ComparisonResultInput,
  Scenario,
} from "./types";

const API_BASE_URL = (
  import.meta.env.VITE_API_BASE_URL ?? "http://127.0.0.1:8082"
).replace(/\/+$/, "");

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, init);
  } catch {
    throw new Error(
      `Could not connect to the benchmark API at ${API_BASE_URL}. Check that it is running.`,
    );
  }

  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as ApiErrorResponse | null;
    throw new Error(body?.message || `The API request failed (${response.status}).`);
  }
  return (await response.json()) as T;
}

export async function getAlgorithms(): Promise<Algorithm[]> {
  const response = await request<{ algorithms: Algorithm[] }>("/api/algorithms");
  return response.algorithms;
}

export async function getScenarios(): Promise<Scenario[]> {
  const response = await request<{ scenarios: Scenario[] }>("/api/scenarios");
  return response.scenarios;
}

export function runBenchmark(config: BenchmarkRequest): Promise<BenchmarkResult> {
  return request<BenchmarkResult>("/api/benchmarks", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
}

export function compareBenchmarks(
  results: ComparisonResultInput[],
): Promise<ComparisonResponse> {
  return request<ComparisonResponse>("/api/benchmarks/compare", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ results }),
  });
}

export { API_BASE_URL };
