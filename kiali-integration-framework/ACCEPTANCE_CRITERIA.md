# Integration Test Framework Acceptance Criteria

## Overview

This document defines specific, measurable acceptance criteria for each functional requirement. These criteria serve as the basis for testing and validation during development and provide clear success metrics for the framework implementation.

## Cluster Management Criteria

### FR-CLUSTER-001: Single Cluster Support

**AC-CLUSTER-001-01**: Framework creates KinD cluster with default configuration in < 3 minutes
- **Given**: Clean environment with Docker/Podman installed
- **When**: User runs `kiali-test init --cluster-type kind`
- **Then**: Single-node KinD cluster is created and ready
- **And**: kubectl can connect to the cluster
- **And**: Cluster creation completes in under 3 minutes

**AC-CLUSTER-001-02**: Framework supports custom cluster configurations
- **Given**: Configuration file with custom cluster settings (node count, k8s version)
- **When**: User runs `kiali-test init --config custom-config.yaml`
- **Then**: Cluster is created with specified configuration
- **And**: Configuration validation succeeds
- **And**: Cluster meets all specified requirements

**AC-CLUSTER-001-03**: Framework supports Minikube as alternative cluster provider
- **Given**: Minikube installed on system
- **When**: User runs `kiali-test init --cluster-type minikube`
- **Then**: Minikube cluster is created successfully
- **And**: Framework detects and uses Minikube automatically

### FR-CLUSTER-002: Multi-Cluster Support

**AC-CLUSTER-002-01**: Framework creates primary-remote multi-cluster setup
- **Given**: Configuration for primary-remote topology
- **When**: User runs `kiali-test init --multicluster primary-remote`
- **Then**: Two clusters are created (east, west)
- **And**: Clusters are connected via mesh federation
- **And**: Service discovery works across clusters
- **And**: Setup completes in under 8 minutes

**AC-CLUSTER-002-02**: Framework creates multi-primary cluster setup
- **Given**: Configuration for multi-primary topology with Keycloak
- **When**: User runs `kiali-test init --multicluster multi-primary`
- **Then**: Two clusters are created with OIDC authentication
- **And**: Keycloak is deployed and configured
- **And**: Cross-cluster authentication works
- **And**: Setup completes in under 10 minutes

**AC-CLUSTER-002-03**: Framework handles external control plane topology
- **Given**: Configuration for external control plane
- **When**: User runs `kiali-test init --multicluster external-controlplane`
- **Then**: Control plane and data plane clusters are created
- **And**: Istio control plane is isolated from data plane
- **And**: Workloads run only on data plane cluster

### FR-CLUSTER-003: Cluster Lifecycle Management

**AC-CLUSTER-003-01**: Framework provides cluster lifecycle commands
- **Given**: Running test environment
- **When**: User runs `kiali-test status`
- **Then**: Current cluster and component status is displayed
- **When**: User runs `kiali-test stop`
- **Then**: All clusters are stopped but preserved
- **When**: User runs `kiali-test start`
- **Then**: Clusters are restarted successfully
- **When**: User runs `kiali-test destroy`
- **Then**: All clusters and resources are cleaned up

**AC-CLUSTER-003-02**: Framework supports cluster state management
- **Given**: Running multi-cluster environment
- **When**: User runs `kiali-test snapshot create my-snapshot`
- **Then**: Cluster state is saved to snapshot
- **When**: User runs `kiali-test snapshot restore my-snapshot`
- **Then**: Cluster state is restored successfully
- **And**: All components are in the same state as when snapshot was taken

## Component Installation Criteria

### FR-COMPONENT-001: Istio Installation

**AC-COMPONENT-001-01**: Framework installs Istio with specified version
- **Given**: Configuration specifying Istio version 1.20.0
- **When**: User runs `kiali-test up`
- **Then**: Istio 1.20.0 is installed
- **And**: Istio control plane is healthy
- **And**: `istioctl version` matches specified version

**AC-COMPONENT-001-02**: Framework supports ambient mesh profile
- **Given**: Configuration with ambient: true
- **When**: User runs `kiali-test up`
- **Then**: Istio is installed with ambient profile
- **And**: Ambient components (ztunnel, waypoint) are running
- **And**: Ambient mesh is functional

**AC-COMPONENT-001-03**: Framework configures Istio appropriately for each scenario
- **Given**: Different test scenarios (frontend, backend, multicluster)
- **When**: Framework installs Istio for each scenario
- **Then**: Istio is configured with scenario-appropriate settings
- **And**: All required Istio components are operational

### FR-COMPONENT-002: Kiali Installation

**AC-COMPONENT-002-01**: Framework installs Kiali with anonymous authentication
- **Given**: Configuration with auth_strategy: anonymous
- **When**: User runs `kiali-test up`
- **Then**: Kiali is installed with anonymous access enabled
- **And**: Kiali UI is accessible without authentication
- **And**: Kiali API endpoints are accessible

**AC-COMPONENT-002-02**: Framework installs Kiali with token authentication
- **Given**: Configuration with auth_strategy: token
- **When**: User runs `kiali-test up`
- **Then**: Kiali is installed with token authentication
- **And**: Service account token can be used for authentication
- **And**: Kiali validates tokens correctly

