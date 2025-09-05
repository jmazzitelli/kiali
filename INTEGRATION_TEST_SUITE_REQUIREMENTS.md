# Kiali Integration Test Suite Requirements

## Overview

This document outlines the comprehensive requirements for implementing a unified integration test suite for Kiali that can run both in CI environments (GitHub Actions) and locally. The solution must support multiple test scenarios with varying infrastructure requirements. The solution must not use any existing hack scripts - all hack scripts used in this solution must be built from the ground up starting from nothing.

## 1. Main Entry Point

### Primary Script: `hack/run-integration-test-suite.sh`

The main entry point script must accept the following command line arguments:

#### Required Arguments
- `--test-suite <suite_name>`: Specifies which test suite to run
- `--cluster-type <type>`: Specifies the Kubernetes cluster type (`kind` or `minikube`)

#### Optional Arguments
- `--istio-version <version>`: Istio version to install (format: "#.#.#" for releases, "#.#-dev" for dev builds)
- `--setup-only <true|false>`: Only setup the environment without running tests (default: false)
- `--tests-only <true|false>`: Only run tests, skip environment setup (default: false)
- `--cleanup <true|false>`: Clean up environment after tests (default: true)
- `--debug <true|false>`: Enable debug output and artifact collection (default: false)
- `--timeout <seconds>`: Test execution timeout (default: 3600)
- `--parallel-setup <true|false>`: Enable parallel cluster and component setup (default: true)

## 2. Test Suite Definitions

### 2.1 Backend Test Suites

#### `backend`
- **Clusters**: 1 (single cluster)
- **Istio**: Required
- **Kiali**: Required (in-cluster)
- **Additional Components**: None
- **Test Type**: Go integration tests for backend API
- **Cypress**: No

#### `backend-external-controlplane`
- **Clusters**: 2 (controlplane + data plane)
- **Istio**: Required (external controlplane setup)
- **Kiali**: Required (on controlplane cluster)
- **Additional Components**: None
- **Test Type**: Go integration tests for multicluster backend API
- **Cypress**: No
- **Special**: External controlplane configuration

### 2.2 Frontend Test Suites

#### `frontend`
- **Clusters**: 1 (single cluster)
- **Istio**: Required
- **Kiali**: Required (in-cluster)
- **Additional Components**: None
- **Test Type**: Cypress E2E tests
- **Cypress Config**: `cypress.config.ts`
- **Test Pattern**: `**/*.feature`

#### `frontend-ambient`
- **Clusters**: 1 (single cluster)
- **Istio**: Required (ambient mode)
- **Kiali**: Required (in-cluster)
- **Additional Components**: None
- **Test Type**: Cypress E2E tests
- **Cypress Config**: `cypress.config.ts`
- **Special**: Istio ambient mesh configuration

#### `frontend-multicluster-primary-remote`
- **Clusters**: 2 (primary + remote)
- **Istio**: Required (multicluster setup)
- **Kiali**: Required (on primary cluster)
- **Additional Components**: None
- **Test Type**: Cypress E2E tests (multicluster features)
- **Cypress Config**: `cypress.config.ts`

#### `frontend-multicluster-multi-primary`
- **Clusters**: 2 (both primary clusters)
- **Istio**: Required (multicluster setup)
- **Kiali**: Required (on both clusters)
- **Additional Components**: None
- **Test Type**: Cypress E2E tests (multi-primary features)
- **Cypress Config**: `cypress.config.ts`

#### `frontend-external-kiali`
- **Clusters**: 2 (management + workload clusters)
- **Istio**: Required (on workload cluster)
- **Kiali**: Required (on management cluster, external to workload)
- **Additional Components**: None
- **Test Type**: Cypress E2E tests (external Kiali setup)
- **Cypress Config**: `cypress.config.ts`

#### `frontend-tempo`
- **Clusters**: 1 (single cluster)
- **Istio**: Required
- **Kiali**: Required (in-cluster)
- **Additional Components**: Tempo (tracing)
- **Test Type**: Cypress E2E tests (tracing features)
- **Cypress Config**: `cypress.config.ts`

