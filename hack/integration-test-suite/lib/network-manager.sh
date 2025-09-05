#!/bin/bash
#
# Network Manager for Kiali Integration Test Suite
# Handles networking differences between KinD and Minikube cluster types
#

# Source utilities if not already sourced
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/utils.sh"
fi

# Network configuration defaults
readonly DEFAULT_METALLB_IP_RANGE="255.70-255.84"
readonly DEFAULT_NODEPORT_RANGE="30000-32767"
readonly DEFAULT_LOAD_BALANCER_TIMEOUT="300s"
readonly DEFAULT_SERVICE_TIMEOUT="120s"

# Global variables for network state
declare -A SERVICE_URLS
declare -A ALLOCATED_IP_RANGES
declare -A PORT_FORWARDS

# Get service URL based on cluster type and configuration
get_service_url() {
    local service_name="$1"
    local namespace="$2"
    local port_name="${3:-http}"
    local cluster_type="$4"
    local context="${5:-}"
    
    debug "Getting service URL: ${service_name}.${namespace}:${port_name} (${cluster_type})"
    
    # Check if URL is already cached
    local cache_key="${cluster_type}:${context}:${namespace}:${service_name}:${port_name}"
    if [[ -n "${SERVICE_URLS[${cache_key}]:-}" ]]; then
        echo "${SERVICE_URLS[${cache_key}]}"
        return 0
    fi
    
    local url=""
    case "${cluster_type}" in
        "kind")
            url=$(kind_get_service_url "${service_name}" "${namespace}" "${port_name}" "${context}")
            ;;
        "minikube")
            url=$(minikube_get_service_url "${service_name}" "${namespace}" "${port_name}" "${context}")
            ;;
        *)
            error "Unsupported cluster type: ${cluster_type}"
            return 1
            ;;
    esac
    
    if [[ -n "${url}" ]]; then
        SERVICE_URLS[${cache_key}]="${url}"
        debug "Cached service URL: ${cache_key} -> ${url}"
        echo "${url}"
        return 0
    else
        error "Failed to get service URL for ${service_name}.${namespace}"
        return 1
    fi
}

# Get service URL for KinD clusters (uses LoadBalancer with MetalLB)
kind_get_service_url() {
    local service_name="$1"
    local namespace="$2"
    local port_name="$3"
    local context="${4:-}"
    
    local kubectl_cmd="kubectl"
    if [[ -n "${context}" ]]; then
        kubectl_cmd="${kubectl_cmd} --context=${context}"
    fi
    
    # Wait for LoadBalancer IP assignment
    info "Waiting for LoadBalancer IP for ${service_name}.${namespace}"
    
    local max_attempts=60
    local attempt=0
    local lb_ip=""
    
    while [[ ${attempt} -lt ${max_attempts} ]]; do
        lb_ip=$(${kubectl_cmd} get svc "${service_name}" -n "${namespace}" \
            -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")
        
        if [[ -n "${lb_ip}" && "${lb_ip}" != "null" ]]; then
            break
        fi
        
        sleep 5
        ((attempt++))
        
        if [[ $((attempt % 12)) -eq 0 ]]; then
            info "Still waiting for LoadBalancer IP (${attempt}/${max_attempts})"
        fi
    done
    
    if [[ -z "${lb_ip}" || "${lb_ip}" == "null" ]]; then
        error "LoadBalancer IP not assigned for ${service_name}.${namespace} after ${max_attempts} attempts"
        return 1
    fi
    
    # Get port number
    local port
    if [[ "${port_name}" == "https" ]]; then
        port="443"
    elif [[ "${port_name}" == "http" ]]; then
        port="80"
    else
        # Try to get the actual port number from the service
        port=$(${kubectl_cmd} get svc "${service_name}" -n "${namespace}" \
            -o jsonpath="{.spec.ports[?(@.name==\"${port_name}\")].port}" 2>/dev/null || echo "80")
        if [[ -z "${port}" || "${port}" == "null" ]]; then
            port="80"
        fi
    fi
    
    local protocol="http"
    if [[ "${port}" == "443" || "${port_name}" == "https" ]]; then
        protocol="https"
    fi
    
    echo "${protocol}://${lb_ip}:${port}"
}

# Get service URL for Minikube clusters (uses NodePort or LoadBalancer with tunnel)
minikube_get_service_url() {
    local service_name="$1"
    local namespace="$2"
    local port_name="$3"
    local context="${4:-}"
    
    local kubectl_cmd="kubectl"
    if [[ -n "${context}" ]]; then
        kubectl_cmd="${kubectl_cmd} --context=${context}"
    fi
    
    # Check service type
    local service_type
    service_type=$(${kubectl_cmd} get svc "${service_name}" -n "${namespace}" \
        -o jsonpath='{.spec.type}' 2>/dev/null || echo "ClusterIP")
    
    case "${service_type}" in
        "LoadBalancer")
            if minikube_tunnel_active "${context}"; then
                # Use LoadBalancer IP (similar to KinD)
                return kind_get_service_url "${service_name}" "${namespace}" "${port_name}" "${context}"
            else
                warn "LoadBalancer service detected but minikube tunnel not active"
                warn "Falling back to NodePort access"
                minikube_get_nodeport_url "${service_name}" "${namespace}" "${port_name}" "${context}"
            fi
            ;;
        "NodePort")
            minikube_get_nodeport_url "${service_name}" "${namespace}" "${port_name}" "${context}"
            ;;
        *)
            error "Unsupported service type for external access: ${service_type}"
            return 1
            ;;
    esac
}

