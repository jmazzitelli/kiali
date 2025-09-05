#!/bin/bash
#
# Validation functions for the Kiali Integration Test Suite
#

# Source utilities if not already sourced
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/utils.sh"
fi

# Required tools for different cluster types
declare -A CLUSTER_TOOLS
CLUSTER_TOOLS[kind]="kind kubectl docker helm"
CLUSTER_TOOLS[minikube]="minikube kubectl docker helm"

# Optional tools
OPTIONAL_TOOLS="yq jq stern"

# Validate prerequisites for the specified cluster type
validate_prerequisites() {
    local cluster_type="$1"
    
    info "Validating prerequisites for cluster type: ${cluster_type}"
    
    # Validate cluster type
    if [[ -z "${cluster_type}" ]]; then
        error "Cluster type not specified"
        return 1
    fi
    
    # Check required tools
    validate_required_tools "${cluster_type}"
    
    # Check optional tools
    validate_optional_tools
    
    # Check Docker daemon
    validate_docker_daemon
    
    # Check system resources
    validate_system_resources
    
    # Validate environment variables
    validate_environment_variables
    
    # Check for existing clusters that might conflict
    validate_cluster_conflicts "${cluster_type}"
    
    info "Prerequisites validation completed successfully"
}

# Validate required tools are installed
validate_required_tools() {
    local cluster_type="$1"
    local tools="${CLUSTER_TOOLS[${cluster_type}]:-}"
    
    if [[ -z "${tools}" ]]; then
        error "Unknown cluster type: ${cluster_type}"
        return 1
    fi
    
    info "Checking required tools for ${cluster_type}: ${tools}"
    
    local missing_tools=()
    for tool in ${tools}; do
        if ! command -v "${tool}" >/dev/null 2>&1; then
            missing_tools+=("${tool}")
        else
            local version
            case "${tool}" in
                kubectl)
                    version=$(kubectl version --client --short 2>/dev/null | cut -d' ' -f3 || echo "unknown")
                    ;;
                docker)
                    version=$(docker --version 2>/dev/null | cut -d' ' -f3 | sed 's/,//' || echo "unknown")
                    ;;
                helm)
                    version=$(helm version --short 2>/dev/null | cut -d' ' -f1 || echo "unknown")
                    ;;
                kind)
                    version=$(kind version 2>/dev/null | cut -d' ' -f2 || echo "unknown")
                    ;;
                minikube)
                    version=$(minikube version --short 2>/dev/null | cut -d' ' -f3 || echo "unknown")
                    ;;
                *)
                    version="installed"
                    ;;
            esac
            debug "✓ ${tool}: ${version}"
        fi
    done
    
    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        error "Missing required tools: ${missing_tools[*]}"
        error "Please install the missing tools and try again"
        return 1
    fi
    
    info "All required tools are available"
}

# Validate optional tools
validate_optional_tools() {
    info "Checking optional tools: ${OPTIONAL_TOOLS}"
    
    for tool in ${OPTIONAL_TOOLS}; do
        if command -v "${tool}" >/dev/null 2>&1; then
            local version
            case "${tool}" in
                yq)
                    version=$(yq --version 2>/dev/null | cut -d' ' -f4 || echo "unknown")
                    ;;
                jq)
                    version=$(jq --version 2>/dev/null | sed 's/jq-//' || echo "unknown")
                    ;;
                stern)
                    version=$(stern --version 2>/dev/null | cut -d' ' -f2 || echo "unknown")
                    ;;
                *)
                    version="installed"
                    ;;
            esac
            debug "✓ ${tool}: ${version}"
        else
            warn "Optional tool not found: ${tool}"
            case "${tool}" in
                yq)
                    warn "  yq is recommended for YAML processing"
                    warn "  Install: https://github.com/mikefarah/yq"
                    ;;
                jq)
                    warn "  jq is recommended for JSON processing"
                    warn "  Install: https://stedolan.github.io/jq/"
                    ;;
                stern)
                    warn "  stern is recommended for log collection"
                    warn "  Install: https://github.com/stern/stern"
                    ;;
            esac
        fi
    done
}

# Validate Docker daemon is running
validate_docker_daemon() {
    info "Checking Docker daemon"
    
    if ! docker info >/dev/null 2>&1; then
        error "Docker daemon is not running or not accessible"
        error "Please start Docker and ensure your user has access to it"
        return 1
    fi
    
    # Check Docker version
    local docker_version
    docker_version=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo "unknown")
    debug "✓ Docker daemon: ${docker_version}"
    
    # Check available disk space for Docker
    local docker_root
    docker_root=$(docker info --format '{{.DockerRootDir}}' 2>/dev/null || echo "/var/lib/docker")
    local available_space
    available_space=$(df -BG "${docker_root}" 2>/dev/null | awk 'NR==2 {print $4}' | sed 's/G//' || echo "0")
    
    if [[ "${available_space}" -lt 10 ]]; then
        warn "Low disk space for Docker: ${available_space}GB available"
        warn "Recommend at least 10GB free space for integration tests"
    else
        debug "✓ Docker disk space: ${available_space}GB available"
    fi
    
    info "Docker daemon is ready"
}