#### `local` (frontend-local)
- **Clusters**: 1 (single cluster)
- **Istio**: Not required
- **Kiali**: Required (local mode - no cluster access)
- **Additional Components**: None
- **Test Type**: Cypress E2E tests (local mode)
- **Cypress Config**: `cypress.config.ts`

## 3. Infrastructure Requirements

### 3.1 Kubernetes Cluster Support

#### KinD (Kubernetes in Docker)
- **Usage**: CI environments (GitHub Actions)
- **Advantages**: Faster startup, consistent environment
- **Configuration**: Must support multi-cluster setups
- **Node Configuration**: Customizable for different test requirements

#### Minikube
- **Usage**: Local development environments
- **Profile**: Must use profile "ci" for consistency with existing tooling
- **Configuration**: Must support multi-cluster setups via multiple profiles
- **Addons**: Support for required Istio components

### 3.2 Component Installation Requirements

#### Istio Service Mesh
- **Versions**: Support for specific versions via `--istio-version` parameter
- **Modes**: 
  - Standard sidecar mode
  - Ambient mesh mode
  - Multicluster configurations (primary-remote, multi-primary)
  - External controlplane setup
- **Components**: Core Istio components + required addons

#### Kiali Installation
- **Deployment Types**:
  - In-cluster (standard deployment)
  - External (management cluster accessing workload clusters)
  - Local mode (no cluster access)
- **Configuration**: Must be configured per test suite requirements
- **Build**: Use locally built images/binaries from CI artifacts

#### Optional Components

##### Keycloak (OpenID Provider)
- **When Required**: Test suites that need authentication testing
- **Configuration**: Pre-configured with test users and clients
- **Memory Limits**: Configurable via parameters

##### Tempo (Tracing Backend)
- **When Required**: `frontend-tempo` test suite
- **Configuration**: Integrated with Istio for distributed tracing
- **Storage**: Temporary storage suitable for testing

##### Grafana (Metrics Dashboard)
- **When Required**: Test suites that validate metrics integration
- **Configuration**: Pre-configured with Kiali dashboards
- **Data Sources**: Connected to Prometheus/other metrics sources

## 4. Test Execution Requirements

### 4.1 Cypress Test Execution

#### Environment Variables
- `CI=1`: Indicate CI environment
- `TERM=xterm`: Prevent terminal warnings
- `CYPRESS_BASE_URL`: Kiali frontend URL
- `CYPRESS_AUTH_STRATEGY`: Authentication strategy (from `getAuthStrategy()`)
- `CYPRESS_AUTH_PROVIDER`: Authentication provider (default: `my_htpasswd_provider`)

#### Test Categories
- **Feature Tests**: Gherkin-based BDD tests (`**/*.feature`)
- **Performance Tests**: Load testing (`**/*.spec.ts` in perf directory)

#### Test Isolation
- `testIsolation: false`: Tests share state within spec files
- Proper cleanup between test suites

### 4.2 Backend Test Execution

#### Go Integration Tests
- **Test Command**: `make test-integration-controller`
- **Environment**: Requires compiled Kiali binary in `~/go/bin/kiali`
- **Configuration**: Uses kubeconfig for cluster access

## 5. Environment Setup Requirements

### 5.1 Parallel Setup (Nice-to-Have)
- **Cluster Creation**: Multiple clusters created simultaneously
- **Component Installation**: Parallel installation where dependencies allow
- **Dependency Management**: Istio must be installed before application workloads

### 5.2 Setup Phases
1. **Pre-flight Checks**: Validate prerequisites and parameters
2. **Cluster Creation**: Create required Kubernetes clusters
3. **Base Component Installation**: Install Istio, networking components
4. **Application Installation**: Install Kiali and test applications
5. **Additional Components**: Install Keycloak, Tempo, Grafana as needed
6. **Validation**: Verify all components are ready
7. **Test Execution**: Run the specified test suite
8. **Cleanup**: Optional cleanup of created resources

## 6. Debug and Artifact Collection

### 6.1 Debug Information Collection
- **Trigger**: On test failure or when `--debug=true`
- **Content**:
  - Kubernetes resource descriptions (`kubectl describe`)
  - Pod logs from all namespaces
  - Istio configuration dumps
  - Kiali configuration and logs
  - Network connectivity tests
  - Resource utilization metrics

