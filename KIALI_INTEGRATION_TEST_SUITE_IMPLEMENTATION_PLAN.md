# Kiali Integration Test Suite Implementation Plan

## Overview

This document provides a comprehensive implementation plan for building a unified integration test suite for Kiali that can run both in CI environments (GitHub Actions) and locally. The solution supports multiple test scenarios with varying infrastructure requirements and addresses critical networking differences between KinD and Minikube cluster types.

## Architecture Overview

The solution is built as a modular, extensible system with clear separation of concerns:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    hack/run-integration-test-suite.sh                       │
│                           (Main Entry Point)                               │
└─────────────────────────┬───────────────────────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────────────────────┐
│                     lib/test-suite-runner.sh                               │
│                        (Core Orchestrator)                                 │
└─┬─────────┬─────────┬─────────┬─────────┬─────────┬─────────┬─────────┬─────┘
  │         │         │         │         │         │         │         │
  ▼         ▼         ▼         ▼         ▼         ▼         ▼         ▼
┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐
│Cluster│ │Comp.  │ │Test   │ │Debug  │ │Network│ │Config │ │Templ. │ │Script │
│Manager│ │Install│ │Exec.  │ │Collect│ │Manager│ │System │ │System │ │Utils  │
└───┬───┘ └───────┘ └───────┘ └───────┘ └───────┘ └───────┘ └───────┘ └───────┘
    │
┌───▼───┐ ┌─────────┐
│ KinD  │ │Minikube │
│Provider│ │Provider │
└───────┘ └─────────┘
```

## Critical Networking Considerations

### KinD vs Minikube Network Architecture Differences

Based on analysis of existing scripts, there are fundamental networking differences that must be addressed:

#### KinD Networking Characteristics
- **Load Balancer**: Uses MetalLB for LoadBalancer services
- **Service Access**: LoadBalancer services get real external IPs
- **Port Access**: Direct access to LoadBalancer IPs on standard ports (80, 443)
- **Multi-cluster**: Network connectivity via shared Docker network
- **Registry**: Can use external registry with network connectivity
- **Ingress**: Direct LoadBalancer access without port mapping

#### Minikube Networking Characteristics  
- **Load Balancer**: Uses `minikube tunnel` or NodePort services
- **Service Access**: NodePort services with high-numbered ports (30000+)
- **Port Access**: Requires `minikube ip` + NodePort for external access
- **Multi-cluster**: Multiple profiles with separate network namespaces
- **Registry**: Uses built-in registry or external registry with port forwarding
- **Ingress**: Requires ingress addon and specific routing

### Network Strategy Implementation

The implementation will include a dedicated **Network Manager** component that abstracts these differences:

```bash
# lib/network-manager.sh - New component for network abstraction
get_service_url() {
    local service_name="$1"
    local namespace="$2" 
    local port_name="$3"
    local cluster_type="$4"
    
    case "${cluster_type}" in
        "kind")
            kind_get_service_url "${service_name}" "${namespace}" "${port_name}"
            ;;
        "minikube")
            minikube_get_service_url "${service_name}" "${namespace}" "${port_name}"
            ;;
    esac
}

kind_get_service_url() {
    local service_name="$1"
    local namespace="$2"
    local port_name="$3"
    
    # Wait for LoadBalancer IP
    kubectl wait --for=jsonpath='{.status.loadBalancer.ingress}' \
        -n "${namespace}" service/"${service_name}" --timeout=300s
    
    local lb_ip=$(kubectl get svc "${service_name}" -n "${namespace}" \
        -o=jsonpath='{.status.loadBalancer.ingress[0].ip}')
    
    local port="80"
    if [[ "${port_name}" == "https" ]]; then
        port="443"
    fi
    
    echo "http://${lb_ip}:${port}"
}

minikube_get_service_url() {
    local service_name="$1"
    local namespace="$2"
    local port_name="$3"
    
    # Get NodePort
    local node_port=$(kubectl get svc "${service_name}" -n "${namespace}" \
        -o=jsonpath="{.spec.ports[?(@.name==\"${port_name}\")].nodePort}")
    
    # Get Minikube IP
    local minikube_ip=$(minikube ip -p "${MINIKUBE_PROFILE}")
    
    echo "http://${minikube_ip}:${node_port}"
}
```

## Detailed Implementation Plan

### Phase 1: Core Infrastructure with Network Abstraction

#### 1.1 Main Entry Script (`hack/run-integration-test-suite.sh`)
**Purpose**: Primary interface for all test suite operations
**Key Features**:
- Argument parsing with validation
- Environment detection (CI vs local)
- Cluster type detection and validation
- Logging and error handling
- Integration with existing CI workflows

**Implementation**:
```bash
#!/bin/bash
set -euo pipefail

# Main entry point - delegates to core orchestrator
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/integration-test-suite/lib/test-suite-runner.sh"

main() {
    parse_arguments "$@"
    validate_prerequisites
    setup_logging
    setup_cleanup_trap
    
    run_test_suite "${TEST_SUITE}" "${CLUSTER_TYPE}"
}

main "$@"
```

#### 1.2 Network Manager (`lib/network-manager.sh`)
**Purpose**: Abstract networking differences between cluster types
**Key Features**:
- Service URL resolution (LoadBalancer vs NodePort)
- Port forwarding management
- Multi-cluster network configuration
- Ingress/Gateway URL generation
- Network connectivity validation

**Network Configuration Templates**:
```yaml
# config/network/kind-network.yaml
network_type: "metallb"
load_balancer:
  enabled: true
  ip_range: "${LOAD_BALANCER_RANGE:-255.70-255.84}"
  wait_for_ip: true
service_access:
  type: "LoadBalancer"
  port_mapping: "direct"  # port 80 -> 80, 443 -> 443
ingress:
  type: "loadbalancer"
  wait_strategy: "ip_assignment"