# Get NodePort URL for Minikube
minikube_get_nodeport_url() {
    local service_name="$1"
    local namespace="$2"
    local port_name="$3"
    local context="${4:-}"
    
    local kubectl_cmd="kubectl"
    if [[ -n "${context}" ]]; then
        kubectl_cmd="${kubectl_cmd} --context=${context}"
    fi
    
    # Get NodePort
    local node_port
    node_port=$(${kubectl_cmd} get svc "${service_name}" -n "${namespace}" \
        -o jsonpath="{.spec.ports[?(@.name==\"${port_name}\")].nodePort}" 2>/dev/null)
    
    if [[ -z "${node_port}" || "${node_port}" == "null" ]]; then
        # Try to get the first port if name-based lookup failed
        node_port=$(${kubectl_cmd} get svc "${service_name}" -n "${namespace}" \
            -o jsonpath='{.spec.ports[0].nodePort}' 2>/dev/null)
    fi
    
    if [[ -z "${node_port}" || "${node_port}" == "null" ]]; then
        error "NodePort not found for ${service_name}.${namespace}:${port_name}"
        return 1
    fi
    
    # Get Minikube IP
    local minikube_profile
    minikube_profile=$(get_minikube_profile_from_context "${context}")
    local minikube_ip
    minikube_ip=$(minikube ip -p "${minikube_profile}" 2>/dev/null)
    
    if [[ -z "${minikube_ip}" ]]; then
        error "Cannot get Minikube IP for profile: ${minikube_profile}"
        return 1
    fi
    
    local protocol="http"
    if [[ "${port_name}" == "https" ]]; then
        protocol="https"
    fi
    
    echo "${protocol}://${minikube_ip}:${node_port}"
}

# Check if minikube tunnel is active
minikube_tunnel_active() {
    local context="${1:-}"
    local minikube_profile
    minikube_profile=$(get_minikube_profile_from_context "${context}")
    
    # Check if tunnel process is running for this profile
    if pgrep -f "minikube.*tunnel.*${minikube_profile}" >/dev/null 2>&1; then
        return 0
    fi
    
    # Alternative check: look for LoadBalancer services with external IPs
    local kubectl_cmd="kubectl"
    if [[ -n "${context}" ]]; then
        kubectl_cmd="${kubectl_cmd} --context=${context}"
    fi
    
    local lb_services
    lb_services=$(${kubectl_cmd} get svc --all-namespaces -o jsonpath='{.items[?(@.spec.type=="LoadBalancer")].status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")
    
    if [[ -n "${lb_services}" && "${lb_services}" != "null" ]]; then
        return 0
    fi
    
    return 1
}

# Get Minikube profile from context name
get_minikube_profile_from_context() {
    local context="${1:-}"
    
    if [[ -z "${context}" ]]; then
        echo "${MINIKUBE_PROFILE:-ci}"
        return 0
    fi
    
    # Extract profile from context name (e.g., "minikube-ci" -> "ci")
    if [[ "${context}" =~ ^minikube-(.+)$ ]]; then
        echo "${BASH_REMATCH[1]}"
    else
        echo "${MINIKUBE_PROFILE:-ci}"
    fi
}

# Setup networking for cluster type
setup_cluster_networking() {
    local cluster_type="$1"
    local cluster_name="$2"
    local config_file="${3:-}"
    
    info "Setting up networking for ${cluster_type} cluster: ${cluster_name}"
    
    case "${cluster_type}" in
        "kind")
            setup_kind_networking "${cluster_name}" "${config_file}"
            ;;
        "minikube")
            setup_minikube_networking "${cluster_name}" "${config_file}"
            ;;
        *)
            error "Unsupported cluster type: ${cluster_type}"
            return 1
            ;;
    esac
}

