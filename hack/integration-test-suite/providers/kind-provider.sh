#!/bin/bash
#
# KinD Provider for Kiali Integration Test Suite
# Handles KinD cluster creation, configuration, and management
#

# Source required libraries
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils.sh"
fi

if ! declare -f setup_cluster_networking >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/network-manager.sh"
fi

# KinD configuration defaults
readonly KIND_VERSION_MIN="0.17.0"
readonly KIND_DEFAULT_IMAGE="kindest/node:v1.28.0"
readonly KIND_CLUSTER_PREFIX="kiali-test"

# Create KinD cluster with networking
kind_create_cluster() {
    local cluster_name="$1"
    local config_file="${2:-}"
    
    info "Creating KinD cluster: ${cluster_name}"
    
    # Validate KinD version
    if ! kind_validate_version; then
        return 1
    fi
    
    # Generate cluster configuration
    local kind_config_file
    kind_config_file=$(generate_kind_config "${cluster_name}" "${config_file}")
    
    # Delete existing cluster if it exists
    if kind get clusters 2>/dev/null | grep -q "^${cluster_name}$"; then
        warn "Cluster ${cluster_name} already exists, deleting it"
        kind_delete_cluster "${cluster_name}"
    fi
    
    # Create the cluster
    info "Creating KinD cluster with config: ${kind_config_file}"
    if ! kind create cluster --name "${cluster_name}" --config "${kind_config_file}" --wait 300s; then
        error "Failed to create KinD cluster: ${cluster_name}"
        return 1
    fi
    
    # Setup networking
    if ! setup_cluster_networking "kind" "${cluster_name}" "${config_file}"; then
        error "Failed to setup networking for KinD cluster: ${cluster_name}"
        kind_delete_cluster "${cluster_name}"
        return 1
    fi
    
    # Validate cluster is ready
    if ! kind_validate_cluster "${cluster_name}"; then
        error "KinD cluster validation failed: ${cluster_name}"
        kind_delete_cluster "${cluster_name}"
        return 1
    fi
    
    info "KinD cluster created successfully: ${cluster_name}"
    return 0
}

# Generate KinD configuration
generate_kind_config() {
    local cluster_name="$1"
    local config_file="${2:-}"
    
    local kind_config
    kind_config=$(create_temp_file "kind-config-${cluster_name}")
    
    # Get configuration values
    local k8s_image="${KIND_DEFAULT_IMAGE}"
    local worker_nodes="1"
    local disable_default_cni="false"
    local port_mappings=""
    
    if [[ -n "${config_file}" && -f "${config_file}" ]]; then
        k8s_image=$(get_yaml_value "${config_file}" ".kubernetes.image" "${KIND_DEFAULT_IMAGE}")
        worker_nodes=$(get_yaml_value "${config_file}" ".kubernetes.worker_nodes" "1")
        disable_default_cni=$(get_yaml_value "${config_file}" ".kubernetes.disable_default_cni" "false")
    fi
    
    # Generate port mappings for ingress
    if [[ "${cluster_name}" == *"primary"* ]] || [[ "${worker_nodes}" -gt 0 ]]; then
        port_mappings="
      - containerPort: 80
        hostPort: 80
        protocol: TCP
      - containerPort: 443
        hostPort: 443
        protocol: TCP"
    fi
    
    # Write KinD configuration
    cat > "${kind_config}" <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${cluster_name}
nodes:
- role: control-plane
  image: ${k8s_image}
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:${port_mappings}
EOF
    
    # Add worker nodes if specified
    if [[ "${worker_nodes}" -gt 0 ]]; then
        for ((i=1; i<=worker_nodes; i++)); do
            cat >> "${kind_config}" <<EOF
- role: worker
  image: ${k8s_image}
EOF
        done
    fi
    
    # Add networking configuration
    if [[ "${disable_default_cni}" == "true" ]]; then
        cat >> "${kind_config}" <<EOF
networking:
  disableDefaultCNI: true
  podSubnet: "10.244.0.0/16"
EOF
    fi
    
    debug "Generated KinD config for ${cluster_name}:"
    debug "$(cat "${kind_config}")"
    
    echo "${kind_config}"
}

# Validate KinD version
kind_validate_version() {
    if ! command -v kind >/dev/null 2>&1; then
        error "KinD not found in PATH"
        return 1
    fi
    
    local kind_version
    kind_version=$(kind version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 | sed 's/v//')
    
    if [[ -z "${kind_version}" ]]; then
        error "Cannot determine KinD version"
        return 1
    fi
    
    local version_check
    version_check=$(version_compare "${kind_version}" "${KIND_VERSION_MIN}")
    
    if [[ "${version_check}" == "-1" ]]; then
        error "KinD version ${kind_version} is too old (minimum: ${KIND_VERSION_MIN})"
        return 1
    fi
    
    debug "KinD version ${kind_version} meets requirements"
    return 0
}