# Validate system resources
validate_system_resources() {
    info "Checking system resources"
    
    # Check available memory
    local available_memory
    if command -v free >/dev/null 2>&1; then
        available_memory=$(free -g | awk '/^Mem:/ {print $7}')
        if [[ "${available_memory}" -lt 4 ]]; then
            warn "Low available memory: ${available_memory}GB"
            warn "Recommend at least 4GB available memory for integration tests"
        else
            debug "✓ Available memory: ${available_memory}GB"
        fi
    else
        debug "Cannot check memory (free command not available)"
    fi
    
    # Check CPU cores
    local cpu_cores
    cpu_cores=$(nproc 2>/dev/null || echo "unknown")
    if [[ "${cpu_cores}" != "unknown" ]]; then
        if [[ "${cpu_cores}" -lt 2 ]]; then
            warn "Low CPU cores: ${cpu_cores}"
            warn "Recommend at least 2 CPU cores for integration tests"
        else
            debug "✓ CPU cores: ${cpu_cores}"
        fi
    fi
    
    info "System resources check completed"
}

# Validate environment variables
validate_environment_variables() {
    debug "Checking environment variables"
    
    # Check KUBECONFIG if set
    if [[ -n "${KUBECONFIG:-}" ]]; then
        if [[ ! -f "${KUBECONFIG}" ]]; then
            warn "KUBECONFIG points to non-existent file: ${KUBECONFIG}"
        else
            debug "✓ KUBECONFIG: ${KUBECONFIG}"
        fi
    fi
    
    # Check HOME directory
    if [[ -z "${HOME:-}" ]]; then
        error "HOME environment variable not set"
        return 1
    fi
    
    # Check for Go binary path (needed for backend tests)
    if [[ -f "${HOME}/go/bin/kiali" ]]; then
        debug "✓ Kiali binary found: ${HOME}/go/bin/kiali"
    else
        warn "Kiali binary not found at ${HOME}/go/bin/kiali"
        warn "This is required for backend integration tests"
    fi
    
    debug "Environment variables check completed"
}

# Check for existing clusters that might conflict
validate_cluster_conflicts() {
    local cluster_type="$1"
    
    debug "Checking for cluster conflicts"
    
    case "${cluster_type}" in
        kind)
            # Check for existing kind clusters
            local existing_clusters
            existing_clusters=$(kind get clusters 2>/dev/null || echo "")
            if [[ -n "${existing_clusters}" ]]; then
                warn "Existing kind clusters found:"
                echo "${existing_clusters}" | while read -r cluster; do
                    warn "  - ${cluster}"
                done
                warn "These may interfere with test execution"
            fi
            ;;
        minikube)
            # Check for existing minikube profiles
            local existing_profiles
            existing_profiles=$(minikube profile list -o json 2>/dev/null | jq -r '.valid[].Name' 2>/dev/null || echo "")
            if [[ -n "${existing_profiles}" ]]; then
                warn "Existing minikube profiles found:"
                echo "${existing_profiles}" | while read -r profile; do
                    warn "  - ${profile}"
                done
                warn "These may interfere with test execution"
            fi
            ;;
    esac
    
    debug "Cluster conflicts check completed"
}

# Validate test suite configuration
validate_test_suite_config() {
    local test_suite="$1"
    local config_file="${INTEGRATION_TEST_DIR}/config/suites/${test_suite}.yaml"
    
    info "Validating test suite configuration: ${test_suite}"
    
    if [[ ! -f "${config_file}" ]]; then
        error "Test suite configuration not found: ${config_file}"
        return 1
    fi
    
    # Validate required fields in configuration
    local required_fields=(
        ".name"
        ".clusters.count"
        ".components.istio.required"
        ".components.kiali.required"
        ".test_execution.type"
    )
    
    for field in "${required_fields[@]}"; do
        local value
        value=$(get_yaml_value "${config_file}" "${field}")
        if [[ -z "${value}" || "${value}" == "null" ]]; then
            error "Missing required field in ${config_file}: ${field}"
            return 1
        fi
        debug "✓ ${field}: ${value}"
    done
    
    info "Test suite configuration is valid"
}

# Validate network connectivity
validate_network_connectivity() {
    info "Validating network connectivity"
    
    # Test internet connectivity (needed for downloading components)
    local test_urls=(
        "https://github.com"
        "https://registry-1.docker.io"
        "https://gcr.io"
    )
    
    for url in "${test_urls[@]}"; do
        if is_url_accessible "${url}" 5; then
            debug "✓ Network connectivity: ${url}"
        else
            warn "Cannot reach: ${url}"
            warn "This may cause issues downloading container images or components"
        fi
    done
    
    info "Network connectivity check completed"
}