# config/network/minikube-network.yaml
network_type: "nodeport"
load_balancer:
  enabled: false
  tunnel_required: true
service_access:
  type: "NodePort"
  port_mapping: "nodeport"  # port 80 -> 30000+
ingress:
  type: "nodeport"
  wait_strategy: "port_assignment"
addons:
  - "ingress"
  - "metallb"  # optional
```

#### 1.3 Enhanced Cluster Providers

**KinD Provider (`providers/kind-provider.sh`)**:
```bash
kind_create_cluster() {
    local cluster_name="$1"
    local config_file="$2"
    
    # Generate KinD config with MetalLB
    local kind_config="/tmp/kind-${cluster_name}.yaml"
    generate_kind_config "${config_file}" "${kind_config}"
    
    # Create cluster
    kind create cluster --name "${cluster_name}" --config "${kind_config}"
    
    # Install MetalLB
    install_metallb "${cluster_name}"
    
    # Configure load balancer IP range
    configure_metallb_ips "${cluster_name}"
    
    # Validate network connectivity
    validate_kind_network "${cluster_name}"
}

install_metallb() {
    local cluster_name="$1"
    
    info "Installing MetalLB for cluster ${cluster_name}"
    kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.7/config/manifests/metallb-native.yaml
    
    # Wait for MetalLB to be ready
    kubectl wait --namespace metallb-system \
        --for=condition=ready pod \
        --selector=app=metallb \
        --timeout=90s
}
```

**Minikube Provider (`providers/minikube-provider.sh`)**:
```bash
minikube_create_cluster() {
    local cluster_name="$1"
    local config_file="$2"
    
    # Use profile-based approach for multi-cluster
    local profile_name="${MINIKUBE_PROFILE_PREFIX:-ci}-${cluster_name}"
    
    # Start minikube with specific profile
    minikube start \
        --profile="${profile_name}" \
        --kubernetes-version="${K8S_VERSION}" \
        --memory="${K8S_MEMORY}" \
        --cpus="${K8S_CPU}" \
        --disk-size="${K8S_DISK}"
    
    # Enable required addons
    minikube addons enable ingress -p "${profile_name}"
    
    if [[ "${ENABLE_METALLB:-false}" == "true" ]]; then
        minikube addons enable metallb -p "${profile_name}"
        configure_minikube_metallb "${profile_name}"
    fi
    
    # Validate network connectivity
    validate_minikube_network "${profile_name}"
}

configure_minikube_metallb() {
    local profile_name="$1"
    
    # Get minikube IP range
    local minikube_ip=$(minikube ip -p "${profile_name}")
    local ip_base=$(echo "${minikube_ip}" | cut -d. -f1-3)
    local lb_range="${ip_base}.200-${ip_base}.250"
    
    # Configure MetalLB
    kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: metallb-system
  name: config
data:
  config: |
    address-pools:
    - name: default
      protocol: layer2
      addresses:
      - ${lb_range}
EOF
}
```

#### 1.4 Service Access Abstraction

**Service URL Resolution**:
```bash
# lib/service-resolver.sh
resolve_kiali_url() {
    local cluster_type="$1"
    local namespace="${2:-istio-system}"
    
    case "${cluster_type}" in
        "kind")
            resolve_kiali_url_kind "${namespace}"
            ;;
        "minikube")
            resolve_kiali_url_minikube "${namespace}"
            ;;
    esac
}

resolve_kiali_url_kind() {
    local namespace="$1"
    
    # Wait for LoadBalancer IP
    kubectl wait --for=jsonpath='{.status.loadBalancer.ingress}' \
        -n "${namespace}" service/kiali --timeout=300s
    
    local kiali_ip=$(kubectl get svc kiali -n "${namespace}" \
        -o=jsonpath='{.status.loadBalancer.ingress[0].ip}')
    
    export KIALI_URL="http://${kiali_ip}/kiali"
    export CYPRESS_BASE_URL="http://${kiali_ip}"
}

resolve_kiali_url_minikube() {
    local namespace="$1"
    
    # Check if using LoadBalancer with tunnel
    if service_has_loadbalancer "kiali" "${namespace}" && minikube_tunnel_active; then
        resolve_kiali_url_kind "${namespace}"  # Same as KinD
    else
        # Use NodePort approach
        local node_port=$(kubectl get svc kiali -n "${namespace}" \
            -o=jsonpath='{.spec.ports[?(@.name=="http")].nodePort}')
        local minikube_ip=$(minikube ip -p "${MINIKUBE_PROFILE}")
        
        export KIALI_URL="http://${minikube_ip}:${node_port}/kiali"
        export CYPRESS_BASE_URL="http://${minikube_ip}:${node_port}"
    fi
}
```

### Phase 2: Enhanced Component Installation

#### 2.1 Network-Aware Component Installation

**Kiali Installer (`installers/kiali-installer.sh`)**:
```bash
kiali_install() {
    local suite_config="$1"
    local cluster_type="$2"
    
    # Determine service type based on cluster
    local service_type
    case "${cluster_type}" in
        "kind")
            service_type="LoadBalancer"
            ;;
        "minikube")
            if minikube_supports_loadbalancer; then
                service_type="LoadBalancer"
            else
                service_type="NodePort"
            fi
            ;;
    esac
    
    # Install Kiali with appropriate service type
    helm upgrade --install kiali kiali-server \
        --set deployment.service_type="${service_type}" \
        --namespace istio-system \
        --wait --timeout=300s
    
    # Validate installation
    kiali_validate_installation "${cluster_type}"
}
```

**Istio Installer with Network Configuration**:
```bash
istio_install() {
    local suite_config="$1"
    local cluster_type="$2"
    
    # Install Istio base
    istioctl install --set values.pilot.env.PILOT_ENABLE_CROSS_CLUSTER_WORKLOAD_ENTRY=true -y
    
    # Configure ingress gateway based on cluster type
    configure_istio_ingress "${cluster_type}"
    
    # Install addons with appropriate service types
    install_istio_addons "${cluster_type}"
}

