# Integration Test Framework Technical Specification

## Overview

This document provides detailed technical specifications for the new integration test framework, including API definitions, data models, interfaces, and implementation details.

## Architecture Overview

### Core Components

```
kiali-test-framework/
├── cli/                    # Command-line interface
├── core/                   # Core framework logic
├── providers/              # Cluster providers (KinD, Minikube)
├── components/             # Component managers (Istio, Kiali, etc.)
├── tests/                  # Test execution engines
├── config/                 # Configuration management
├── storage/                # State and artifact storage
└── api/                    # REST API (optional)
```

### Design Principles

- **Modular Architecture**: Clear separation of concerns with pluggable components
- **Configuration-Driven**: Behavior controlled by declarative configuration
- **Error Resilience**: Comprehensive error handling and recovery
- **Resource Efficiency**: Optimized resource usage and cleanup
- **Developer Experience**: Intuitive CLI and clear error messages

## Command-Line Interface Specification

### CLI Structure

```bash
kiali-test [global-options] <command> [command-options] [arguments]
```

### Global Options

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `--config` | string | Path to configuration file | `./kiali-test.yaml` |
| `--debug` | boolean | Enable debug logging | `false` |
| `--dry-run` | boolean | Show what would be executed | `false` |
| `--verbose` | boolean | Enable verbose output | `false` |
| `--yes` | boolean | Skip confirmation prompts | `false` |

### Core Commands

#### 1. Initialization Commands

**Command**: `kiali-test init [options]`

Initializes a new test environment.

**Options:**
- `--cluster-type <kind|minikube>`: Cluster provider type
- `--multicluster <topology>`: Multi-cluster topology (primary-remote, multi-primary, etc.)
- `--istio-version <version>`: Istio version to install
- `--kiali-version <version>`: Kiali version to install
- `--auth-strategy <anonymous|token|openid>`: Authentication strategy

**Example:**
```bash
kiali-test init --cluster-type kind --multicluster primary-remote --istio-version 1.20.0
```

#### 2. Environment Management Commands

**Command**: `kiali-test up [options]`

Starts the test environment and installs all components.

**Options:**
- `--components <list>`: Components to install (comma-separated)
- `--demo-apps <list>`: Demo applications to deploy
- `--timeout <duration>`: Operation timeout

**Command**: `kiali-test down [options]`

Stops the test environment and cleans up resources.

**Options:**
- `--force`: Force cleanup without confirmation
- `--keep-data`: Preserve persistent data

**Command**: `kiali-test status [options]`

Shows the current status of the test environment.

**Options:**
- `--format <table|json|yaml>`: Output format
- `--watch`: Continuously monitor status

#### 3. Test Execution Commands

**Command**: `kiali-test run [options]`

Executes tests in the current environment.

**Options:**
- `--test-type <cypress|backend|performance>`: Type of tests to run
- `--test-suite <suite>`: Test suite to execute
- `--tags <tags>`: Test tags to include/exclude
- `--parallel <count>`: Number of parallel executions
- `--report-format <junit|json|html>`: Test report format

**Examples:**
```bash
# Run all Cypress core tests
kiali-test run --test-type cypress --tags @core

# Run backend integration tests
kiali-test run --test-type backend

# Run performance tests in parallel
kiali-test run --test-type performance --parallel 4
```

#### 4. Configuration Commands

**Command**: `kiali-test config validate [options]`

Validates the configuration file.

**Options:**
- `--strict`: Fail on warnings
- `--output <format>`: Validation report format

**Command**: `kiali-test config show [options]`

Displays the current configuration.

**Options:**
- `--format <json|yaml|table>`: Output format
- `--mask-secrets`: Mask sensitive values

#### 5. Utility Commands

**Command**: `kiali-test logs [options] [component]`

Shows logs from components.

**Options:**
- `--component <name>`: Specific component logs
- `--follow`: Follow log output
- `--since <duration>`: Show logs since duration

**Command**: `kiali-test debug [options]`

Collects diagnostic information.

**Options:**
- `--output <directory>`: Output directory for diagnostics
- `--include-logs`: Include component logs
- `--include-configs`: Include configuration files

**Command**: `kiali-test clean [options]`

Cleans up test artifacts and temporary files.

**Options:**
- `--all`: Clean all artifacts
- `--older-than <duration>`: Clean artifacts older than duration

## Configuration Specification

### Configuration File Format

The framework uses YAML for configuration files with the following structure:

```yaml
# Global configuration
version: "1.0"
name: "my-test-environment"

# Cluster configuration
cluster:
  provider: kind  # kind, minikube
  name: "kiali-test"
  version: "1.27.0"  # Kubernetes version
  nodes: 1
  memory: "4Gi"
  cpu: "2"

# Multi-cluster configuration (optional)
multicluster:
  enabled: true
  topology: "primary-remote"  # primary-remote, multi-primary, external-controlplane
  clusters:
    - name: "east"
      role: "primary"
    - name: "west"
      role: "remote"

# Component configuration
components:
  istio:
    version: "1.20.0"
    profile: "default"  # default, ambient, minimal
    namespace: "istio-system"

  kiali:
    version: "latest"
    namespace: "istio-system"
    auth:
      strategy: "token"  # anonymous, token, openid
    web:
      port: 20001

  prometheus:
    enabled: true
    namespace: "monitoring"

  grafana:
    enabled: false

  tempo:
    enabled: false
    backend: "jaeger"  # jaeger, tempo

# Demo applications
demo_apps:
  - name: "bookinfo"
    enabled: true
    namespace: "bookinfo"
    traffic_generator: true

  - name: "custom-app"
    enabled: false
    manifests:
      - "path/to/manifests/"

# Test configuration
tests:
  cypress:
    base_url: "http://localhost:20001"
    video: false
    screenshots: true
    timeout: 60000

  backend:
    timeout: 300
    parallel: 1

  performance:
    duration: "5m"
    users: 100
    ramp_up: "30s"

# Resource limits
resources:
  limits:
    memory: "8Gi"
    cpu: "4"
  requests:
    memory: "4Gi"
    cpu: "2"

# Environment variables
env:
  CYPRESS_BASE_URL: "{{ .KialiURL }}"
  CYPRESS_CLUSTER1_CONTEXT: "{{ .Cluster1Context }}"
  CYPRESS_USERNAME: "kiali"
  CYPRESS_PASSWD: "kiali"
```

### Configuration Schema

#### Cluster Configuration Schema

```typescript
interface ClusterConfig {
  provider: 'kind' | 'minikube';
  name: string;
  version: string;  // Kubernetes version
  nodes: number;
  memory?: string;
  cpu?: string;
  network?: string;
  registry?: string;
}
```

#### Component Configuration Schema

```typescript
interface ComponentConfig {
  enabled: boolean;
  version?: string;
  namespace: string;
  values?: Record<string, any>;  // Helm values
  manifests?: string[];  // Additional manifests
}
```

#### Test Configuration Schema

```typescript
interface TestConfig {
  type: 'cypress' | 'backend' | 'performance';
  suite?: string;
  tags?: string[];
  timeout?: number;
  parallel?: number;
  reports?: {
    format: 'junit' | 'json' | 'html';
    output: string;
  };
}
```

## Data Models

### Environment State Model

```typescript
interface EnvironmentState {
  id: string;
  name: string;
  status: 'creating' | 'running' | 'stopped' | 'error';
  created: Date;
  updated: Date;

  cluster: {
    provider: string;
    name: string;
    status: 'creating' | 'running' | 'error';
    nodes: ClusterNode[];
  };

  components: ComponentState[];
  tests: TestExecution[];
}
```

### Component State Model

```typescript
interface ComponentState {
  name: string;
  type: string;
  status: 'installing' | 'running' | 'error' | 'not-installed';
  version: string;
  namespace: string;
  endpoint?: string;
  health: 'healthy' | 'unhealthy' | 'unknown';
  lastChecked: Date;
}
```

### Test Execution Model

```typescript
interface TestExecution {
  id: string;
  type: 'cypress' | 'backend' | 'performance';
  suite: string;
  status: 'running' | 'passed' | 'failed' | 'error';
  startTime: Date;
  endTime?: Date;
  duration?: number;

  results: {
    total: number;
    passed: number;
    failed: number;
    skipped: number;
  };

  artifacts: {
    logs: string[];
    screenshots: string[];
    videos: string[];
    reports: string[];
  };
}
```

## REST API Specification (Optional)

### API Endpoints

#### Environment Management

```
GET    /api/v1/environments           # List environments
POST   /api/v1/environments           # Create environment
GET    /api/v1/environments/{id}      # Get environment
PUT    /api/v1/environments/{id}      # Update environment
DELETE /api/v1/environments/{id}      # Delete environment

POST   /api/v1/environments/{id}/start    # Start environment
POST   /api/v1/environments/{id}/stop     # Stop environment
GET    /api/v1/environments/{id}/status   # Get environment status
```

#### Test Execution

```
GET    /api/v1/environments/{id}/tests           # List test executions
POST   /api/v1/environments/{id}/tests           # Execute tests
GET    /api/v1/environments/{id}/tests/{testId}  # Get test execution
GET    /api/v1/environments/{id}/tests/{testId}/logs     # Get test logs
GET    /api/v1/environments/{id}/tests/{testId}/artifacts # Get test artifacts
```

#### Configuration

```
GET    /api/v1/config                    # Get current configuration
PUT    /api/v1/config                    # Update configuration
POST   /api/v1/config/validate           # Validate configuration
GET    /api/v1/config/schema             # Get configuration schema
```

### API Response Format

#### Success Response

```json
{
  "success": true,
  "data": { ... },
  "meta": {
    "timestamp": "2024-01-01T00:00:00Z",
    "requestId": "req-12345"
  }
}
```