**AC-COMPONENT-002-03**: Framework installs Kiali with OIDC authentication
- **Given**: Multi-primary configuration with Keycloak
- **When**: User runs `kiali-test up`
- **Then**: Kiali is configured for OIDC authentication
- **And**: Keycloak integration is functional
- **And**: User authentication works through Keycloak

### FR-COMPONENT-003: Demo Applications

**AC-COMPONENT-003-01**: Framework installs BookInfo demo application
- **Given**: Configuration with demo_apps: bookinfo
- **When**: User runs `kiali-test up`
- **Then**: BookInfo application is deployed
- **And**: All microservices are running
- **And**: Traffic flows correctly between services

**AC-COMPONENT-003-02**: Framework supports custom demo applications
- **Given**: Configuration with custom application manifests
- **When**: User runs `kiali-test up`
- **Then**: Custom applications are deployed
- **And**: Applications are configured for the mesh
- **And**: Applications generate appropriate traffic

## Test Execution Criteria

### FR-TEST-001: Cypress Test Support

**AC-TEST-001-01**: Framework executes Cypress tests successfully
- **Given**: Test environment with Kiali and demo apps running
- **When**: User runs `kiali-test run --test-type cypress --tags @core`
- **Then**: Cypress tests execute successfully
- **And**: All core tests pass
- **And**: Test results are collected and reported

**AC-TEST-001-02**: Framework configures Cypress environment variables
- **Given**: Test environment configuration
- **When**: Cypress tests are executed
- **Then**: CYPRESS_BASE_URL is set to Kiali URL
- **And**: CYPRESS_CLUSTER1_CONTEXT is set for multi-cluster tests
- **And**: All required environment variables are configured

**AC-TEST-001-03**: Framework supports different Cypress test categories
- **Given**: Test environment ready
- **When**: User runs tests with different tags
- **Then**: @ambient tests run in ambient environment
- **And**: @multi-cluster tests run in multi-cluster environment
- **And**: @tracing tests run with tracing backend

### FR-TEST-002: Backend Test Support

**AC-TEST-002-01**: Framework executes Go integration tests
- **Given**: Test environment with Kiali running
- **When**: User runs `kiali-test run --test-type backend`
- **Then**: Go tests compile and execute
- **And**: Tests connect to Kiali API successfully
- **And**: Test results are collected in JUnit format

**AC-TEST-002-02**: Framework configures backend test environment
- **Given**: Backend test configuration
- **When**: Tests are executed
- **Then**: URL environment variable points to Kiali
- **And**: Required test dependencies are available
- **And**: Test database/data is properly initialized

### FR-TEST-004: Test Scenarios

**AC-TEST-004-01**: Framework supports all existing test scenarios
- **Given**: All current test scenarios
- **When**: User runs each test scenario
- **Then**: All scenarios execute successfully
- **And**: Test results match current framework results
- **And**: No test scenarios are lost in migration

## Environment Management Criteria

### FR-ENV-001: Local Development Support

**AC-ENV-001-01**: Framework works on developer workstations
- **Given**: Developer machine with required tools
- **When**: User runs `kiali-test init && kiali-test up`
- **Then**: Environment starts successfully
- **And**: No root/admin privileges required beyond Docker
- **And**: Framework handles common network restrictions

**AC-ENV-001-02**: Framework provides simplified local development commands
- **Given**: Developer workflow requirements
- **When**: User runs `kiali-test dev-setup`
- **Then**: Complete development environment is created
- **And**: All components are configured for development
- **And**: Setup completes in under 5 minutes

### FR-ENV-002: CI/CD Integration

**AC-ENV-002-01**: Framework integrates with GitHub Actions
- **Given**: GitHub Actions workflow
- **When**: Workflow executes framework commands
- **Then**: All commands run successfully in CI environment
- **And**: Framework handles CI-specific constraints
- **And**: Artifacts are collected and uploaded

**AC-ENV-002-02**: Framework supports parallel test execution
- **Given**: Multiple test scenarios
- **When**: User runs `kiali-test run --parallel 4`
- **Then**: Tests execute in parallel across available resources
- **And**: Resource conflicts are avoided
- **And**: Results are aggregated correctly

## User Interface Criteria

### FR-UI-001: Command-Line Interface

**AC-UI-001-01**: Framework provides user-friendly CLI
- **Given**: Framework installation
- **When**: User runs `kiali-test --help`
- **Then**: Comprehensive help is displayed
- **And**: All commands are documented
- **And**: Usage examples are provided

**AC-UI-001-02**: Framework supports both interactive and non-interactive modes
- **Given**: Various usage scenarios
- **When**: User runs commands with --yes flag
- **Then**: Commands run without user interaction
- **When**: User runs commands without flags
- **Then**: Commands prompt for confirmation when appropriate

### FR-UI-002: Error Handling and Debugging

**AC-UI-002-01**: Framework provides clear error messages
- **Given**: Various failure scenarios
- **When**: Commands fail
- **Then**: Error messages are clear and actionable
- **And**: Error messages include troubleshooting suggestions
- **And**: Exit codes are appropriate for automation