# Setup networking for KinD cluster
setup_kind_networking() {
    local cluster_name="$1"
    local config_file="${2:-}"
    
    info "Setting up KinD networking for cluster: ${cluster_name}"
    
    # Install MetalLB
    install_metallb_kind "${cluster_name}"
    
    # Configure MetalLB IP range
    local ip_range
    if [[ -n "${config_file}" ]]; then
        ip_range=$(get_yaml_value "${config_file}" ".network.metallb_range" "${DEFAULT_METALLB_IP_RANGE}")
    else
        ip_range=$(allocate_metallb_ip_range "${cluster_name}")
    fi
    
    configure_metallb_ip_range "${cluster_name}" "${ip_range}"
    
    # Validate networking
    validate_kind_networking "${cluster_name}"
    
    info "KinD networking setup completed for cluster: ${cluster_name}"
}

# Install MetalLB for KinD cluster
install_metallb_kind() {
    local cluster_name="$1"
    local context="kind-${cluster_name}"
    
    info "Installing MetalLB for KinD cluster: ${cluster_name}"
    
    # Apply MetalLB manifests
    kubectl --context="${context}" apply -f https://raw.githubusercontent.com/metallb/metallb/v0.13.12/config/manifests/metallb-native.yaml
    
    # Wait for MetalLB to be ready
    info "Waiting for MetalLB to be ready"
    kubectl --context="${context}" wait --namespace metallb-system \
        --for=condition=ready pod \
        --selector=app=metallb \
        --timeout=90s
    
    debug "MetalLB installation completed"
}

# Configure MetalLB IP range
configure_metallb_ip_range() {
    local cluster_name="$1"
    local ip_range="$2"
    local context="kind-${cluster_name}"
    
    info "Configuring MetalLB IP range: ${ip_range}"
    
    # Create IPAddressPool
    kubectl --context="${context}" apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: default-pool
  namespace: metallb-system
spec:
  addresses:
  - ${ip_range}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: default-advertisement
  namespace: metallb-system
spec:
  ipAddressPools:
  - default-pool
EOF
    
    # Store the allocated range
    ALLOCATED_IP_RANGES[${cluster_name}]="${ip_range}"
    
    debug "MetalLB IP range configured: ${ip_range}"
}

# Allocate MetalLB IP range for cluster
allocate_metallb_ip_range() {
    local cluster_name="$1"
    local base_ip="255"
    
    case "${cluster_name}" in
        *primary*|*cluster1*)
            echo "${base_ip}.70-${base_ip}.84"
            ;;
        *remote*|*cluster2*)
            echo "${base_ip}.85-${base_ip}.99"
            ;;
        *cluster3*)
            echo "${base_ip}.100-${base_ip}.114"
            ;;
        *)
            echo "${base_ip}.115-${base_ip}.129"
            ;;
    esac
}