# Validate Kubernetes cluster access
validate_kubernetes_access() {
    local context="${1:-}"
    
    info "Validating Kubernetes cluster access"
    
    local kubectl_cmd="kubectl cluster-info"
    if [[ -n "${context}" ]]; then
        kubectl_cmd="${kubectl_cmd} --context=${context}"
    fi
    
    if ${kubectl_cmd} >/dev/null 2>&1; then
        local cluster_version
        cluster_version=$(kubectl version --short 2>/dev/null | grep "Server Version" | cut -d' ' -f3 || echo "unknown")
        debug "✓ Kubernetes cluster access: ${cluster_version}"
        
        # Check cluster nodes
        local node_count
        node_count=$(kubectl get nodes --no-headers 2>/dev/null | wc -l)
        debug "✓ Cluster nodes: ${node_count}"
        
        info "Kubernetes cluster access validated"
        return 0
    else
        error "Cannot access Kubernetes cluster"
        if [[ -n "${context}" ]]; then
            error "Context: ${context}"
        fi
        return 1
    fi
}

# Validate component prerequisites
validate_component_prerequisites() {
    local component="$1"
    
    case "${component}" in
        istio)
            info "Validating Istio prerequisites"
            # Check if istioctl is available
            if ! command -v istioctl >/dev/null 2>&1; then
                warn "istioctl not found in PATH"
                warn "Will attempt to download istioctl during installation"
            else
                local istioctl_version
                istioctl_version=$(istioctl version --short --remote=false 2>/dev/null | cut -d':' -f2 | tr -d ' ' || echo "unknown")
                debug "✓ istioctl: ${istioctl_version}"
            fi
            ;;
        kiali)
            info "Validating Kiali prerequisites"
            # Check for Kiali binary (needed for backend tests)
            if [[ -f "${HOME}/go/bin/kiali" ]]; then
                debug "✓ Kiali binary available"
            else
                warn "Kiali binary not found (needed for backend tests)"
            fi
            ;;
        keycloak)
            info "Validating Keycloak prerequisites"
            # No specific prerequisites for Keycloak
            debug "✓ Keycloak prerequisites satisfied"
            ;;
        tempo)
            info "Validating Tempo prerequisites"
            # No specific prerequisites for Tempo
            debug "✓ Tempo prerequisites satisfied"
            ;;
        grafana)
            info "Validating Grafana prerequisites"
            # No specific prerequisites for Grafana
            debug "✓ Grafana prerequisites satisfied"
            ;;
        *)
            warn "Unknown component for prerequisite validation: ${component}"
            ;;
    esac
}

# Validate frontend test prerequisites
validate_frontend_prerequisites() {
    info "Validating frontend test prerequisites"
    
    # Check for Node.js and yarn
    if ! command -v node >/dev/null 2>&1; then
        error "Node.js not found (required for frontend tests)"
        return 1
    fi
    
    local node_version
    node_version=$(node --version 2>/dev/null)
    debug "✓ Node.js: ${node_version}"
    
    if ! command -v yarn >/dev/null 2>&1; then
        error "Yarn not found (required for frontend tests)"
        return 1
    fi
    
    local yarn_version
    yarn_version=$(yarn --version 2>/dev/null)
    debug "✓ Yarn: ${yarn_version}"
    
    # Check if frontend dependencies are installed
    local frontend_dir="${SCRIPT_DIR}/../frontend"
    if [[ -d "${frontend_dir}" && -f "${frontend_dir}/package.json" ]]; then
        if [[ ! -d "${frontend_dir}/node_modules" ]]; then
            warn "Frontend dependencies not installed"
            warn "Run 'yarn install' in the frontend directory"
        else
            debug "✓ Frontend dependencies appear to be installed"
        fi
    else
        error "Frontend directory not found: ${frontend_dir}"
        return 1
    fi
    
    info "Frontend test prerequisites validated"
}

# Validate backend test prerequisites  
validate_backend_prerequisites() {
    info "Validating backend test prerequisites"
    
    # Check for Go
    if ! command -v go >/dev/null 2>&1; then
        error "Go not found (required for backend tests)"
        return 1
    fi
    
    local go_version
    go_version=$(go version 2>/dev/null | cut -d' ' -f3)
    debug "✓ Go: ${go_version}"
    
    # Check for Kiali binary
    if [[ ! -f "${HOME}/go/bin/kiali" ]]; then
        error "Kiali binary not found at ${HOME}/go/bin/kiali"
        error "This is required for backend integration tests"
        return 1
    fi
    
    # Make sure the binary is executable
    if [[ ! -x "${HOME}/go/bin/kiali" ]]; then
        warn "Kiali binary is not executable, making it executable"
        chmod +x "${HOME}/go/bin/kiali"
    fi
    
    debug "✓ Kiali binary: ${HOME}/go/bin/kiali"
    
    info "Backend test prerequisites validated"
}

# Export validation functions
export -f validate_prerequisites validate_required_tools validate_optional_tools
export -f validate_docker_daemon validate_system_resources validate_environment_variables
export -f validate_cluster_conflicts validate_test_suite_config validate_network_connectivity
export -f validate_kubernetes_access validate_component_prerequisites
export -f validate_frontend_prerequisites validate_backend_prerequisites