configure_istio_ingress() {
    local cluster_type="$1"
    
    case "${cluster_type}" in
        "kind")
            # Use LoadBalancer for KinD
            kubectl patch svc istio-ingressgateway -n istio-system \
                -p '{"spec":{"type":"LoadBalancer"}}'
            ;;
        "minikube")
            # Use NodePort for Minikube (unless tunnel is available)
            if ! minikube_tunnel_active; then
                kubectl patch svc istio-ingressgateway -n istio-system \
                    -p '{"spec":{"type":"NodePort"}}'
            fi
            ;;
    esac
}
```

### Phase 3: Multi-Cluster Network Configuration

#### 3.1 Multi-Cluster Network Setup

**Multi-Cluster KinD Configuration**:
```bash
kind_setup_multicluster() {
    local primary_cluster="$1"
    local remote_cluster="$2"
    
    # Create clusters with different MetalLB ranges
    create_kind_cluster_with_metallb "${primary_cluster}" "255.70-255.84"
    create_kind_cluster_with_metallb "${remote_cluster}" "255.85-255.99"
    
    # Configure cross-cluster networking
    setup_kind_cluster_networking "${primary_cluster}" "${remote_cluster}"
    
    # Exchange cluster secrets
    exchange_cluster_secrets "${primary_cluster}" "${remote_cluster}"
}

setup_kind_cluster_networking() {
    local primary_cluster="$1"
    local remote_cluster="$2"
    
    # Get cluster networks
    local primary_network=$(kind get clusters | grep "${primary_cluster}")
    local remote_network=$(kind get clusters | grep "${remote_cluster}")
    
    # Configure network policies for cross-cluster communication
    configure_cross_cluster_network_policies "${primary_cluster}" "${remote_cluster}"
}
```

**Multi-Cluster Minikube Configuration**:
```bash
minikube_setup_multicluster() {
    local primary_cluster="$1"
    local remote_cluster="$2"
    
    # Create separate profiles
    local primary_profile="ci-${primary_cluster}"
    local remote_profile="ci-${remote_cluster}"
    
    # Start clusters with different IP ranges
    start_minikube_with_profile "${primary_profile}" "192.168.58.0/24"
    start_minikube_with_profile "${remote_profile}" "192.168.59.0/24"
    
    # Configure cluster networking
    setup_minikube_cluster_networking "${primary_profile}" "${remote_profile}"
}
```

### Phase 4: Test Execution with Network Awareness

#### 4.1 Network-Aware Test Configuration

**Cypress Environment Setup**:
```bash
setup_cypress_environment() {
    local cluster_type="$1"
    local test_suite="$2"
    
    # Resolve service URLs based on cluster type
    resolve_service_urls "${cluster_type}"
    
    # Set Cypress environment variables
    export CI="1"
    export TERM="xterm"
    export CYPRESS_BASE_URL="${KIALI_URL}"
    export CYPRESS_AUTH_STRATEGY="${AUTH_STRATEGY}"
    export CYPRESS_AUTH_PROVIDER="${AUTH_PROVIDER:-my_htpasswd_provider}"
    
    # Set cluster-specific timeouts
    case "${cluster_type}" in
        "kind")
            export CYPRESS_COMMAND_TIMEOUT="30000"
            export CYPRESS_REQUEST_TIMEOUT="10000"
            ;;
        "minikube")
            # Minikube may be slower, especially with NodePort
            export CYPRESS_COMMAND_TIMEOUT="60000"
            export CYPRESS_REQUEST_TIMEOUT="20000"
            ;;
    esac
}
```

**Backend Test Configuration**:
```bash
setup_backend_test_environment() {
    local cluster_type="$1"
    
    # Configure kubeconfig for the test
    setup_kubeconfig_for_tests "${cluster_type}"
    
    # Set cluster-specific timeouts
    case "${cluster_type}" in
        "kind")
            export TEST_TIMEOUT="600s"
            ;;
        "minikube")
            export TEST_TIMEOUT="900s"  # Longer timeout for Minikube
            ;;
    esac
}
```

## File Structure and Organization

```
kiali/hack/
├── run-integration-test-suite.sh              # Main entry point
├── integration-test-suite/                    # New directory for all components
│   ├── lib/                                   # Core library functions
│   │   ├── test-suite-runner.sh              # Main orchestrator
│   │   ├── cluster-manager.sh                # Cluster operations
│   │   ├── component-installer.sh            # Component management
│   │   ├── test-executor.sh                  # Test execution
│   │   ├── debug-collector.sh                # Debug info collection
│   │   ├── network-manager.sh                # Network abstraction (NEW)
│   │   ├── service-resolver.sh               # Service URL resolution (NEW)
│   │   ├── utils.sh                          # Common utilities
│   │   └── validation.sh                     # Pre-flight checks
│   ├── providers/                             # Cluster providers
│   │   ├── kind-provider.sh                  # KinD operations
│   │   └── minikube-provider.sh               # Minikube operations
│   ├── installers/                           # Component installers
│   │   ├── istio-installer.sh                # Istio management
│   │   ├── kiali-installer.sh                # Kiali management
│   │   ├── keycloak-installer.sh              # Keycloak management
│   │   ├── tempo-installer.sh                # Tempo management
│   │   └── grafana-installer.sh               # Grafana management
│   ├── executors/                            # Test executors
│   │   ├── backend-executor.sh               # Go test execution
│   │   └── frontend-executor.sh              # Cypress execution
│   ├── config/                               # Configuration files
│   │   ├── suites/                           # Test suite definitions
│   │   │   ├── backend.yaml
│   │   │   ├── backend-external-controlplane.yaml
│   │   │   ├── frontend.yaml
│   │   │   ├── frontend-ambient.yaml
│   │   │   ├── frontend-multicluster-primary-remote.yaml
│   │   │   ├── frontend-multicluster-multi-primary.yaml
│   │   │   ├── frontend-external-kiali.yaml
│   │   │   ├── frontend-tempo.yaml
│   │   │   └── local.yaml
│   │   ├── clusters/                         # Cluster configurations
│   │   │   ├── single-kind.yaml
│   │   │   ├── multi-kind.yaml
│   │   │   └── minikube-profiles.yaml
│   │   ├── network/                          # Network configurations (NEW)
│   │   │   ├── kind-network.yaml
│   │   │   ├── minikube-network.yaml
│   │   │   └── multicluster-network.yaml
│   │   └── components/                       # Component configurations
│   │       ├── istio-standard.yaml
│   │       ├── istio-ambient.yaml
│   │       ├── istio-multicluster.yaml
│   │       ├── kiali-in-cluster.yaml
│   │       ├── kiali-external.yaml
│   │       ├── keycloak.yaml
│   │       ├── tempo.yaml
│   │       └── grafana.yaml
│   ├── templates/                            # Resource templates
│   │   ├── clusters/                         # Cluster templates
│   │   │   ├── kind-single.yaml
│   │   │   ├── kind-multi.yaml
│   │   │   └── minikube-config.yaml
│   │   ├── network/                          # Network templates (NEW)
│   │   │   ├── metallb-config.yaml
│   │   │   ├── ingress-config.yaml
│   │   │   └── service-templates/
│   │   │       ├── loadbalancer.yaml
│   │   │       └── nodeport.yaml
│   │   ├── istio/                            # Istio templates
│   │   │   ├── standard.yaml
│   │   │   ├── ambient.yaml
│   │   │   └── multicluster/
│   │   │       ├── primary.yaml
│   │   │       └── remote.yaml
│   │   └── kiali/                            # Kiali templates
│   │       ├── in-cluster.yaml
│   │       ├── external.yaml
│   │       └── local-mode.yaml
│   └── scripts/                              # Utility scripts
│       ├── wait-for-deployment.sh
│       ├── port-forward.sh
│       ├── cleanup-resources.sh
│       ├── validate-network.sh               # Network validation (NEW)
│       └── validate-environment.sh
```

## Test Suite Configuration Examples

### Network-Aware Test Suite Configuration

```yaml
# config/suites/frontend-multicluster-primary-remote.yaml
name: "frontend-multicluster-primary-remote"
description: "Frontend tests with primary-remote multicluster setup"
clusters:
  count: 2
  type: "${CLUSTER_TYPE}"
  network_config: "multicluster-network"
  configs:
    - name: "primary"
      template: "multicluster-primary"
      context: "${CLUSTER_TYPE}-primary"
      network:
        metallb_range: "255.70-255.84"  # KinD
        minikube_profile: "ci-primary"   # Minikube
    - name: "remote" 
      template: "multicluster-remote"
      context: "${CLUSTER_TYPE}-remote"
      network:
        metallb_range: "255.85-255.99"  # KinD  
        minikube_profile: "ci-remote"    # Minikube