# Setup networking for Minikube cluster
setup_minikube_networking() {
    local cluster_name="$1"
    local config_file="${2:-}"
    
    info "Setting up Minikube networking for cluster: ${cluster_name}"
    
    local profile
    profile=$(get_minikube_profile_from_context "minikube-${cluster_name}")
    
    # Enable ingress addon
    minikube addons enable ingress -p "${profile}"
    
    # Optionally enable MetalLB
    local enable_metallb="false"
    if [[ -n "${config_file}" ]]; then
        enable_metallb=$(get_yaml_value "${config_file}" ".network.enable_metallb" "false")
    fi
    
    if [[ "${enable_metallb}" == "true" ]]; then
        setup_minikube_metallb "${profile}"
    fi
    
    # Validate networking
    validate_minikube_networking "${profile}"
    
    info "Minikube networking setup completed for cluster: ${cluster_name}"
}

# Setup MetalLB for Minikube
setup_minikube_metallb() {
    local profile="$1"
    
    info "Setting up MetalLB for Minikube profile: ${profile}"
    
    # Enable MetalLB addon
    minikube addons enable metallb -p "${profile}"
    
    # Get Minikube IP range for MetalLB
    local minikube_ip
    minikube_ip=$(minikube ip -p "${profile}")
    local ip_base
    ip_base=$(echo "${minikube_ip}" | cut -d. -f1-3)
    local lb_range="${ip_base}.200-${ip_base}.250"
    
    info "Configuring MetalLB with IP range: ${lb_range}"
    
    # Configure MetalLB
    kubectl --context="minikube-${profile}" apply -f - <<EOF
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: default-pool
  namespace: metallb-system
spec:
  addresses:
  - ${lb_range}
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: default-advertisement
  namespace: metallb-system
spec:
  ipAddressPools:
  - default-pool
EOF
    
    debug "MetalLB setup completed for Minikube profile: ${profile}"
}

# Validate KinD networking
validate_kind_networking() {
    local cluster_name="$1"
    local context="kind-${cluster_name}"
    
    info "Validating KinD networking for cluster: ${cluster_name}"
    
    # Check MetalLB pods are running
    if ! kubectl --context="${context}" get pods -n metallb-system -l app=metallb --no-headers | grep -q "Running"; then
        error "MetalLB pods are not running"
        return 1
    fi
    
    # Check IPAddressPool exists
    if ! kubectl --context="${context}" get ipaddresspool -n metallb-system default-pool >/dev/null 2>&1; then
        error "MetalLB IPAddressPool not found"
        return 1
    fi
    
    debug "KinD networking validation passed"
}

# Validate Minikube networking
validate_minikube_networking() {
    local profile="$1"
    local context="minikube-${profile}"
    
    info "Validating Minikube networking for profile: ${profile}"
    
    # Check ingress addon is enabled
    if ! minikube addons list -p "${profile}" | grep -q "ingress.*enabled"; then
        warn "Ingress addon is not enabled"
    fi
    
    # Check if MetalLB is enabled and working
    if minikube addons list -p "${profile}" | grep -q "metallb.*enabled"; then
        if ! kubectl --context="${context}" get pods -n metallb-system -l app=metallb --no-headers | grep -q "Running"; then
            warn "MetalLB addon is enabled but pods are not running"
        else
            debug "MetalLB addon is working"
        fi
    fi
    
    debug "Minikube networking validation passed"
}

# Start port forwarding
start_port_forward() {
    local service_name="$1"
    local namespace="$2"
    local local_port="$3"
    local remote_port="${4:-80}"
    local context="${5:-}"
    
    local kubectl_cmd="kubectl"
    if [[ -n "${context}" ]]; then
        kubectl_cmd="${kubectl_cmd} --context=${context}"
    fi
    
    info "Starting port forward: ${service_name}.${namespace} ${local_port}:${remote_port}"
    
    # Kill any existing port forward on the same local port
    stop_port_forward "${local_port}"
    
    # Start port forward in background
    ${kubectl_cmd} port-forward -n "${namespace}" "svc/${service_name}" "${local_port}:${remote_port}" >/dev/null 2>&1 &
    local pid=$!
    
    # Store the PID for cleanup
    PORT_FORWARDS[${local_port}]="${pid}"
    
    # Wait for port forward to be ready
    local max_attempts=30
    local attempt=0
    
    while [[ ${attempt} -lt ${max_attempts} ]]; do
        if is_url_accessible "http://localhost:${local_port}" 2; then
            info "Port forward ready: localhost:${local_port}"
            return 0
        fi
        sleep 1
        ((attempt++))
    done
    
    error "Port forward failed to become ready: ${local_port}"
    stop_port_forward "${local_port}"
    return 1
}

