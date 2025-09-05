#!/bin/bash
#
# Minikube Provider for Kiali Integration Test Suite
# Handles Minikube cluster creation, configuration, and management
#

# Source required libraries
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils.sh"
fi

if ! declare -f setup_cluster_networking >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/network-manager.sh"
fi

# Minikube configuration defaults
readonly MINIKUBE_VERSION_MIN="1.25.0"
readonly MINIKUBE_DEFAULT_PROFILE="ci"
readonly MINIKUBE_DEFAULT_DRIVER="docker"
readonly MINIKUBE_DEFAULT_K8S_VERSION="v1.28.0"
readonly MINIKUBE_DEFAULT_MEMORY="4096"
readonly MINIKUBE_DEFAULT_CPUS="2"
readonly MINIKUBE_DEFAULT_DISK="20g"

# Create Minikube cluster
minikube_create_cluster() {
    local cluster_name="$1"
    local config_file="${2:-}"
    
    info "Creating Minikube cluster: ${cluster_name}"
    
    # Validate Minikube version
    if ! minikube_validate_version; then
        return 1
    fi
    
    # Get profile name
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    
    # Get configuration values
    local driver="${MINIKUBE_DEFAULT_DRIVER}"
    local k8s_version="${MINIKUBE_DEFAULT_K8S_VERSION}"
    local memory="${MINIKUBE_DEFAULT_MEMORY}"
    local cpus="${MINIKUBE_DEFAULT_CPUS}"
    local disk_size="${MINIKUBE_DEFAULT_DISK}"
    
    if [[ -n "${config_file}" && -f "${config_file}" ]]; then
        driver=$(get_yaml_value "${config_file}" ".minikube.driver" "${driver}")
        k8s_version=$(get_yaml_value "${config_file}" ".kubernetes.version" "${k8s_version}")
        memory=$(get_yaml_value "${config_file}" ".minikube.memory" "${memory}")
        cpus=$(get_yaml_value "${config_file}" ".minikube.cpus" "${cpus}")
        disk_size=$(get_yaml_value "${config_file}" ".minikube.disk_size" "${disk_size}")
    fi
    
    # Delete existing profile if it exists
    if minikube profile list -o json 2>/dev/null | jq -r '.valid[].Name' 2>/dev/null | grep -q "^${profile}$"; then
        warn "Minikube profile ${profile} already exists, deleting it"
        minikube_delete_cluster "${cluster_name}"
    fi
    
    # Create the cluster
    info "Starting Minikube with profile: ${profile}"
    if ! minikube start \
        --profile="${profile}" \
        --driver="${driver}" \
        --kubernetes-version="${k8s_version}" \
        --memory="${memory}" \
        --cpus="${cpus}" \
        --disk-size="${disk_size}" \
        --wait=300s; then
        error "Failed to create Minikube cluster: ${cluster_name}"
        return 1
    fi
    
    # Setup networking
    if ! setup_cluster_networking "minikube" "${cluster_name}" "${config_file}"; then
        error "Failed to setup networking for Minikube cluster: ${cluster_name}"
        minikube_delete_cluster "${cluster_name}"
        return 1
    fi
    
    # Validate cluster is ready
    if ! minikube_validate_cluster "${cluster_name}"; then
        error "Minikube cluster validation failed: ${cluster_name}"
        minikube_delete_cluster "${cluster_name}"
        return 1
    fi
    
    info "Minikube cluster created successfully: ${cluster_name}"
    return 0
}

# Get Minikube profile name from cluster name
get_minikube_profile_name() {
    local cluster_name="$1"
    local prefix="${MINIKUBE_PROFILE_PREFIX:-ci}"
    
    if [[ "${cluster_name}" == *"${prefix}"* ]]; then
        echo "${cluster_name}"
    else
        echo "${prefix}-${cluster_name}"
    fi
}

# Validate Minikube version
minikube_validate_version() {
    if ! command -v minikube >/dev/null 2>&1; then
        error "Minikube not found in PATH"
        return 1
    fi
    
    local minikube_version
    minikube_version=$(minikube version --short 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | sed 's/v//')
    
    if [[ -z "${minikube_version}" ]]; then
        error "Cannot determine Minikube version"
        return 1
    fi
    
    local version_check
    version_check=$(version_compare "${minikube_version}" "${MINIKUBE_VERSION_MIN}")
    
    if [[ "${version_check}" == "-1" ]]; then
        error "Minikube version ${minikube_version} is too old (minimum: ${MINIKUBE_VERSION_MIN})"
        return 1
    fi
    
    debug "Minikube version ${minikube_version} meets requirements"
    return 0
}

