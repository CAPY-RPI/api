# API Route Benchmarks

This directory contains performance benchmarks for the CAPY API routes using Go's native `testing.B` framework.

## Overview

Benchmarks measure the performance of API endpoints in terms of:
- **Response Time**: Nanoseconds per operation (ns/op).
- **Memory Usage**: Bytes allocated per operation (B/op).
- **Allocations**: Number of distinct allocations (allocs/op).

Benchmarks run against a real PostgreSQL instance using `testcontainers`, ensuring realistic performance data while maintaining isolation.

## Usage

To run all benchmarks, use the following Make command:

```bash
make benchmark
```

Results are displayed on the console and automatically saved to timestamped log files in `benchmarks/results/`.

## Interpreting Results

Sample output:
```
BenchmarkHealthEndpoint-10      15085    78187 ns/op    7146 B/op    80 allocs/op
```

- **15085**: The number of iterations run to get a stable measurement.
- **78187 ns/op**: Average time per request (0.078 ms).
- **7146 B/op**: Average heap memory allocated per request.
- **80 allocs/op**: Average number of heap allocations per request.

## Benchmark Scope

Currently covered routes:
- `GET /health`: Basic health check performance.
- `GET /v1/auth/google`: Google OAuth initiation performance.
- `GET /v1/auth/microsoft`: Microsoft OAuth initiation performance.

*Note: Protected routes requiring JWT or M2M tokens are currently excluded from this minimal implementation.*

## Output Files

Benchmark runs are logged to:
`benchmarks/results/benchmark-YYYY-MM-DD-HHMMSS.txt`

These files are ignored by git to keep the repository clean.

## Future Enhancements

- **Protected Routes**: Add benchmarks for authenticated endpoints.
- **Regression Detection**: Compare current runs against a baseline.
- **Performance Thresholds**: Define p99 latency targets and fail benchmarks if exceeded.
- **Concurrent Load Testing**: Use multiple goroutines to simulate concurrent users.