# Stop port forwarding
stop_port_forward() {
    local local_port="$1"
    
    if [[ -n "${PORT_FORWARDS[${local_port}]:-}" ]]; then
        local pid="${PORT_FORWARDS[${local_port}]}"
        debug "Stopping port forward on port ${local_port} (PID: ${pid})"
        kill_process_tree "${pid}"
        unset PORT_FORWARDS[${local_port}]
    fi
    
    # Also kill any processes using the port
    local port_processes
    port_processes=$(lsof -ti:${local_port} 2>/dev/null || echo "")
    for pid in ${port_processes}; do
        debug "Killing process using port ${local_port}: ${pid}"
        kill -TERM "${pid}" 2>/dev/null || true
    done
}

# Cleanup all port forwards
cleanup_port_forwards() {
    info "Cleaning up port forwards"
    for local_port in "${!PORT_FORWARDS[@]}"; do
        stop_port_forward "${local_port}"
    done
}

# Detect optimal service type for cluster
detect_optimal_service_type() {
    local cluster_type="$1"
    local component="${2:-default}"
    
    case "${cluster_type}" in
        "kind")
            echo "LoadBalancer"
            ;;
        "minikube")
            # Check if MetalLB is available and tunnel can be used
            if command -v minikube >/dev/null 2>&1; then
                local profile="${MINIKUBE_PROFILE:-ci}"
                if minikube addons list -p "${profile}" 2>/dev/null | grep -q "metallb.*enabled"; then
                    echo "LoadBalancer"
                else
                    echo "NodePort"
                fi
            else
                echo "NodePort"
            fi
            ;;
        *)
            echo "ClusterIP"
            ;;
    esac
}

# Wait for service to be ready
wait_for_service_ready() {
    local service_name="$1"
    local namespace="$2"
    local cluster_type="$3"
    local context="${4:-}"
    local timeout="${5:-${DEFAULT_SERVICE_TIMEOUT}}"
    
    info "Waiting for service to be ready: ${service_name}.${namespace}"
    
    local kubectl_cmd="kubectl"
    if [[ -n "${context}" ]]; then
        kubectl_cmd="${kubectl_cmd} --context=${context}"
    fi
    
    # First wait for service to exist
    if ! wait_for_condition \
        "service ${service_name}.${namespace} to exist" \
        "${kubectl_cmd} get svc ${service_name} -n ${namespace} >/dev/null 2>&1" \
        120 5; then
        return 1
    fi
    
    # Then wait based on service type
    local service_type
    service_type=$(${kubectl_cmd} get svc "${service_name}" -n "${namespace}" -o jsonpath='{.spec.type}')
    
    case "${service_type}" in
        "LoadBalancer")
            wait_for_condition \
                "LoadBalancer IP for ${service_name}.${namespace}" \
                "test -n \"\$(${kubectl_cmd} get svc ${service_name} -n ${namespace} -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null)\"" \
                "${timeout%s}" 10
            ;;
        "NodePort")
            wait_for_condition \
                "NodePort for ${service_name}.${namespace}" \
                "test -n \"\$(${kubectl_cmd} get svc ${service_name} -n ${namespace} -o jsonpath='{.spec.ports[0].nodePort}' 2>/dev/null)\"" \
                30 5
            ;;
        *)
            debug "Service type ${service_type} doesn't require external access wait"
            return 0
            ;;
    esac
}

# Add cleanup for port forwards
add_cleanup_function cleanup_port_forwards

# Export network manager functions
export -f get_service_url kind_get_service_url minikube_get_service_url
export -f minikube_get_nodeport_url minikube_tunnel_active get_minikube_profile_from_context
export -f setup_cluster_networking setup_kind_networking setup_minikube_networking
export -f install_metallb_kind configure_metallb_ip_range allocate_metallb_ip_range
export -f setup_minikube_metallb validate_kind_networking validate_minikube_networking
export -f start_port_forward stop_port_forward cleanup_port_forwards
export -f detect_optimal_service_type wait_for_service_ready