# Validate KinD cluster
kind_validate_cluster() {
    local cluster_name="$1"
    local context="kind-${cluster_name}"
    
    info "Validating KinD cluster: ${cluster_name}"
    
    # Check if cluster exists
    if ! kind get clusters 2>/dev/null | grep -q "^${cluster_name}$"; then
        error "KinD cluster not found: ${cluster_name}"
        return 1
    fi
    
    # Check kubectl access
    if ! kubectl cluster-info --context="${context}" >/dev/null 2>&1; then
        error "Cannot access KinD cluster: ${cluster_name}"
        return 1
    fi
    
    # Wait for all nodes to be ready
    if ! kubectl wait --context="${context}" --for=condition=Ready nodes --all --timeout=300s; then
        error "KinD cluster nodes not ready: ${cluster_name}"
        return 1
    fi
    
    # Wait for system pods to be ready
    if ! kubectl wait --context="${context}" --namespace=kube-system --for=condition=Ready pods --all --timeout=300s; then
        error "KinD cluster system pods not ready: ${cluster_name}"
        return 1
    fi
    
    # Check networking
    if ! validate_kind_networking "${cluster_name}"; then
        error "KinD cluster networking validation failed: ${cluster_name}"
        return 1
    fi
    
    info "KinD cluster validation passed: ${cluster_name}"
    return 0
}

# Delete KinD cluster
kind_delete_cluster() {
    local cluster_name="$1"
    
    info "Deleting KinD cluster: ${cluster_name}"
    
    if kind get clusters 2>/dev/null | grep -q "^${cluster_name}$"; then
        if ! kind delete cluster --name "${cluster_name}"; then
            error "Failed to delete KinD cluster: ${cluster_name}"
            return 1
        fi
        info "KinD cluster deleted: ${cluster_name}"
    else
        debug "KinD cluster does not exist: ${cluster_name}"
    fi
    
    return 0
}

