# Kiali Integration Test Framework

A robust, modular Go-based framework for Kiali integration testing that replaces the fragile bash-based integration test system.

## Overview

This framework provides a comprehensive solution for:

- **Cluster Management**: Support for KinD, Minikube, and other Kubernetes environments
- **Component Installation**: Automated installation of Istio, Kiali, Prometheus, and other components
- **Test Execution**: Support for Cypress, Go, and custom test suites
- **Configuration Management**: YAML-based declarative configuration
- **CI/CD Integration**: Seamless integration with existing pipelines

## Architecture

The framework follows a modular plugin-based architecture with clear separation of concerns:

- **CLI Layer**: Command-line interface built with Cobra
- **Configuration Layer**: YAML-based configuration management with Viper
- **Provider Layer**: Cluster providers (KinD, Minikube) implementing common interfaces
- **Component Layer**: Component managers for Istio, Kiali, Prometheus, etc.
- **Test Layer**: Test executors for different test types

## Quick Start

### Prerequisites

- Go 1.19+
- Docker or Podman
- kubectl
- For Minikube: Minikube binary (optional, for Minikube cluster provider)

### Installation

```bash
go build -o kiali-test cmd/main.go
```

### Basic Usage

#### KinD Cluster (Lightweight/Fast)
```bash
# Initialize a new test environment with KinD
./kiali-test init --cluster-type kind --multicluster none

# Bring up the environment with components
./kiali-test up --components istio,kiali,prometheus

# Run tests
./kiali-test run --test-type cypress --tags @core

# Check status
./kiali-test status

# Tear down the environment
./kiali-test down
```

#### Minikube Cluster (Feature-rich/Production-like)
```bash
# Initialize with Minikube and custom resources
./kiali-test init --cluster-type minikube --memory 4g --cpus 2 --addons ingress,dashboard

# Or use the new cluster command for fine-grained control
./kiali-test cluster create my-test-cluster --provider minikube --memory 6g --cpus 3 --addons ingress,dashboard,metrics-server

# Bring up components
./kiali-test up --components istio,kiali,prometheus

# Run tests
./kiali-test run --test-type cypress --base-url http://localhost:30080

# Check cluster status
./kiali-test cluster status my-test-cluster --provider minikube

# List all clusters
./kiali-test cluster list --provider minikube

# Delete the cluster
./kiali-test cluster delete my-test-cluster --provider minikube
```

#### Multi-Cluster Topologies
```bash
# Create multi-cluster topology with KinD
./kiali-test topology create --provider kind --primary-name kiali-primary --remotes east-cluster,west-cluster --federation

# Create multi-cluster topology with Minikube
./kiali-test topology create --provider minikube --primary-name kiali-primary --primary-memory 6g --primary-cpus 3 --remotes east-cluster,west-cluster --remote-memory 4g --remote-cpus 2 --federation

# Check topology status
./kiali-test topology status --provider minikube --primary-name kiali-primary --remotes east-cluster,west-cluster

# Delete topology
./kiali-test topology delete --provider minikube --primary-name kiali-primary --remotes east-cluster,west-cluster
```

## Cluster Providers

This framework supports multiple Kubernetes cluster providers, giving you flexibility based on your testing needs:

### KinD (Kubernetes in Docker)
- **Best for**: Fast CI/CD pipelines, lightweight testing, development
- **Pros**: Fast startup (~30s), low resource usage, reliable for automation
- **Cons**: Limited to single-node clusters, fewer built-in features
- **Use case**: Unit tests, integration tests, CI pipelines

### Minikube
- **Best for**: Feature-complete testing, production-like environments, development
- **Pros**: Multi-node support, extensive addons, production-like features, flexible drivers
- **Cons**: Slower startup (~2-3min), higher resource usage
- **Use case**: End-to-end testing, performance testing, feature validation

### Choosing a Provider

| Requirement | KinD | Minikube |
|-------------|------|----------|
| Speed | ✅ Fast | ⚠️ Slower |
| Resources | ✅ Light | ⚠️ Heavy |
| Multi-node | ❌ Single | ✅ Multi |
| Addons | ⚠️ Limited | ✅ Extensive |
| CI/CD | ✅ Ideal | ✅ Good |
| Production-like | ⚠️ Basic | ✅ Advanced |
| Custom drivers | ❌ Docker only | ✅ Multiple |

**Recommendation**:
- Use **KinD** for most testing scenarios and CI/CD pipelines
- Use **Minikube** when you need production-like features, multi-node testing, or specific addons
- Both providers work seamlessly with all framework features

## Configuration

Create a `.kiali-integration-framework.yaml` file in your home directory:

### KinD Configuration
```yaml
version: "1.0"
cluster:
  provider: kind
  name: "kiali-test"
  version: "1.27.0"
  config:
    nodes: 1
```

### Minikube Configuration
```yaml
version: "1.0"
cluster:
  provider: minikube
  name: "kiali-test"
  version: "1.27.0"
  config:
    nodes: 1
    memory: "4g"
    cpus: "2"
    driver: "docker"
    diskSize: "20g"
    addons: ["ingress", "dashboard"]
    ports: ["8080:30080", "8443:30443"]
```

### Multi-Cluster Configuration
```yaml
version: "1.0"
clusters:
  primary:
    provider: minikube
    name: kiali-primary
    version: "1.27.0"
    config:
      nodes: 1
      memory: "6g"
      cpus: "3"
  remotes:
    east-cluster:
      provider: minikube
      name: kiali-east
      version: "1.27.0"
      config:
        nodes: 1
        memory: "4g"
        cpus: "2"

components:
  istio:
    version: "1.20.0"
    profile: "default"
  kiali:
    auth:
      strategy: "token"

tests:
  cypress:
    base_url: "http://localhost:30080"  # Minikube default port
    timeout: 60000
```

## Development Status

This framework has completed **Phase 9 (Minikube Support Implementation)** and is now a production-ready, enterprise-grade testing platform:

- ✅ **Phase 1-7**: All core infrastructure, cluster management, connectivity, network setup, service discovery, and test execution phases completed
- ✅ **Phase 8**: Comprehensive testing and validation (ongoing enterprise validation)
- ✅ **Phase 9**: Minikube Support Implementation - Complete with full CLI integration and documentation

### Key Achievements
- **Dual Cluster Provider Support**: KinD (lightweight/fast) and Minikube (feature-rich/production-like)
- **Multi-Cluster Capabilities**: Full federation support with primary/remote cluster relationships
- **Enterprise Architecture**: Modular design with clean separation of concerns
- **Production-Ready**: 84+ unit tests, comprehensive error handling, CI/CD integration
- **Service Discovery Framework**: DNS, API server aggregation, and service propagation
- **Connectivity Framework**: Kubernetes, Istio, Linkerd, and Manual networking providers

## Project Structure

```
kiali-integration-framework/
├── cmd/
│   └── main.go                 # Application entry point
├── internal/
│   ├── cli/                    # CLI command implementations
│   ├── cluster/                # Cluster provider implementations
│   ├── component/              # Component manager implementations
│   ├── config/                 # Configuration management
│   └── test/                   # Test executor implementations
├── pkg/
│   ├── api/                    # Public API interfaces
│   ├── types/                  # Core type definitions
│   └── utils/                  # Utility functions
├── testdata/                   # Test data and fixtures
├── scripts/                    # Build and deployment scripts
├── go.mod                      # Go module definition
└── README.md                   # This file
```

## Contributing

This project follows the Kiali project's contribution guidelines. Please see the main Kiali repository for detailed information.

## License

This project is licensed under the Apache License 2.0 - see the LICENSE file for details.