components:
  istio:
    required: true
    mode: "multicluster"
    topology: "primary-remote"
    version: "${ISTIO_VERSION:-latest}"
    primary_cluster: "primary"
    remote_clusters: ["remote"]
    network_config:
      service_type: "${SERVICE_TYPE:-auto}"  # auto-detected
      cross_cluster_discovery: true
  kiali:
    required: true
    deployment_type: "in-cluster"
    cluster: "primary"
    remote_clusters: ["remote"]
    network_config:
      service_type: "${SERVICE_TYPE:-auto}"
      external_access: true
test_execution:
  type: "cypress"
  config_file: "cypress.config.ts"
  pattern: "**/*.feature"
  environment:
    CI: "1"
    TERM: "xterm"
    CYPRESS_BASE_URL: "${KIALI_URL}"  # Resolved at runtime
    CYPRESS_COMMAND_TIMEOUT: "${CYPRESS_TIMEOUT:-40000}"
network:
  validation:
    - "kiali_accessible"
    - "istio_gateway_accessible"
    - "cross_cluster_connectivity"
timeout: 3600
```

## Implementation Strategy and Coding Approach

### 1. Network-First Development Methodology

#### 1.1 Network Abstraction Priority
- Build network abstraction layer first
- Test with both KinD and Minikube before proceeding
- Validate service access patterns early
- Create network validation utilities

#### 1.2 Cluster-Specific Testing
- Test each provider in isolation
- Validate multi-cluster scenarios separately
- Test network connectivity between clusters
- Validate service discovery across clusters

### 2. Network Configuration Management

#### 2.1 Service Type Detection
```bash
# lib/network-manager.sh
detect_optimal_service_type() {
    local cluster_type="$1"
    local component="$2"
    
    case "${cluster_type}" in
        "kind")
            echo "LoadBalancer"
            ;;
        "minikube")
            if minikube_tunnel_active && metallb_available; then
                echo "LoadBalancer"
            else
                echo "NodePort"
            fi
            ;;
    esac
}

minikube_tunnel_active() {
    # Check if minikube tunnel is running
    pgrep -f "minikube.*tunnel" > /dev/null
}

metallb_available() {
    # Check if MetalLB is installed and configured
    kubectl get configmap -n metallb-system config > /dev/null 2>&1
}
```

#### 2.2 Port Management
```bash
# lib/service-resolver.sh
allocate_port_range() {
    local cluster_type="$1"
    local cluster_name="$2"
    
    case "${cluster_type}" in
        "kind")
            # Use IP-based allocation
            allocate_metallb_ip_range "${cluster_name}"
            ;;
        "minikube")
            # Use port-based allocation  
            allocate_nodeport_range "${cluster_name}"
            ;;
    esac
}