# Validate Minikube cluster
minikube_validate_cluster() {
    local cluster_name="$1"
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    local context="minikube-${profile}"
    
    info "Validating Minikube cluster: ${cluster_name} (profile: ${profile})"
    
    # Check if profile exists
    if ! minikube profile list -o json 2>/dev/null | jq -r '.valid[].Name' 2>/dev/null | grep -q "^${profile}$"; then
        error "Minikube profile not found: ${profile}"
        return 1
    fi
    
    # Check cluster status
    local status
    status=$(minikube status -p "${profile}" --format="{{.Host}}" 2>/dev/null || echo "")
    if [[ "${status}" != "Running" ]]; then
        error "Minikube cluster not running: ${cluster_name} (status: ${status})"
        return 1
    fi
    
    # Check kubectl access
    if ! kubectl cluster-info --context="${context}" >/dev/null 2>&1; then
        error "Cannot access Minikube cluster: ${cluster_name}"
        return 1
    fi
    
    # Wait for all nodes to be ready
    if ! kubectl wait --context="${context}" --for=condition=Ready nodes --all --timeout=300s; then
        error "Minikube cluster nodes not ready: ${cluster_name}"
        return 1
    fi
    
    # Wait for system pods to be ready
    if ! kubectl wait --context="${context}" --namespace=kube-system --for=condition=Ready pods --all --timeout=300s; then
        error "Minikube cluster system pods not ready: ${cluster_name}"
        return 1
    fi
    
    # Check networking
    if ! validate_minikube_networking "${profile}"; then
        error "Minikube cluster networking validation failed: ${cluster_name}"
        return 1
    fi
    
    info "Minikube cluster validation passed: ${cluster_name}"
    return 0
}

# Delete Minikube cluster
minikube_delete_cluster() {
    local cluster_name="$1"
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    
    info "Deleting Minikube cluster: ${cluster_name} (profile: ${profile})"
    
    if minikube profile list -o json 2>/dev/null | jq -r '.valid[].Name' 2>/dev/null | grep -q "^${profile}$"; then
        if ! minikube delete --profile="${profile}"; then
            error "Failed to delete Minikube cluster: ${cluster_name}"
            return 1
        fi
        info "Minikube cluster deleted: ${cluster_name}"
    else
        debug "Minikube cluster does not exist: ${cluster_name}"
    fi
    
    return 0
}