#### Error Response

```json
{
  "success": false,
  "error": {
    "code": "CLUSTER_CREATE_FAILED",
    "message": "Failed to create cluster",
    "details": "KinD cluster creation failed",
    "suggestions": [
      "Check Docker daemon status",
      "Verify system resources"
    ]
  },
  "meta": {
    "timestamp": "2024-01-01T00:00:00Z",
    "requestId": "req-12345"
  }
}
```

## Component Interfaces

### Cluster Provider Interface

```typescript
interface ClusterProvider {
  name: string;

  validateConfig(config: ClusterConfig): Promise<ValidationResult>;
  createCluster(config: ClusterConfig): Promise<Cluster>;
  deleteCluster(cluster: Cluster): Promise<void>;
  getClusterStatus(cluster: Cluster): Promise<ClusterStatus>;

  createSnapshot(cluster: Cluster, name: string): Promise<Snapshot>;
  restoreSnapshot(cluster: Cluster, snapshot: Snapshot): Promise<void>;
}
```

### Component Manager Interface

```typescript
interface ComponentManager {
  name: string;
  type: string;

  validateConfig(config: ComponentConfig): Promise<ValidationResult>;
  install(config: ComponentConfig, cluster: Cluster): Promise<Component>;
  uninstall(component: Component): Promise<void>;
  getStatus(component: Component): Promise<ComponentStatus>;

  upgrade(component: Component, version: string): Promise<Component>;
  getLogs(component: Component, options: LogOptions): Promise<string[]>;
}
```

### Test Executor Interface

```typescript
interface TestExecutor {
  type: string;

  validateConfig(config: TestConfig): Promise<ValidationResult>;
  execute(config: TestConfig, environment: Environment): Promise<TestResult>;
  getStatus(execution: TestExecution): Promise<TestStatus>;

  cancel(execution: TestExecution): Promise<void>;
  cleanup(execution: TestExecution): Promise<void>;
}
```

## Error Handling

### Error Categories

- **Configuration Errors**: Invalid configuration files or values
- **Resource Errors**: Insufficient resources or resource conflicts
- **Network Errors**: Connectivity issues or DNS resolution failures
- **Component Errors**: Component installation or configuration failures
- **Test Errors**: Test execution failures or timeouts

### Error Response Format

```typescript
interface FrameworkError extends Error {
  code: string;
  category: ErrorCategory;
  details?: any;
  suggestions?: string[];
  retryable: boolean;
  context?: Record<string, any>;
}
```

### Error Handling Strategy

1. **Validation**: Pre-flight checks before operations
2. **Graceful Degradation**: Continue with partial functionality when possible
3. **Automatic Retry**: Retry transient failures with exponential backoff
4. **Clear Messaging**: Provide actionable error messages
5. **Cleanup**: Ensure resources are cleaned up on failures

## Security Considerations

### Authentication and Authorization

- **API Authentication**: Token-based authentication for REST API
- **CLI Security**: Secure credential handling in CLI
- **Component Security**: Secure component configurations
- **Audit Logging**: Comprehensive audit trail for operations

### Data Protection

- **Configuration Security**: Encrypt sensitive configuration values
- **Log Security**: Sanitize logs to prevent credential leakage
- **Artifact Security**: Secure storage of test artifacts
- **Network Security**: Secure communication between components

## Performance Specifications

### Performance Targets

- **Setup Time**: Single cluster < 5 minutes, Multi-cluster < 10 minutes
- **Test Execution**: Cypress tests < 10 minutes, Backend tests < 5 minutes
- **Resource Usage**: Memory < 8GB for multi-cluster, CPU < 4 cores
- **Concurrent Tests**: Support up to 4 parallel test executions

### Performance Monitoring

- **Metrics Collection**: CPU, memory, disk, and network usage
- **Performance Profiling**: Identify performance bottlenecks
- **Optimization**: Continuous performance optimization
- **Benchmarking**: Regular performance benchmarking

## Implementation Guidelines

### Code Organization

```
src/
├── cli/                    # CLI commands and options
├── core/                   # Core framework logic
│   ├── config/            # Configuration management
│   ├── state/             # Environment state management
│   └── errors/            # Error handling
├── providers/             # Cluster providers
│   ├── kind/
│   └── minikube/
├── components/            # Component managers
│   ├── istio/
│   ├── kiali/
│   └── prometheus/
├── tests/                 # Test executors
│   ├── cypress/
│   └── backend/
└── utils/                 # Utility functions
```

### Development Standards

- **Language**: Go for core framework, with optional Python/Node.js bindings
- **Testing**: Unit tests for all components, integration tests for workflows
- **Documentation**: Comprehensive inline documentation and API docs
- **Versioning**: Semantic versioning with backward compatibility
- **CI/CD**: Automated testing and release processes

This technical specification provides the foundation for implementing the new integration test framework with clear interfaces, data models, and implementation guidelines.
