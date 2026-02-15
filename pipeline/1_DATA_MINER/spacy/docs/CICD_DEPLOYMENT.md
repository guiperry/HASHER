# CI/CD and Deployment Guide

This guide covers continuous integration, continuous deployment, and production deployment strategies for the Go-Spacy project.

## Table of Contents

- [CI/CD Pipeline Overview](#cicd-pipeline-overview)
- [GitHub Actions Setup](#github-actions-setup)
- [Testing Strategy](#testing-strategy)
- [Build and Release Process](#build-and-release-process)
- [Deployment Options](#deployment-options)
- [Production Considerations](#production-considerations)
- [Monitoring and Logging](#monitoring-and-logging)
- [Security Best Practices](#security-best-practices)

## CI/CD Pipeline Overview

Our CI/CD pipeline ensures code quality, runs comprehensive tests, and automates releases. The pipeline consists of:

1. **Code Quality Checks**: Linting, formatting, security scanning
2. **Multi-Platform Testing**: Linux, macOS, Windows (WSL2)
3. **Multi-Version Testing**: Go 1.16+, Python 3.7-3.11
4. **Performance Testing**: Benchmarks and memory profiling
5. **Security Scanning**: Dependency vulnerabilities, static analysis
6. **Automated Releases**: Semantic versioning and changelog generation

## GitHub Actions Setup

### Workflow Configuration

Create `.github/workflows/ci.yml`:

```yaml
name: CI/CD Pipeline

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]
  release:
    types: [ published ]

env:
  GO_VERSION: '1.21'
  PYTHON_VERSION: '3.9'

jobs:
  lint:
    name: Code Quality
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v3
        with:
          version: latest
          args: --timeout=5m

      - name: Check formatting
        run: |
          gofmt -s -l . | tee /tmp/gofmt.out
          test ! -s /tmp/gofmt.out

      - name: Run go vet
        run: go vet ./...

      - name: Run staticcheck
        uses: dominikh/staticcheck-action@v1.3.0

  test-matrix:
    name: Test Matrix
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
        go-version: ['1.18', '1.19', '1.20', '1.21']
        python-version: ['3.8', '3.9', '3.10', '3.11']
        exclude:
          # Reduce matrix size for resource efficiency
          - os: macos-latest
            go-version: '1.18'
          - os: macos-latest
            python-version: '3.8'

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: ${{ matrix.go-version }}

      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: ${{ matrix.python-version }}

      - name: Install system dependencies (Ubuntu)
        if: matrix.os == 'ubuntu-latest'
        run: |
          sudo apt-get update
          sudo apt-get install -y build-essential python3-dev pkg-config

      - name: Install system dependencies (macOS)
        if: matrix.os == 'macos-latest'
        run: |
          brew install pkg-config

      - name: Install Python dependencies
        run: |
          pip install spacy
          python -m spacy download en_core_web_sm

      - name: Cache Go modules
        uses: actions/cache@v3
        with:
          path: ~/go/pkg/mod
          key: ${{ runner.os }}-go-${{ matrix.go-version }}-${{ hashFiles('**/go.sum') }}

      - name: Build
        run: |
          export CGO_ENABLED=1
          make clean && make

      - name: Run tests
        run: |
          export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$PWD/lib
          export DYLD_LIBRARY_PATH=$DYLD_LIBRARY_PATH:$PWD/lib
          go test -v -race -coverprofile=coverage.out ./...

      - name: Upload coverage to Codecov
        if: matrix.os == 'ubuntu-latest' && matrix.go-version == '1.21' && matrix.python-version == '3.9'
        uses: codecov/codecov-action@v3
        with:
          file: ./coverage.out
          flags: unittests
          name: codecov-umbrella

  benchmark:
    name: Performance Benchmarks
    runs-on: ubuntu-latest
    needs: [lint]
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Needed for benchmark comparison

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: ${{ env.PYTHON_VERSION }}

      - name: Install dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y build-essential python3-dev pkg-config
          pip install spacy
          python -m spacy download en_core_web_sm en_core_web_md

      - name: Build
        run: |
          export CGO_ENABLED=1
          make clean && make

      - name: Run benchmarks
        run: |
          export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$PWD/lib
          go test -bench=. -benchmem -run=^$ ./... > benchmark.txt

      - name: Compare benchmarks
        if: github.event_name == 'pull_request'
        run: |
          # Install benchcmp tool
          go install golang.org/x/tools/cmd/benchcmp@latest

          # Get baseline benchmarks from main branch
          git checkout main
          make clean && make
          export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$PWD/lib
          go test -bench=. -benchmem -run=^$ ./... > benchmark-main.txt

          # Compare benchmarks
          benchcmp benchmark-main.txt benchmark.txt

      - name: Upload benchmark results
        uses: actions/upload-artifact@v3
        with:
          name: benchmark-results
          path: benchmark.txt

  security:
    name: Security Scanning
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Run Gosec Security Scanner
        uses: securecodewarrior/github-action-gosec@master
        with:
          args: '-fmt sarif -out gosec.sarif ./...'

      - name: Upload SARIF file
        uses: github/codeql-action/upload-sarif@v2
        with:
          sarif_file: gosec.sarif

      - name: Run Govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...

  multi-language:
    name: Multi-Language Testing
    runs-on: ubuntu-latest
    if: github.event_name == 'push' || contains(github.event.pull_request.labels.*.name, 'multilang')
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: ${{ env.PYTHON_VERSION }}

      - name: Install dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y build-essential python3-dev pkg-config
          pip install spacy

      - name: Install language models
        run: |
          python -m spacy download en_core_web_sm
          python -m spacy download de_core_news_sm
          python -m spacy download fr_core_news_sm
          python -m spacy download es_core_news_sm

      - name: Build and test
        run: |
          export CGO_ENABLED=1
          make clean && make
          export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:$PWD/lib
          go test -v -run TestMultiLanguage ./...

  build-artifacts:
    name: Build Release Artifacts
    runs-on: ${{ matrix.os }}
    if: github.event_name == 'release'
    needs: [test-matrix, security, benchmark]
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
        include:
          - os: ubuntu-latest
            artifact_name: linux-amd64
            library_name: libspacy_wrapper.so
          - os: macos-latest
            artifact_name: darwin-amd64
            library_name: libspacy_wrapper.dylib

    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: ${{ env.PYTHON_VERSION }}

      - name: Install dependencies
        run: |
          if [[ "${{ matrix.os }}" == "ubuntu-latest" ]]; then
            sudo apt-get update
            sudo apt-get install -y build-essential python3-dev pkg-config
          else
            brew install pkg-config
          fi
          pip install spacy

      - name: Build optimized
        run: |
          export CGO_ENABLED=1
          export CFLAGS="-O3 -march=native -DNDEBUG"
          export CXXFLAGS="-O3 -march=native -DNDEBUG"
          make clean && make

      - name: Package artifact
        run: |
          mkdir -p dist/${{ matrix.artifact_name }}
          cp lib/${{ matrix.library_name }} dist/${{ matrix.artifact_name }}/
          cp README.md LICENSE dist/${{ matrix.artifact_name }}/
          cd dist && tar -czf go-spacy-${{ matrix.artifact_name }}.tar.gz ${{ matrix.artifact_name }}

      - name: Upload artifact
        uses: actions/upload-artifact@v3
        with:
          name: go-spacy-${{ matrix.artifact_name }}
          path: dist/go-spacy-${{ matrix.artifact_name }}.tar.gz

  release:
    name: GitHub Release
    runs-on: ubuntu-latest
    if: github.event_name == 'release'
    needs: [build-artifacts]
    steps:
      - name: Download all artifacts
        uses: actions/download-artifact@v3

      - name: Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            go-spacy-linux-amd64/go-spacy-linux-amd64.tar.gz
            go-spacy-darwin-amd64/go-spacy-darwin-amd64.tar.gz
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Additional Workflows

Create `.github/workflows/dependency-update.yml`:

```yaml
name: Dependency Updates

on:
  schedule:
    - cron: '0 0 * * 1'  # Weekly on Mondays
  workflow_dispatch:

jobs:
  update-go-dependencies:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Update Go dependencies
        run: |
          go get -u ./...
          go mod tidy

      - name: Create Pull Request
        uses: peter-evans/create-pull-request@v5
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          commit-message: 'chore: update Go dependencies'
          title: 'Automated Go dependency updates'
          body: |
            This PR contains automated updates to Go dependencies.

            Please review the changes and ensure all tests pass before merging.
          branch: automated/go-deps-update
```

## Testing Strategy

### Test Types and Coverage

1. **Unit Tests**: Individual function testing
2. **Integration Tests**: End-to-end workflow testing
3. **Performance Tests**: Benchmarks and memory profiling
4. **Multi-language Tests**: Cross-language functionality
5. **Security Tests**: Vulnerability scanning

### Test Configuration

Create `.golangci.yml`:

```yaml
run:
  timeout: 5m
  issues-exit-code: 1
  tests: true

output:
  format: colored-line-number
  print-issued-lines: true
  print-linter-name: true

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - typecheck
    - unused
    - gosec
    - gofmt
    - goimports
    - misspell
    - lll
    - unconvert
    - dupl
    - goconst
    - gocyclo
    - unparam

linters-settings:
  errcheck:
    check-type-assertions: true
    check-blank: true
  gosec:
    severity: medium
    confidence: medium
  lll:
    line-length: 120
  gocyclo:
    min-complexity: 15
```

## Build and Release Process

### Semantic Versioning

We follow [Semantic Versioning](https://semver.org/):
- **MAJOR.MINOR.PATCH** (e.g., 1.2.3)
- **Breaking changes**: Increment MAJOR
- **New features**: Increment MINOR
- **Bug fixes**: Increment PATCH

### Release Automation

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Generate Changelog
        id: changelog
        uses: requarks/changelog-action@v1
        with:
          token: ${{ github.token }}
          tag: ${{ github.ref_name }}

      - name: Create Release
        uses: ncipollo/release-action@v1
        with:
          allowUpdates: true
          body: ${{ steps.changelog.outputs.changes }}
          name: ${{ github.ref_name }}
          token: ${{ secrets.GITHUB_TOKEN }}
```

## Deployment Options

### 1. Direct Integration

For applications that directly embed Go-Spacy:

```dockerfile
FROM golang:1.21-bullseye as builder

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    python3-dev \
    python3-pip \
    pkg-config

# Install Python dependencies
RUN pip3 install spacy && \
    python3 -m spacy download en_core_web_sm

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 make clean && make
RUN go build -o main .

# Runtime stage
FROM debian:bullseye-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Python runtime dependencies
RUN pip3 install spacy && \
    python3 -m spacy download en_core_web_sm

# Copy application and libraries
COPY --from=builder /app/main /usr/local/bin/
COPY --from=builder /app/lib/ /usr/local/lib/

# Set library path
ENV LD_LIBRARY_PATH=/usr/local/lib

# Create non-root user
RUN useradd -r -s /bin/false appuser
USER appuser

EXPOSE 8080
CMD ["main"]
```

### 2. Microservice Architecture

For microservice deployments:

```dockerfile
# Dockerfile.nlp-service
FROM golang:1.21-bullseye as builder

WORKDIR /app

# Install dependencies
RUN apt-get update && apt-get install -y \
    build-essential python3-dev python3-pip pkg-config

RUN pip3 install spacy && \
    python3 -m spacy download en_core_web_sm en_core_web_md

# Build service
COPY . .
RUN CGO_ENABLED=1 make clean && make
RUN go build -o nlp-service ./cmd/service

# Runtime
FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y \
    python3 python3-pip ca-certificates && \
    pip3 install spacy && \
    python3 -m spacy download en_core_web_sm en_core_web_md && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/nlp-service /usr/local/bin/
COPY --from=builder /app/lib/ /usr/local/lib/

ENV LD_LIBRARY_PATH=/usr/local/lib
EXPOSE 8080

CMD ["nlp-service"]
```

### 3. Kubernetes Deployment

Create Kubernetes manifests:

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-spacy-app
  labels:
    app: go-spacy-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: go-spacy-app
  template:
    metadata:
      labels:
        app: go-spacy-app
    spec:
      containers:
      - name: app
        image: your-repo/go-spacy-app:latest
        ports:
        - containerPort: 8080
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        env:
        - name: LOG_LEVEL
          value: "info"
        - name: SPACY_MODEL
          value: "en_core_web_sm"
---
apiVersion: v1
kind: Service
metadata:
  name: go-spacy-service
spec:
  selector:
    app: go-spacy-app
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: go-spacy-ingress
  annotations:
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - api.your-domain.com
    secretName: api-tls
  rules:
  - host: api.your-domain.com
    http:
      paths:
      - path: /nlp
        pathType: Prefix
        backend:
          service:
            name: go-spacy-service
            port:
              number: 80
```

### 4. AWS Lambda Deployment

For serverless deployments:

```dockerfile
# Dockerfile.lambda
FROM public.ecr.aws/lambda/provided:al2

# Install dependencies
RUN yum update -y && \
    yum install -y gcc gcc-c++ python3-devel && \
    pip3 install spacy && \
    python3 -m spacy download en_core_web_sm

# Copy function code and libraries
COPY main ${LAMBDA_RUNTIME_DIR}
COPY lib/ ${LAMBDA_RUNTIME_DIR}/lib/

# Set library path
ENV LD_LIBRARY_PATH=${LAMBDA_RUNTIME_DIR}/lib

CMD ["main"]
```

## Production Considerations

### Performance Optimization

1. **Model Caching**: Preload models at startup
2. **Connection Pooling**: Reuse NLP instances
3. **Memory Management**: Monitor memory usage
4. **Horizontal Scaling**: Use multiple instances

### Resource Requirements

| Model Size | RAM Usage | CPU Usage | Throughput |
|------------|-----------|-----------|------------|
| Small      | 100MB     | Low       | High       |
| Medium     | 300MB     | Medium    | Medium     |
| Large      | 600MB     | High      | Lower      |
| Transformer| 800MB+    | Very High | Lowest     |

### Configuration Management

```go
type Config struct {
    SpacyModel     string        `env:"SPACY_MODEL" default:"en_core_web_sm"`
    MaxWorkers     int          `env:"MAX_WORKERS" default:"10"`
    RequestTimeout time.Duration `env:"REQUEST_TIMEOUT" default:"30s"`
    LogLevel       string        `env:"LOG_LEVEL" default:"info"`
    MetricsPort    int          `env:"METRICS_PORT" default:"9090"`
}
```

### Health Checks

Implement comprehensive health checks:

```go
func healthCheck(nlp *spacy.NLP) error {
    // Test model loading
    if nlp == nil {
        return fmt.Errorf("NLP instance not initialized")
    }

    // Test basic functionality
    tokens := nlp.Tokenize("health check")
    if len(tokens) == 0 {
        return fmt.Errorf("tokenization failed")
    }

    return nil
}
```

## Monitoring and Logging

### Metrics Collection

Use Prometheus metrics:

```go
var (
    requestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "spacy_requests_total",
            Help: "Total number of NLP requests",
        },
        []string{"method", "status"},
    )

    requestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "spacy_request_duration_seconds",
            Help: "Request duration histogram",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method"},
    )

    modelMemoryUsage = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "spacy_model_memory_bytes",
            Help: "Memory usage by model",
        },
        []string{"model"},
    )
)
```

### Structured Logging

Use structured logging with context:

```go
func processText(ctx context.Context, text string) {
    logger := log.WithFields(log.Fields{
        "request_id": ctx.Value("request_id"),
        "text_length": len(text),
    })

    start := time.Now()
    defer func() {
        logger.WithField("duration", time.Since(start)).Info("Text processed")
    }()

    // Process text...
}
```

### Alerting Rules

Create alerting rules for common issues:

```yaml
# prometheus-rules.yaml
groups:
- name: go-spacy
  rules:
  - alert: HighErrorRate
    expr: rate(spacy_requests_total{status="error"}[5m]) > 0.1
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: High error rate in Go-Spacy service

  - alert: HighMemoryUsage
    expr: spacy_model_memory_bytes > 1000000000
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: Go-Spacy using excessive memory
```

## Security Best Practices

### Container Security

1. **Non-root User**: Run containers as non-root
2. **Minimal Images**: Use distroless or minimal base images
3. **Security Scanning**: Regular vulnerability scans
4. **Secret Management**: Use proper secret management

### Network Security

1. **TLS Encryption**: All communication over TLS
2. **Network Policies**: Restrict network access
3. **API Security**: Authentication and authorization
4. **Rate Limiting**: Prevent abuse

### Code Security

1. **Input Validation**: Validate all inputs
2. **Error Handling**: Don't leak sensitive information
3. **Dependency Scanning**: Regular security updates
4. **Static Analysis**: Use security linters

### Example Security Middleware

```go
func securityMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Security headers
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")

        // Content length limit
        r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB limit

        next.ServeHTTP(w, r)
    })
}
```

## Troubleshooting Deployment Issues

### Common Production Issues

1. **Memory Leaks**: Monitor memory usage over time
2. **Performance Degradation**: Profile CPU and memory
3. **Model Loading Failures**: Verify model availability
4. **Library Path Issues**: Check LD_LIBRARY_PATH

### Debug Configuration

```yaml
# debug-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-spacy-debug
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: app
        image: your-repo/go-spacy-app:debug
        env:
        - name: LOG_LEVEL
          value: "debug"
        - name: PPROF_ENABLED
          value: "true"
        ports:
        - containerPort: 6060  # pprof port
          name: pprof
```

## Conclusion

This CI/CD and deployment guide provides a comprehensive approach to maintaining high code quality, ensuring reliable deployments, and operating Go-Spacy applications in production environments.

Key takeaways:
- Implement comprehensive testing at all levels
- Use automated CI/CD pipelines for reliability
- Choose deployment strategy based on requirements
- Monitor and log extensively for observability
- Follow security best practices throughout

For additional help with deployment issues, consult the troubleshooting guide or create an issue on GitHub.