# Create multiple Minikube clusters
minikube_create_multicluster() {
    local primary_cluster="$1"
    local remote_clusters=("${@:2}")
    
    info "Creating Minikube multicluster setup"
    info "Primary cluster: ${primary_cluster}"
    info "Remote clusters: ${remote_clusters[*]}"
    
    # Create primary cluster
    if ! minikube_create_cluster "${primary_cluster}"; then
        error "Failed to create primary Minikube cluster: ${primary_cluster}"
        return 1
    fi
    
    # Create remote clusters in parallel
    local pids=()
    for remote_cluster in "${remote_clusters[@]}"; do
        (
            if ! minikube_create_cluster "${remote_cluster}"; then
                error "Failed to create remote Minikube cluster: ${remote_cluster}"
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
    if ! minikube_setup_cross_cluster_networking "${primary_cluster}" "${remote_clusters[@]}"; then
        error "Failed to setup cross-cluster networking"
        return 1
    fi
    
    info "Minikube multicluster setup completed successfully"
    return 0
}

# Setup cross-cluster networking for Minikube
minikube_setup_cross_cluster_networking() {
    local primary_cluster="$1"
    local remote_clusters=("${@:2}")
    
    info "Setting up cross-cluster networking for Minikube"
    
    # For Minikube, cross-cluster networking is more complex
    # Each profile runs in its own network namespace
    # We need to configure network routing or use LoadBalancer services
    
    local primary_profile
    primary_profile=$(get_minikube_profile_name "${primary_cluster}")
    
    # Get primary cluster network info
    local primary_ip
    primary_ip=$(minikube ip -p "${primary_profile}")
    
    info "Primary cluster IP: ${primary_ip}"
    
    # For each remote cluster, configure access to primary
    for remote_cluster in "${remote_clusters[@]}"; do
        local remote_profile
        remote_profile=$(get_minikube_profile_name "${remote_cluster}")
        
        local remote_ip
        remote_ip=$(minikube ip -p "${remote_profile}")
        
        info "Remote cluster ${remote_cluster} IP: ${remote_ip}"
        
        # Test basic connectivity
        if ! minikube_test_cluster_connectivity "${primary_profile}" "${remote_profile}"; then
            warn "Limited connectivity between ${primary_cluster} and ${remote_cluster}"
        fi
    done
    
    info "Cross-cluster networking setup completed (limited by Minikube architecture)"
    return 0
}

# Test connectivity between Minikube clusters
minikube_test_cluster_connectivity() {
    local primary_profile="$1"
    local remote_profile="$2"
    
    local primary_ip
    primary_ip=$(minikube ip -p "${primary_profile}")
    local remote_ip
    remote_ip=$(minikube ip -p "${remote_profile}")
    
    # Test if remote cluster can reach primary cluster IP
    local remote_context="minikube-${remote_profile}"
    
    # Create a test pod to check connectivity
    local test_pod="connectivity-test-$(date +%s)"
    if kubectl --context="${remote_context}" run "${test_pod}" --image=curlimages/curl:latest --rm -i --restart=Never --timeout=30s -- \
        sh -c "curl --connect-timeout 5 http://${primary_ip}:8080/healthz" >/dev/null 2>&1; then
        debug "✓ Connectivity from ${remote_profile} to ${primary_profile}"
        return 0
    else
        debug "✗ Limited connectivity from ${remote_profile} to ${primary_profile}"
        return 1
    fi
}

# Get Minikube cluster info
minikube_get_cluster_info() {
    local cluster_name="$1"
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    local context="minikube-${profile}"
    
    info "Getting Minikube cluster info: ${cluster_name} (profile: ${profile})"
    
    # Basic cluster info
    minikube status -p "${profile}"
    
    # Cluster IP
    local cluster_ip
    cluster_ip=$(minikube ip -p "${profile}")
    echo "Cluster IP: ${cluster_ip}"
    
    # Enabled addons
    echo ""
    info "Enabled addons:"
    minikube addons list -p "${profile}" | grep enabled || echo "No addons enabled"
    
    # Node info
    echo ""
    info "Cluster nodes:"
    kubectl get nodes --context="${context}" -o wide
    
    # Service info
    echo ""
    info "Services:"
    kubectl get svc --context="${context}" --all-namespaces -o wide
}

# Enable Minikube addon
minikube_enable_addon() {
    local cluster_name="$1"
    local addon_name="$2"
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    
    info "Enabling addon ${addon_name} for cluster ${cluster_name}"
    
    if ! minikube addons enable "${addon_name}" -p "${profile}"; then
        error "Failed to enable addon: ${addon_name}"
        return 1
    fi
    
    # Wait for addon to be ready
    case "${addon_name}" in
        "ingress")
            # Wait for ingress controller
            local context="minikube-${profile}"
            kubectl wait --context="${context}" --namespace ingress-nginx \
                --for=condition=ready pod \
                --selector=app.kubernetes.io/component=controller \
                --timeout=300s
            ;;
        "metallb")
            # Wait for MetalLB
            local context="minikube-${profile}"
            kubectl wait --context="${context}" --namespace metallb-system \
                --for=condition=ready pod \
                --selector=app=metallb \
                --timeout=300s
            ;;
    esac
    
    info "Addon ${addon_name} enabled and ready"
    return 0
}

# Disable Minikube addon
minikube_disable_addon() {
    local cluster_name="$1"
    local addon_name="$2"
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    
    info "Disabling addon ${addon_name} for cluster ${cluster_name}"
    
    if ! minikube addons disable "${addon_name}" -p "${profile}"; then
        error "Failed to disable addon: ${addon_name}"
        return 1
    fi
    
    info "Addon ${addon_name} disabled"
    return 0
}