allocate_metallb_ip_range() {
    local cluster_name="$1"
    local base_ip="255"
    
    case "${cluster_name}" in
        *primary*|*cluster1*) echo "${base_ip}.70-${base_ip}.84" ;;
        *remote*|*cluster2*)  echo "${base_ip}.85-${base_ip}.99" ;;
        *) echo "${base_ip}.100-${base_ip}.114" ;;
    esac
}
```

### 3. Multi-Cluster Network Coordination

#### 3.1 Cross-Cluster Service Discovery
```bash
# lib/multicluster-network.sh
setup_cross_cluster_service_discovery() {
    local primary_cluster="$1"
    local remote_cluster="$2"
    local cluster_type="$3"
    
    case "${cluster_type}" in
        "kind")
            setup_kind_cross_cluster_discovery "${primary_cluster}" "${remote_cluster}"
            ;;
        "minikube")
            setup_minikube_cross_cluster_discovery "${primary_cluster}" "${remote_cluster}"
            ;;
    esac
}

setup_kind_cross_cluster_discovery() {
    local primary_cluster="$1"
    local remote_cluster="$2"
    
    # Get LoadBalancer IPs for cross-cluster communication
    local primary_istio_ip=$(kubectl --context "kind-${primary_cluster}" \
        get svc istio-ingressgateway -n istio-system \
        -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
    
    local remote_istio_ip=$(kubectl --context "kind-${remote_cluster}" \
        get svc istio-ingressgateway -n istio-system \
        -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
    
    # Configure Istio for cross-cluster discovery
    configure_istio_cross_cluster_endpoints "${primary_cluster}" "${remote_cluster}" \
        "${primary_istio_ip}" "${remote_istio_ip}"
}
```

#### 3.2 Network Validation
```bash
# scripts/validate-network.sh
validate_cluster_network() {
    local cluster_type="$1"
    local cluster_name="$2"
    
    info "Validating network for ${cluster_type} cluster ${cluster_name}"
    
    # Test basic connectivity
    validate_basic_connectivity "${cluster_type}" "${cluster_name}"
    
    # Test service access
    validate_service_access "${cluster_type}" "${cluster_name}"
    
    # Test ingress/gateway access
    validate_ingress_access "${cluster_type}" "${cluster_name}"
}

validate_service_access() {
    local cluster_type="$1"
    local cluster_name="$2"
    
    case "${cluster_type}" in
        "kind")
            validate_loadbalancer_access "${cluster_name}"
            ;;
        "minikube")
            validate_nodeport_access "${cluster_name}"
            ;;
    esac
}
```

## Performance Optimization for Network Operations

### 1. Parallel Network Setup
```bash
# lib/cluster-manager.sh
create_clusters_with_network_parallel() {
    local suite_config="$1"
    local cluster_configs=($(yq eval '.clusters.configs[].name' "${suite_config}"))
    
    local pids=()
    for cluster in "${cluster_configs[@]}"; do
        (
            create_single_cluster "${cluster}" "${suite_config}"
            setup_cluster_network "${cluster}" "${suite_config}"
        ) &
        pids+=($!)
    done
    
    # Wait for all clusters and networks
    for pid in "${pids[@]}"; do
        wait ${pid} || error "Cluster creation with network setup failed"
    done
    
    # Configure cross-cluster networking after all clusters are ready
    if [[ ${#cluster_configs[@]} -gt 1 ]]; then
        setup_cross_cluster_networking "${suite_config}"
    fi
}
```

### 2. Network Readiness Optimization
```bash
# lib/network-manager.sh
wait_for_network_ready() {
    local cluster_type="$1"
    local service_name="$2"
    local namespace="$3"
    
    case "${cluster_type}" in
        "kind")
            # Wait for LoadBalancer IP assignment
            kubectl wait --for=jsonpath='{.status.loadBalancer.ingress}' \
                -n "${namespace}" service/"${service_name}" --timeout=300s
            ;;
        "minikube")
            # Wait for NodePort assignment and accessibility
            wait_for_nodeport_ready "${service_name}" "${namespace}"
            ;;
    esac
}

wait_for_nodeport_ready() {
    local service_name="$1"
    local namespace="$2"
    
    # Wait for NodePort assignment
    local max_attempts=60
    local attempt=0
    
    while [[ ${attempt} -lt ${max_attempts} ]]; do
        local node_port=$(kubectl get svc "${service_name}" -n "${namespace}" \
            -o jsonpath='{.spec.ports[0].nodePort}' 2>/dev/null)
        
        if [[ -n "${node_port}" ]] && [[ "${node_port}" != "null" ]]; then
            # Test accessibility
            local minikube_ip=$(minikube ip -p "${MINIKUBE_PROFILE}")
            if curl -s --connect-timeout 5 "http://${minikube_ip}:${node_port}" >/dev/null; then
                info "NodePort ${node_port} is accessible"
                return 0
            fi
        fi
        
        sleep 5
        ((attempt++))
    done
    
    error "NodePort for ${service_name} never became accessible"
}
```

## Integration with Existing CI/CD

### 1. GitHub Actions Integration

The new system will be a drop-in replacement for existing workflows. All current integration test workflows will be updated to use the new unified script.

#### Current Workflows to Update:
- `integration-tests-backend.yml`
- `integration-tests-backend-multicluster-external-controlplane.yml`
- `integration-tests-frontend.yml`
- `integration-tests-frontend-ambient.yml`
- `integration-tests-frontend-multicluster-primary-remote.yml`
- `integration-tests-frontend-multicluster-multi-primary.yml`
- `integration-tests-frontend-multicluster-external-kiali.yml`
- `integration-tests-frontend-tempo.yml`
- `integration-tests-frontend-local.yml`

#### Migration Strategy:
1. **Phase 1**: Update workflows to call new script alongside existing (parallel validation)
2. **Phase 2**: Switch to new script only after validation
3. **Phase 3**: Remove old script calls and cleanup

#### Updated Workflow Examples:

**Backend Integration Tests** (`integration-tests-backend.yml`):
```yaml
name: Integration Tests Backend

on:
  workflow_call:
    inputs:
      target_branch:
        required: true
        type: string
      build_branch:
        required: true
        type: string
      istio_version:
        required: false
        type: string
        default: ""

env:
  TARGET_BRANCH: ${{ inputs.target_branch }}
  BUILD_BRANCH: ${{ inputs.build_branch || inputs.target_branch }}

jobs:
  integration_tests_backend:
    name: Backend API integration tests
    runs-on: ubuntu-latest
    steps:
    - name: Check out code
      uses: actions/checkout@v5
      with:
        ref: ${{ inputs.build_branch }}

    - name: Install Helm
      uses: Azure/setup-helm@v4.2.0
      with:
        version: 'v3.18.4'

    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version-file: go.mod
        cache: true
        cache-dependency-path: go.sum

    - name: Download go binary
      uses: actions/download-artifact@v5
      with:
        name: kiali
        path: ~/go/bin/

    - name: Ensure kiali binary is executable
      run: chmod +x ~/go/bin/kiali

    # NEW: Single unified command replaces multiple steps
    - name: Run backend integration tests
      id: intTests
      run: |
        hack/run-integration-test-suite.sh \
          --test-suite backend \
          --cluster-type kind \
          $(if [ -n "${{ inputs.istio_version }}" ]; then echo "--istio-version ${{ inputs.istio_version }}"; fi)

    # Debug info collection is now handled by the unified script
    - name: Upload debug info artifact
      if: ${{ failure() && steps.intTests.conclusion == 'failure' }}
      uses: actions/upload-artifact@v4
      with:
        name: debug-info-backend-${{ github.run_id }}
        path: debug-output/
```

**Frontend Ambient Tests** (`integration-tests-frontend-ambient.yml`):
```yaml
name: Integration Tests Frontend Ambient

on:
  workflow_call:
    inputs:
      target_branch:
        required: true
        type: string
      build_branch:
        required: true
        type: string
      istio_version:
        required: false
        type: string
        default: ""

env:
  TARGET_BRANCH: ${{ inputs.target_branch }}
  BUILD_BRANCH: ${{ inputs.build_branch || inputs.target_branch }}

jobs:
  integration_tests_frontend_ambient:
    name: Cypress integration tests (Ambient)
    runs-on: ubuntu-latest
    env:
      CI: 1
      TERM: xterm
    steps:
    - name: Check out code
      uses: actions/checkout@v5
      with:
        ref: ${{ inputs.build_branch }}

    - name: Install Helm
      uses: Azure/setup-helm@v4.2.0
      with:
        version: 'v3.18.4'

    - name: Setup node
      uses: actions/setup-node@v4
      with:
        node-version: "20"
        cache: yarn
        cache-dependency-path: frontend/yarn.lock

    - name: Download go binary
      uses: actions/download-artifact@v5
      with:
        name: kiali
        path: ~/go/bin/

    - name: Ensure kiali binary is executable
      run: chmod +x ~/go/bin/kiali

    - name: Install frontend dependencies
      working-directory: ./frontend
      run: yarn install --frozen-lockfile

    # NEW: Single unified command
    - name: Run frontend ambient integration tests
      id: intTests
      run: |
        hack/run-integration-test-suite.sh \
          --test-suite frontend-ambient \
          --cluster-type kind \
          $(if [ -n "${{ inputs.istio_version }}" ]; then echo "--istio-version ${{ inputs.istio_version }}"; fi)

    # Artifact collection handled by unified script
    - name: Upload debug info artifact
      if: failure()
      uses: actions/upload-artifact@v4
      with:
        name: debug-info-frontend-ambient-${{ github.run_id }}
        path: debug-output/

    - name: Upload cypress artifacts
      if: failure()
      uses: actions/upload-artifact@v4
      with:
        name: cypress-artifacts-frontend-ambient-${{ github.run_id }}
        path: |
          frontend/cypress/screenshots/
          frontend/cypress/videos/
```

**Multicluster Primary-Remote Tests** (`integration-tests-frontend-multicluster-primary-remote.yml`):
```yaml
name: Integration Tests Frontend Multicluster Primary-Remote

on:
  workflow_call:
    inputs:
      target_branch:
        required: true
        type: string
      build_branch:
        required: true
        type: string
      istio_version:
        required: false
        type: string
        default: ""

env:
  TARGET_BRANCH: ${{ inputs.target_branch }}
  BUILD_BRANCH: ${{ inputs.build_branch || inputs.target_branch }}

jobs:
  integration_tests_frontend_multicluster:
    name: Cypress integration tests (Multicluster Primary-Remote)
    runs-on: ubuntu-latest
    env:
      CI: 1
      TERM: xterm
    steps:
    - name: Check out code
      uses: actions/checkout@v5
      with:
        ref: ${{ inputs.build_branch }}

    - name: Install Helm
      uses: Azure/setup-helm@v4.2.0
      with:
        version: 'v3.18.4'

    - name: Setup node
      uses: actions/setup-node@v4
      with:
        node-version: "20"
        cache: yarn
        cache-dependency-path: frontend/yarn.lock

    - name: Download go binary
      uses: actions/download-artifact@v5
      with:
        name: kiali
        path: ~/go/bin/

    - name: Ensure kiali binary is executable
      run: chmod +x ~/go/bin/kiali

    - name: Install frontend dependencies
      working-directory: ./frontend
      run: yarn install --frozen-lockfile

    # NEW: Single unified command for complex multicluster setup
    - name: Run frontend multicluster primary-remote integration tests
      id: intTests
      run: |
        hack/run-integration-test-suite.sh \
          --test-suite frontend-multicluster-primary-remote \
          --cluster-type kind \
          --timeout 4800 \
          $(if [ -n "${{ inputs.istio_version }}" ]; then echo "--istio-version ${{ inputs.istio_version }}"; fi)

    - name: Upload debug info artifact
      if: failure()
      uses: actions/upload-artifact@v4
      with:
        name: debug-info-multicluster-pr-${{ github.run_id }}
        path: debug-output/

    - name: Upload cypress artifacts
      if: failure()
      uses: actions/upload-artifact@v4
      with:
        name: cypress-artifacts-multicluster-pr-${{ github.run_id }}
        path: |
          frontend/cypress/screenshots/
          frontend/cypress/videos/
```

**Local Mode Tests** (`integration-tests-frontend-local.yml`):
```yaml
name: Integration Tests Frontend Local Mode

on:
  workflow_call:
    inputs:
      target_branch:
        required: true
        type: string
      build_branch:
        required: true
        type: string
      istio_version:
        required: false
        type: string
        default: ""
      stern_logs:
        required: false
        type: boolean
        default: false

env:
  TARGET_BRANCH: ${{ inputs.target_branch }}

jobs:
  integration_tests_frontend_local:
    name: Cypress integration tests (Local Mode)
    runs-on: ubuntu-latest
    env:
      CI: 1
      TERM: xterm
    steps:
    - name: Check out code
      uses: actions/checkout@v5
      with:
        ref: ${{ inputs.build_branch }}

    - name: Install Helm
      uses: Azure/setup-helm@v4.2.0
      with:
        version: 'v3.18.4'

    - name: Setup node
      uses: actions/setup-node@v4
      with:
        node-version: "20"
        cache: yarn
        cache-dependency-path: frontend/yarn.lock

    - name: Download go binary
      uses: actions/download-artifact@v5
      with:
        name: kiali
        path: ~/go/bin/

    - name: Ensure kiali binary is executable
      run: chmod +x ~/go/bin/kiali

    - name: Install frontend dependencies
      working-directory: ./frontend
      run: yarn install --frozen-lockfile

    # NEW: Local mode with no cluster setup
    - name: Run frontend local mode integration tests
      id: intTests
      run: |
        hack/run-integration-test-suite.sh \
          --test-suite local \
          --cluster-type kind \
          $(if [ -n "${{ inputs.istio_version }}" ]; then echo "--istio-version ${{ inputs.istio_version }}"; fi) \
          $(if [ "${{ inputs.stern_logs }}" == "true" ]; then echo "--debug true"; fi)

    - name: Upload debug info artifact
      if: failure()
      uses: actions/upload-artifact@v4
      with:
        name: debug-info-local-${{ github.run_id }}
        path: debug-output/

    - name: Upload cypress artifacts
      if: failure()
      uses: actions/upload-artifact@v4
      with:
        name: cypress-artifacts-local-${{ github.run_id }}
        path: |
          frontend/cypress/screenshots/
          frontend/cypress/videos/
```

#### Workflow Migration Benefits:
1. **Simplified Maintenance**: Single script to maintain instead of multiple workflow-specific logic
2. **Consistent Environment**: Same setup logic across all test types
3. **Better Error Handling**: Unified debug collection and artifact management
4. **Reduced Duplication**: Common setup steps consolidated
5. **Enhanced Debugging**: Comprehensive debug info collection built-in
6. **Network Awareness**: Proper handling of KinD networking in CI

#### Workflow Parameters Mapping:
- `--test-suite`: Maps directly to the test suite name
- `--cluster-type`: Always `kind` for CI (GitHub Actions)
- `--istio-version`: Pass through from workflow inputs
- `--timeout`: Configurable per test suite (multicluster needs longer)
- `--debug`: Enabled automatically on failure, or via workflow input

#### Complete Workflow Migration Plan:

**Phase 1: Parallel Validation (Week 7)**
```yaml
# Example: Run both old and new systems in parallel
- name: Run integration tests (current)
  id: currentTests
  continue-on-error: true
  run: hack/run-integration-tests.sh --test-suite backend

- name: Run integration tests (new)
  id: newTests
  run: |
    hack/run-integration-test-suite.sh \
      --test-suite backend \
      --cluster-type kind

- name: Compare results
  run: |
    if [[ "${{ steps.currentTests.outcome }}" != "${{ steps.newTests.outcome }}" ]]; then
      echo "Results differ between old and new systems"
      exit 1
    fi
```

**Phase 2: Switch to New System (Week 8)**
- Update all workflows to use new script only
- Remove old script calls
- Update artifact collection paths

**Phase 3: Cleanup (Week 8)**
- Remove old `hack/run-integration-tests.sh` script
- Remove old CI helper scripts that are now integrated
- Update documentation

#### Workflow Files to Update:

1. **Backend Tests**:
   - `integration-tests-backend.yml`
   - `integration-tests-backend-multicluster-external-controlplane.yml`

2. **Frontend Tests**:
   - `integration-tests-frontend.yml` 
   - `integration-tests-frontend-ambient.yml`
   - `integration-tests-frontend-tempo.yml`
   - `integration-tests-frontend-local.yml`

3. **Multicluster Tests**:
   - `integration-tests-frontend-multicluster-primary-remote.yml`
   - `integration-tests-frontend-multicluster-multi-primary.yml`
   - `integration-tests-frontend-multicluster-external-kiali.yml`

#### Additional Workflow Considerations:

**Artifact Collection Enhancement**:
```yaml
# Enhanced artifact collection with better organization
- name: Upload comprehensive debug artifacts
  if: failure()
  uses: actions/upload-artifact@v4
  with:
    name: debug-artifacts-${{ matrix.test-suite }}-${{ github.run_id }}
    path: |
      debug-output/
      frontend/cypress/screenshots/
      frontend/cypress/videos/
      ~/.kube/config
    retention-days: 7
```

**Matrix Strategy Support**:
```yaml
# Enable matrix builds for testing multiple configurations
strategy:
  matrix:
    istio-version: ["1.19.0", "1.20.0", "latest"]
    include:
      - istio-version: "1.19.0"
        timeout: 3600
      - istio-version: "1.20.0" 
        timeout: 3600
      - istio-version: "latest"
        timeout: 4800
```

**Conditional Test Execution**:
```yaml
# Skip certain tests based on conditions
- name: Run integration tests
  if: ${{ !contains(github.event.head_commit.message, '[skip-integration]') }}
  run: |
    hack/run-integration-test-suite.sh \
      --test-suite ${{ matrix.test-suite }} \
      --cluster-type kind \
      --timeout ${{ matrix.timeout }}
```

### 2. Local Development Support
```bash
# Example local usage with Minikube
./hack/run-integration-test-suite.sh \
  --test-suite frontend \
  --cluster-type minikube \
  --setup-only true

# Example local usage with KinD
./hack/run-integration-test-suite.sh \
  --test-suite backend \
  --cluster-type kind \
  --debug true
```

## Risk Mitigation Strategies

### 1. Network-Specific Risks
- **LoadBalancer IP allocation failures**: Implement retry logic and fallback to NodePort
- **Port conflicts**: Dynamic port allocation and validation
- **Cross-cluster connectivity issues**: Comprehensive network validation and diagnostics
- **DNS resolution problems**: Custom DNS configuration and validation

### 2. Cluster-Specific Compatibility
- **KinD version compatibility**: Pin to tested KinD versions, validate before use
- **Minikube driver variations**: Support multiple drivers (docker, kvm2, hyperkit)
- **Network plugin differences**: Abstract network operations, validate functionality

### 3. Performance and Reliability
- **Network timeout handling**: Configurable timeouts based on cluster type
- **Resource cleanup**: Comprehensive cleanup of network resources
- **State recovery**: Ability to resume from partial failures

## Implementation Timeline

### Phase 1: Network Foundation (Week 1-2)
**Goal**: Network abstraction and basic cluster support

1. **Network Manager Implementation** (Days 1-3)
   - Network abstraction layer
   - Service URL resolution
   - Cluster type detection

2. **Enhanced Cluster Providers** (Days 4-6)
   - KinD provider with MetalLB
   - Minikube provider with NodePort/MetalLB
   - Network validation utilities

3. **Basic Test Suites** (Days 7-10)
   - `backend` and `frontend` with network awareness
   - Network-specific configuration
   - CI integration testing

**Deliverables**: 
- Network-aware `backend` and `frontend` test suites
- KinD and Minikube support with proper networking
- CI integration validated

### Phase 2: Multi-Cluster Networking (Week 3-4)
**Goal**: Multi-cluster configurations with cross-cluster networking

1. **Multi-Cluster Infrastructure** (Days 11-14)
   - Multi-cluster KinD with separate MetalLB ranges
   - Multi-cluster Minikube with profiles
   - Cross-cluster network configuration

2. **Advanced Istio Networking** (Days 15-18)
   - Multicluster service discovery
   - Cross-cluster load balancing
   - Network policy configuration

3. **Multi-Cluster Test Suites** (Days 19-20)
   - All multicluster test suites with network validation
   - Cross-cluster connectivity testing

**Deliverables**:
- All multi-cluster test suites working with proper networking
- Cross-cluster service discovery validated
- Network performance optimized

### Phase 3: Advanced Features (Week 5-6)
**Goal**: Complete feature set with optimization

1. **Additional Components** (Days 21-24)
   - Network-aware component installation
   - Service discovery integration
   - Performance optimization

2. **Final Integration** (Days 25-30)
   - Complete test suite coverage
   - Performance benchmarking
   - Documentation and migration guide

**Deliverables**:
- All test suites implemented with network awareness
- Performance targets met
- Complete migration from existing scripts

### Phase 4: GitHub Actions Workflow Migration (Week 7-8)
**Goal**: Migrate all existing GitHub Actions workflows to use the new unified system

1. **Workflow Analysis and Planning** (Days 31-32)
   - Analyze all 9 existing integration test workflows
   - Plan migration strategy with parallel validation
   - Prepare workflow templates and examples

2. **Parallel Validation Implementation** (Days 33-35)
   - Update workflows to run both old and new systems in parallel
   - Implement result comparison and validation
   - Test all workflow variations (different Istio versions, test suites)

3. **Migration Execution** (Days 36-38)
   - Switch workflows to use new system only
   - Update artifact collection and debug info handling
   - Implement enhanced features (matrix builds, conditional execution)

4. **Cleanup and Optimization** (Days 39-40)
   - Remove old `hack/run-integration-tests.sh` script
   - Remove deprecated CI helper scripts
   - Optimize workflow performance and artifact handling
   - Update all documentation

**Workflow Files Updated**:
- `integration-tests-backend.yml`
- `integration-tests-backend-multicluster-external-controlplane.yml`
- `integration-tests-frontend.yml`
- `integration-tests-frontend-ambient.yml`
- `integration-tests-frontend-multicluster-primary-remote.yml`
- `integration-tests-frontend-multicluster-multi-primary.yml`
- `integration-tests-frontend-multicluster-external-kiali.yml`
- `integration-tests-frontend-tempo.yml`
- `integration-tests-frontend-local.yml`

**Migration Benefits Achieved**:
- Single unified command for all test execution
- Consistent environment setup across all test types
- Enhanced debug collection and artifact management
- Better error handling and recovery
- Network-aware configuration for KinD clusters
- Simplified maintenance and reduced code duplication

**Deliverables**:
- All GitHub Actions workflows migrated and validated
- Old integration test scripts removed
- Enhanced artifact collection implemented
- Complete workflow documentation updated
- CI/CD pipeline fully optimized

This implementation plan provides a robust foundation for building the unified Kiali integration test suite with proper networking abstraction that handles the critical differences between KinD and Minikube cluster types, and includes a comprehensive migration plan for all existing GitHub Actions workflows.