### 6.2 Artifact Upload (CI Only)
- **Debug Info**: Collected debug information as compressed archive
- **Cypress Screenshots**: Test failure screenshots
- **Cypress Videos**: Test execution videos (if enabled)
- **Logs**: Structured logs from stern or similar tools
- **Test Reports**: JUnit/JSON test result reports

## 7. Configuration Management

### 7.1 Environment Parity
- **Requirement**: Local and CI environments must be identical
- **Implementation**: Same scripts, same configurations, same component versions
- **Validation**: Environment fingerprinting and validation

### 7.2 Configuration Files
- **Cluster Configs**: Templates for different cluster topologies
- **Component Configs**: Helm values and Kubernetes manifests
- **Test Configs**: Cypress and Go test configurations
- **Environment Configs**: Environment-specific overrides

## 8. Error Handling and Resilience

### 8.1 Retry Logic
- **Component Installation**: Retry failed installations with backoff
- **Network Operations**: Retry network-dependent operations
- **Test Execution**: Configurable test retry on transient failures

### 8.2 Cleanup on Failure
- **Partial Cleanup**: Clean up successfully created resources on failure
- **Full Cleanup**: Option to preserve environment for debugging
- **Resource Tracking**: Track created resources for proper cleanup

## 9. Performance Requirements

### 9.1 Setup Time Optimization
- **Target**: Complete environment setup in < 10 minutes for single cluster
- **Target**: Complete environment setup in < 15 minutes for multi-cluster
- **Method**: Parallel operations, caching, optimized images

### 9.2 Resource Efficiency
- **Memory**: Optimize component memory usage for CI environments
- **CPU**: Efficient resource allocation and limits
- **Storage**: Minimal storage footprint, cleanup temporary files

## 10. Integration Points

### 10.1 GitHub Actions Integration
- **Workflow Simplification**: Single command execution in workflows
- **Artifact Integration**: Seamless artifact upload/download
- **Status Reporting**: Clear success/failure reporting
- **Matrix Builds**: Support for testing multiple configurations

### 10.2 Local Development Integration
- **Developer Experience**: Simple setup and execution
- **IDE Integration**: Compatible with common development environments
- **Debugging**: Easy access to logs and debug information
- **Iteration**: Fast setup for repeated testing

## 11. Validation and Testing

### 11.1 Environment Validation
- **Component Health**: Verify all components are healthy before testing
- **Connectivity**: Validate network connectivity between components
- **Authentication**: Verify authentication mechanisms work
- **Data Flow**: Validate metrics, traces, and logs flow correctly

### 11.2 Test Suite Validation
- **Test Discovery**: Automatic discovery and validation of test files
- **Configuration Validation**: Validate test configurations before execution
- **Dependency Validation**: Ensure test dependencies are met

## 12. Documentation and Maintenance

### 12.1 Usage Documentation
- **Quick Start**: Simple getting started guide
- **Reference**: Complete parameter and option reference
- **Troubleshooting**: Common issues and solutions
- **Examples**: Example usage for different scenarios

### 12.2 Maintenance Requirements
- **Version Updates**: Easy updates for component versions
- **Configuration Updates**: Simple configuration management
- **Test Updates**: Easy addition/modification of test suites
- **Monitoring**: Health monitoring and alerting for CI environments

## Implementation Priority

### Phase 1: Core Infrastructure
1. Main entry script with argument parsing
2. Basic cluster creation (KinD and Minikube)
3. Istio installation
4. Kiali installation
5. Single cluster test suites (`backend`, `frontend`)

### Phase 2: Multi-cluster Support
1. Multi-cluster setup scripts
2. Multicluster test suites
3. External controlplane setup
4. Advanced Istio configurations

### Phase 3: Additional Components
1. Keycloak integration
2. Tempo integration
3. Grafana integration
4. Performance optimizations

### Phase 4: Advanced Features
1. Parallel setup implementation
2. Advanced debug collection
3. Enhanced error handling
4. Performance monitoring

This requirements document provides a comprehensive foundation for implementing the Kiali integration test suite that meets all specified needs while maintaining consistency between CI and local environments.