# Start Minikube tunnel (for LoadBalancer services)
minikube_start_tunnel() {
    local cluster_name="$1"
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    
    info "Starting Minikube tunnel for cluster: ${cluster_name}"
    
    # Check if tunnel is already running
    if pgrep -f "minikube.*tunnel.*${profile}" >/dev/null 2>&1; then
        info "Minikube tunnel already running for profile: ${profile}"
        return 0
    fi
    
    # Start tunnel in background
    local tunnel_pid
    tunnel_pid=$(start_background_process "minikube-tunnel-${profile}" "minikube tunnel -p ${profile} --cleanup")
    
    # Wait for tunnel to be ready
    local max_attempts=30
    local attempt=0
    
    while [[ ${attempt} -lt ${max_attempts} ]]; do
        if minikube_tunnel_active "minikube-${profile}"; then
            info "Minikube tunnel is ready for profile: ${profile}"
            return 0
        fi
        sleep 2
        ((attempt++))
    done
    
    error "Minikube tunnel failed to start for profile: ${profile}"
    return 1
}

# Stop Minikube tunnel
minikube_stop_tunnel() {
    local cluster_name="$1"
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    
    info "Stopping Minikube tunnel for cluster: ${cluster_name}"
    
    # Find and kill tunnel processes
    local tunnel_pids
    tunnel_pids=$(pgrep -f "minikube.*tunnel.*${profile}" || echo "")
    
    for pid in ${tunnel_pids}; do
        debug "Stopping tunnel process: ${pid}"
        kill_process_tree "${pid}"
    done
    
    info "Minikube tunnel stopped for profile: ${profile}"
    return 0
}

# Load Docker image into Minikube cluster
minikube_load_image() {
    local cluster_name="$1"
    local image_name="$2"
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    
    info "Loading Docker image into Minikube cluster: ${image_name} -> ${cluster_name}"
    
    if ! minikube image load "${image_name}" -p "${profile}"; then
        error "Failed to load image into Minikube cluster: ${image_name}"
        return 1
    fi
    
    debug "Image loaded successfully: ${image_name}"
    return 0
}

# Cleanup all Minikube profiles with prefix
minikube_cleanup_all() {
    local prefix="${1:-ci}"
    
    info "Cleaning up all Minikube profiles with prefix: ${prefix}"
    
    local profiles
    profiles=$(minikube profile list -o json 2>/dev/null | jq -r '.valid[].Name' 2>/dev/null | grep "^${prefix}" || echo "")
    
    if [[ -z "${profiles}" ]]; then
        info "No Minikube profiles found with prefix: ${prefix}"
        return 0
    fi
    
    local cleanup_pids=()
    for profile in ${profiles}; do
        (
            # Extract cluster name from profile
            local cluster_name="${profile#${prefix}-}"
            if [[ "${cluster_name}" == "${profile}" ]]; then
                cluster_name="${profile}"
            fi
            minikube_delete_cluster "${cluster_name}"
        ) &
        cleanup_pids+=($!)
    done
    
    # Wait for all cleanup operations
    for pid in "${cleanup_pids[@]}"; do
        wait "${pid}" || warn "Cleanup operation failed for PID: ${pid}"
    done
    
    info "Minikube cluster cleanup completed"
    return 0
}

# Get service URL using minikube service command
minikube_service_url() {
    local cluster_name="$1"
    local service_name="$2"
    local namespace="${3:-default}"
    local profile
    profile=$(get_minikube_profile_name "${cluster_name}")
    
    local service_url
    service_url=$(minikube service "${service_name}" --namespace="${namespace}" --url -p "${profile}" 2>/dev/null | head -1)
    
    if [[ -n "${service_url}" ]]; then
        echo "${service_url}"
        return 0
    else
        error "Failed to get service URL: ${service_name}.${namespace}"
        return 1
    fi
}

# Export Minikube provider functions
export -f minikube_create_cluster get_minikube_profile_name minikube_validate_version
export -f minikube_validate_cluster minikube_delete_cluster minikube_create_multicluster
export -f minikube_setup_cross_cluster_networking minikube_test_cluster_connectivity
export -f minikube_get_cluster_info minikube_enable_addon minikube_disable_addon
export -f minikube_start_tunnel minikube_stop_tunnel minikube_load_image
export -f minikube_cleanup_all minikube_service_url