**AC-UI-002-02**: Framework supports debug mode
- **Given**: Problematic scenario
- **When**: User runs `kiali-test --debug command`
- **Then**: Detailed logging is enabled
- **And**: All operations are logged with timestamps
- **And**: Debug information is collected for analysis

## Performance Criteria

### NFR-PERF-001: Setup Time

**AC-PERF-001-01**: Single cluster setup completes quickly
- **Given**: Clean environment
- **When**: User runs `kiali-test init && kiali-test up`
- **Then**: Total setup time is under 5 minutes
- **And**: Progress is reported during setup
- **And**: Setup can be interrupted and resumed

**AC-PERF-001-02**: Multi-cluster setup meets time requirements
- **Given**: Multi-cluster configuration
- **When**: User runs setup commands
- **Then**: Setup completes in under 10 minutes
- **And**: Each cluster setup is optimized
- **And**: Parallel operations are used where possible

### NFR-PERF-003: Resource Efficiency

**AC-PERF-003-01**: Framework uses resources efficiently
- **Given**: Resource-constrained environment
- **When**: Framework runs single cluster scenario
- **Then**: Memory usage stays under 4GB
- **When**: Framework runs multi-cluster scenario
- **Then**: Memory usage stays under 8GB
- **And**: CPU usage is optimized

## Reliability Criteria

### NFR-REL-001: System Reliability

**AC-REL-001-01**: Framework achieves high success rate
- **Given**: 100 test executions
- **When**: Tests are run in various conditions
- **Then**: At least 95% of executions succeed
- **And**: Failures are due to external factors, not framework issues
- **And**: Framework provides clear failure analysis

**AC-REL-001-02**: Framework handles transient failures
- **Given**: Network interruptions, resource constraints
- **When**: Framework encounters transient failures
- **Then**: Framework retries operations automatically
- **And**: Framework provides exponential backoff
- **And**: Framework fails gracefully after reasonable attempts

### NFR-REL-003: Data Integrity

**AC-REL-003-01**: Framework prevents resource leaks
- **Given**: Multiple test executions
- **When**: Tests complete (success or failure)
- **Then**: All resources are cleaned up
- **And**: No orphaned processes remain
- **And**: No orphaned containers/clusters remain

## Compatibility Criteria

### NFR-COMPAT-001: Backward Compatibility

**AC-COMPAT-001-01**: Framework supports existing test scenarios
- **Given**: All current test workflows
- **When**: Tests are migrated to new framework
- **Then**: All test scenarios work identically
- **And**: Test results are comparable
- **And**: No functionality is lost

**AC-COMPAT-001-02**: Framework provides migration support
- **Given**: Existing CI workflows
- **When**: Migration is performed
- **Then**: Migration path is clear and documented
- **And**: Migration can be done incrementally
- **And**: Rollback is possible if needed

### NFR-COMPAT-002: Platform Support

**AC-COMPAT-002-01**: Framework works on Linux
- **Given**: Linux environment with required tools
- **When**: Framework is used
- **Then**: All functionality works as expected
- **And**: Performance meets requirements

**AC-COMPAT-002-02**: Framework works on macOS
- **Given**: macOS environment with required tools
- **When**: Framework is used
- **Then**: All functionality works as expected
- **And**: Performance meets requirements

**AC-COMPAT-002-03**: Framework works on Windows
- **Given**: Windows environment with required tools
- **When**: Framework is used
- **Then**: All functionality works as expected
- **And**: Performance meets requirements

## Security Criteria

### NFR-SEC-001: Secure Operations

**AC-SEC-001-01**: Framework handles sensitive data appropriately
- **Given**: Configuration with passwords, tokens
- **When**: Framework processes configuration
- **Then**: Sensitive data is not logged in plain text
- **And**: Sensitive data is handled securely
- **And**: Framework uses secure defaults

**AC-SEC-001-02**: Framework validates certificates
- **Given**: HTTPS endpoints and certificates
- **When**: Framework makes connections
- **Then**: SSL/TLS certificates are validated
- **And**: Self-signed certificates are handled appropriately
- **And**: Certificate errors are reported clearly

## Quality Assurance Criteria

### QA-001: Test Coverage
- **Given**: Framework codebase
- **When**: Test coverage is measured
- **Then**: Code coverage is at least 80%
- **And**: Critical paths have 90%+ coverage
- **And**: Coverage reports are generated and accessible

### QA-002: Documentation Completeness
- **Given**: Framework implementation
- **When**: Documentation is reviewed
- **Then**: All public APIs are documented
- **And**: All commands have help text
- **And**: Usage examples are provided for all features

### QA-003: Performance Benchmarks
- **Given**: Framework performance tests
- **When**: Benchmarks are run
- **Then**: Performance meets or exceeds requirements
- **And**: Performance regressions are detected
- **And**: Performance trends are tracked

These acceptance criteria provide measurable, testable requirements that ensure the new framework meets all stakeholder needs and maintains the reliability and functionality of the current system while providing significant improvements.