# Create multiple KinD clusters
kind_create_multicluster() {
    local primary_cluster="$1"
    local remote_clusters=("${@:2}")
    
    info "Creating KinD multicluster setup"
    info "Primary cluster: ${primary_cluster}"
    info "Remote clusters: ${remote_clusters[*]}"
    
    # Create primary cluster
    if ! kind_create_cluster "${primary_cluster}"; then
        error "Failed to create primary KinD cluster: ${primary_cluster}"
        return 1
    fi
    
    # Create remote clusters
    local pids=()
    for remote_cluster in "${remote_clusters[@]}"; do
        (
            if ! kind_create_cluster "${remote_cluster}"; then
                error "Failed to create remote KinD cluster: ${remote_cluster}"
                exit 1
            fi
        ) &
        pids+=($!)
    done
    
    # Wait for all remote clusters
    local failed_clusters=()
    for i in "${!pids[@]}"; do
        local pid="${pids[i]}"
        local remote_cluster="${remote_clusters[i]}"
        
        if ! wait "${pid}"; then
            failed_clusters+=("${remote_cluster}")
        fi
    done
    
    if [[ ${#failed_clusters[@]} -gt 0 ]]; then
        error "Failed to create remote clusters: ${failed_clusters[*]}"
        return 1
    fi
    
    # Setup cross-cluster networking
    if ! kind_setup_cross_cluster_networking "${primary_cluster}" "${remote_clusters[@]}"; then
        error "Failed to setup cross-cluster networking"
        return 1
    fi
    
    info "KinD multicluster setup completed successfully"
    return 0
}

# Setup cross-cluster networking for KinD
kind_setup_cross_cluster_networking() {
    local primary_cluster="$1"
    local remote_clusters=("${@:2}")
    
    info "Setting up cross-cluster networking for KinD"
    
    # Get cluster networks
    local primary_network
    primary_network=$(docker network ls --filter name=kind --format "{{.Name}}" | grep "${primary_cluster}" | head -1)
    
    if [[ -z "${primary_network}" ]]; then
        error "Cannot find Docker network for primary cluster: ${primary_cluster}"
        return 1
    fi
    
    # Connect remote clusters to primary network
    for remote_cluster in "${remote_clusters[@]}"; do
        info "Connecting ${remote_cluster} to primary network: ${primary_network}"
        
        local remote_containers
        remote_containers=$(docker ps --filter name="${remote_cluster}" --format "{{.Names}}")
        
        for container in ${remote_containers}; do
            if ! docker network connect "${primary_network}" "${container}" 2>/dev/null; then
                debug "Container ${container} may already be connected to ${primary_network}"
            fi
        done
    done
    
    # Validate cross-cluster connectivity
    if ! kind_validate_cross_cluster_connectivity "${primary_cluster}" "${remote_clusters[@]}"; then
        error "Cross-cluster connectivity validation failed"
        return 1
    fi
    
    info "Cross-cluster networking setup completed"
    return 0
}

# Validate cross-cluster connectivity
kind_validate_cross_cluster_connectivity() {
    local primary_cluster="$1"
    local remote_clusters=("${@:2}")
    
    info "Validating cross-cluster connectivity"
    
    local primary_context="kind-${primary_cluster}"
    
    # Get primary cluster API server endpoint
    local primary_endpoint
    primary_endpoint=$(kubectl config view --context="${primary_context}" -o jsonpath='{.clusters[0].cluster.server}')
    
    # Test connectivity from each remote cluster to primary
    for remote_cluster in "${remote_clusters[@]}"; do
        local remote_context="kind-${remote_cluster}"
        
        info "Testing connectivity from ${remote_cluster} to ${primary_cluster}"
        
        # Create a test pod to check connectivity
        local test_pod="connectivity-test-$(date +%s)"
        kubectl --context="${remote_context}" run "${test_pod}" --image=curlimages/curl:latest --rm -i --restart=Never -- \
            sh -c "curl -k --connect-timeout 10 ${primary_endpoint}/healthz" >/dev/null 2>&1
        
        if [[ $? -eq 0 ]]; then
            debug "✓ Connectivity from ${remote_cluster} to ${primary_cluster}"
        else
            warn "✗ Connectivity from ${remote_cluster} to ${primary_cluster} failed"
        fi
    done
    
    return 0
}

# Get KinD cluster info
kind_get_cluster_info() {
    local cluster_name="$1"
    local context="kind-${cluster_name}"
    
    info "Getting KinD cluster info: ${cluster_name}"
    
    # Basic cluster info
    kubectl cluster-info --context="${context}"
    
    # Node info
    echo ""
    info "Cluster nodes:"
    kubectl get nodes --context="${context}" -o wide
    
    # Network info
    echo ""
    info "LoadBalancer services:"
    kubectl get svc --context="${context}" --all-namespaces -o wide | grep LoadBalancer || echo "No LoadBalancer services found"
    
    # MetalLB info
    echo ""
    info "MetalLB status:"
    kubectl get pods --context="${context}" -n metallb-system 2>/dev/null || echo "MetalLB not installed"
}

# Load Docker image into KinD cluster
kind_load_image() {
    local cluster_name="$1"
    local image_name="$2"
    
    info "Loading Docker image into KinD cluster: ${image_name} -> ${cluster_name}"
    
    if ! kind load docker-image "${image_name}" --name "${cluster_name}"; then
        error "Failed to load image into KinD cluster: ${image_name}"
        return 1
    fi
    
    debug "Image loaded successfully: ${image_name}"
    return 0
}

# Export KinD image from cluster
kind_export_image() {
    local cluster_name="$1"
    local image_name="$2"
    local output_file="$3"
    
    info "Exporting image from KinD cluster: ${image_name}"
    
    local context="kind-${cluster_name}"
    
    # Get image ID from cluster
    local image_id
    image_id=$(kubectl --context="${context}" get nodes -o jsonpath='{.items[0].status.images[?(@.names[0]=="'${image_name}'")].imageID}' 2>/dev/null)
    
    if [[ -z "${image_id}" ]]; then
        error "Image not found in cluster: ${image_name}"
        return 1
    fi
    
    # Export via docker
    if ! docker save "${image_name}" -o "${output_file}"; then
        error "Failed to export image: ${image_name}"
        return 1
    fi
    
    info "Image exported to: ${output_file}"
    return 0
}

# Cleanup all KinD clusters with prefix
kind_cleanup_all() {
    local prefix="${1:-${KIND_CLUSTER_PREFIX}}"
    
    info "Cleaning up all KinD clusters with prefix: ${prefix}"
    
    local clusters
    clusters=$(kind get clusters 2>/dev/null | grep "^${prefix}" || echo "")
    
    if [[ -z "${clusters}" ]]; then
        info "No KinD clusters found with prefix: ${prefix}"
        return 0
    fi
    
    local cleanup_pids=()
    for cluster in ${clusters}; do
        (
            kind_delete_cluster "${cluster}"
        ) &
        cleanup_pids+=($!)
    done
    
    # Wait for all cleanup operations
    for pid in "${cleanup_pids[@]}"; do
        wait "${pid}" || warn "Cleanup operation failed for PID: ${pid}"
    done
    
    info "KinD cluster cleanup completed"
    return 0
}

# Export KinD provider functions
export -f kind_create_cluster generate_kind_config kind_validate_version
export -f kind_validate_cluster kind_delete_cluster kind_create_multicluster
export -f kind_setup_cross_cluster_networking kind_validate_cross_cluster_connectivity
export -f kind_get_cluster_info kind_load_image kind_export_image kind_cleanup_all
