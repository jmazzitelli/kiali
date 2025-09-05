#!/bin/bash
#
# Debug Collector for Kiali Integration Test Suite
# Collects comprehensive debug information when tests fail
#

# Source required libraries
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/utils.sh"
fi

# Debug collection configuration
readonly DEBUG_TIMEOUT="30s"

# Collect cluster information
collect_cluster_info() {
    local cluster_type="$1"
    local debug_dir="$2"
    
    info "Collecting cluster information"
    
    local cluster_info_dir="${debug_dir}/cluster-info"
    ensure_directory "${cluster_info_dir}"
    
    # Collect info for each created cluster
    for cluster_name in "${!CREATED_CLUSTERS[@]}"; do
        local cluster_debug_dir="${cluster_info_dir}/${cluster_name}"
        ensure_directory "${cluster_debug_dir}"
        
        local context
        context=$(get_cluster_context "${cluster_name}" "${cluster_type}")
        
        info "Collecting info for cluster: ${cluster_name} (${context})"
        
        # Basic cluster info
        kubectl --context="${context}" cluster-info > "${cluster_debug_dir}/cluster-info.txt" 2>&1 || true
        
        # Node information
        kubectl --context="${context}" get nodes -o wide > "${cluster_debug_dir}/nodes.txt" 2>&1 || true
        kubectl --context="${context}" describe nodes > "${cluster_debug_dir}/nodes-describe.txt" 2>&1 || true
        
        # Pod information
        kubectl --context="${context}" get pods --all-namespaces -o wide > "${cluster_debug_dir}/pods.txt" 2>&1 || true
        kubectl --context="${context}" get pods --all-namespaces -o yaml > "${cluster_debug_dir}/pods.yaml" 2>&1 || true
        
        # Service information
        kubectl --context="${context}" get svc --all-namespaces -o wide > "${cluster_debug_dir}/services.txt" 2>&1 || true
        kubectl --context="${context}" get svc --all-namespaces -o yaml > "${cluster_debug_dir}/services.yaml" 2>&1 || true
        
        # Deployment information
        kubectl --context="${context}" get deployments --all-namespaces -o wide > "${cluster_debug_dir}/deployments.txt" 2>&1 || true
        kubectl --context="${context}" describe deployments --all-namespaces > "${cluster_debug_dir}/deployments-describe.txt" 2>&1 || true
        
        # Events
        kubectl --context="${context}" get events --all-namespaces --sort-by='.lastTimestamp' > "${cluster_debug_dir}/events.txt" 2>&1 || true
        
        # Resource usage
        kubectl --context="${context}" top nodes > "${cluster_debug_dir}/top-nodes.txt" 2>&1 || true
        kubectl --context="${context}" top pods --all-namespaces > "${cluster_debug_dir}/top-pods.txt" 2>&1 || true
    done
    
    debug "Cluster information collection completed"
}

# Collect service logs
collect_service_logs() {
    local cluster_type="$1"
    local debug_dir="$2"
    
    info "Collecting service logs"
    
    local logs_dir="${debug_dir}/logs"
    ensure_directory "${logs_dir}"
    
    # Collect logs for each created cluster
    for cluster_name in "${!CREATED_CLUSTERS[@]}"; do
        local cluster_logs_dir="${logs_dir}/${cluster_name}"
        ensure_directory "${cluster_logs_dir}"
        
        local context
        context=$(get_cluster_context "${cluster_name}" "${cluster_type}")
        
        info "Collecting logs for cluster: ${cluster_name}"
        
        # Collect Istio logs
        collect_istio_logs "${context}" "${cluster_logs_dir}"
        
        # Collect Kiali logs
        collect_kiali_logs "${context}" "${cluster_logs_dir}"
        
        # Collect system logs
        collect_system_logs "${context}" "${cluster_logs_dir}"
        
        # Collect application logs
        collect_application_logs "${context}" "${cluster_logs_dir}"
    done
    
    debug "Service logs collection completed"
}

# Collect Istio logs
collect_istio_logs() {
    local context="$1"
    local logs_dir="$2"
    
    local istio_logs_dir="${logs_dir}/istio"
    ensure_directory "${istio_logs_dir}"
    
    # Istiod logs
    kubectl --context="${context}" logs -n istio-system deployment/istiod --tail=500 > "${istio_logs_dir}/istiod.log" 2>&1 || true
    
    # Ingress gateway logs
    kubectl --context="${context}" logs -n istio-system deployment/istio-ingressgateway --tail=500 > "${istio_logs_dir}/ingress-gateway.log" 2>&1 || true
    
    # Istio proxy logs (sample)
    local sample_pods
    sample_pods=$(kubectl --context="${context}" get pods --all-namespaces -l security.istio.io/tlsMode=istio --no-headers | head -5 | awk '{print $2 " " $1}')
    
    while read -r pod namespace; do
        if [[ -n "${pod}" && -n "${namespace}" ]]; then
            kubectl --context="${context}" logs -n "${namespace}" "${pod}" -c istio-proxy --tail=100 > "${istio_logs_dir}/proxy-${namespace}-${pod}.log" 2>&1 || true
        fi
    done <<< "${sample_pods}"
    
    # Istio configuration
    kubectl --context="${context}" get istio-io --all-namespaces -o yaml > "${istio_logs_dir}/istio-config.yaml" 2>&1 || true
    
    debug "Istio logs collected"
}

# Collect Kiali logs
collect_kiali_logs() {
    local context="$1"
    local logs_dir="$2"
    
    local kiali_logs_dir="${logs_dir}/kiali"
    ensure_directory "${kiali_logs_dir}"
    
    # Kiali server logs
    kubectl --context="${context}" logs -n istio-system deployment/kiali --tail=1000 > "${kiali_logs_dir}/kiali.log" 2>&1 || true
    
    # Kiali operator logs (if exists)
    kubectl --context="${context}" logs -n kiali-operator deployment/kiali-operator --tail=500 > "${kiali_logs_dir}/kiali-operator.log" 2>&1 || true
    
    # Kiali configuration
    kubectl --context="${context}" get configmap -n istio-system kiali -o yaml > "${kiali_logs_dir}/kiali-config.yaml" 2>&1 || true
    kubectl --context="${context}" get kiali --all-namespaces -o yaml > "${kiali_logs_dir}/kiali-cr.yaml" 2>&1 || true
    
    debug "Kiali logs collected"
}

# Collect system logs
collect_system_logs() {
    local context="$1"
    local logs_dir="$2"
    
    local system_logs_dir="${logs_dir}/system"
    ensure_directory "${system_logs_dir}"
    
    # CoreDNS logs
    kubectl --context="${context}" logs -n kube-system -l k8s-app=kube-dns --tail=200 > "${system_logs_dir}/coredns.log" 2>&1 || true
    
    # CNI logs (if available)
    kubectl --context="${context}" logs -n kube-system daemonset/kindnet --tail=100 > "${system_logs_dir}/cni.log" 2>&1 || true
    
    # MetalLB logs (if available)
    kubectl --context="${context}" logs -n metallb-system -l app=metallb --tail=200 > "${system_logs_dir}/metallb.log" 2>&1 || true
    
    debug "System logs collected"
}

# Collect application logs
collect_application_logs() {
    local context="$1"
    local logs_dir="$2"
    
    local app_logs_dir="${logs_dir}/applications"
    ensure_directory "${app_logs_dir}"
    
    # Test application logs
    kubectl --context="${context}" logs -n kiali-test -l app=test-app --tail=100 > "${app_logs_dir}/test-app.log" 2>&1 || true
    
    # Bookinfo logs (if exists)
    local bookinfo_apps=("productpage" "details" "ratings" "reviews")
    for app in "${bookinfo_apps[@]}"; do
        kubectl --context="${context}" logs -l app="${app}" --tail=100 > "${app_logs_dir}/bookinfo-${app}.log" 2>&1 || true
    done
    
    debug "Application logs collected"
}

# Collect network information
collect_network_info() {
    local cluster_type="$1"
    local debug_dir="$2"
    
    info "Collecting network information"
    
    local network_dir="${debug_dir}/network"
    ensure_directory "${network_dir}"
    
    # Collect network info for each cluster
    for cluster_name in "${!CREATED_CLUSTERS[@]}"; do
        local cluster_network_dir="${network_dir}/${cluster_name}"
        ensure_directory "${cluster_network_dir}"
        
        local context
        context=$(get_cluster_context "${cluster_name}" "${cluster_type}")
        
        info "Collecting network info for cluster: ${cluster_name}"
        
        # Service endpoints
        kubectl --context="${context}" get endpoints --all-namespaces > "${cluster_network_dir}/endpoints.txt" 2>&1 || true
        
        # Ingress information
        kubectl --context="${context}" get ingress --all-namespaces -o wide > "${cluster_network_dir}/ingress.txt" 2>&1 || true
        
        # Network policies
        kubectl --context="${context}" get networkpolicies --all-namespaces -o yaml > "${cluster_network_dir}/network-policies.yaml" 2>&1 || true
        
        # LoadBalancer services
        kubectl --context="${context}" get svc --all-namespaces -o wide | grep LoadBalancer > "${cluster_network_dir}/loadbalancer-services.txt" 2>&1 || true
        
        # Test network connectivity
        test_network_connectivity "${context}" "${cluster_network_dir}"
    done
    
    # Collect cluster-specific network information
    case "${cluster_type}" in
        "kind")
            collect_kind_network_info "${network_dir}"
            ;;
        "minikube")
            collect_minikube_network_info "${network_dir}"
            ;;
    esac
    
    debug "Network information collection completed"
}

# Test network connectivity
test_network_connectivity() {
    local context="$1"
    local network_dir="$2"
    
    local connectivity_file="${network_dir}/connectivity-test.txt"
    
    echo "=== Network Connectivity Test ===" > "${connectivity_file}"
    echo "Timestamp: $(date)" >> "${connectivity_file}"
    echo "" >> "${connectivity_file}"
    
    # Test DNS resolution
    echo "DNS Resolution Test:" >> "${connectivity_file}"
    kubectl --context="${context}" run connectivity-test --image=curlimages/curl:latest --rm -i --restart=Never --timeout=30s -- \
        sh -c "nslookup kubernetes.default.svc.cluster.local" >> "${connectivity_file}" 2>&1 || true
    echo "" >> "${connectivity_file}"
    
    # Test service connectivity
    echo "Service Connectivity Test:" >> "${connectivity_file}"
    
    # Test Kiali service
    local kiali_svc
    kiali_svc=$(kubectl --context="${context}" get svc -n istio-system kiali -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")
    if [[ -n "${kiali_svc}" ]]; then
        echo "Testing Kiali service (${kiali_svc}:20001):" >> "${connectivity_file}"
        kubectl --context="${context}" run connectivity-test --image=curlimages/curl:latest --rm -i --restart=Never --timeout=30s -- \
            curl -s --connect-timeout 10 "http://${kiali_svc}:20001/kiali/healthz" >> "${connectivity_file}" 2>&1 || true
        echo "" >> "${connectivity_file}"
    fi
    
    # Test Istio ingress gateway
    local gateway_svc
    gateway_svc=$(kubectl --context="${context}" get svc -n istio-system istio-ingressgateway -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")
    if [[ -n "${gateway_svc}" ]]; then
        echo "Testing Istio Ingress Gateway (${gateway_svc}:80):" >> "${connectivity_file}"
        kubectl --context="${context}" run connectivity-test --image=curlimages/curl:latest --rm -i --restart=Never --timeout=30s -- \
            curl -s --connect-timeout 10 "http://${gateway_svc}/" >> "${connectivity_file}" 2>&1 || true
        echo "" >> "${connectivity_file}"
    fi
    
    echo "=== End Connectivity Test ===" >> "${connectivity_file}"
}

# Collect KinD-specific network information
collect_kind_network_info() {
    local network_dir="$1"
    
    local kind_network_dir="${network_dir}/kind"
    ensure_directory "${kind_network_dir}"
    
    # Docker networks
    docker network ls > "${kind_network_dir}/docker-networks.txt" 2>&1 || true
    
    # Kind cluster networks
    for cluster_name in "${!CREATED_CLUSTERS[@]}"; do
        if [[ "${CREATED_CLUSTERS[${cluster_name}]}" == "kind" ]]; then
            docker network inspect "kind-${cluster_name}" > "${kind_network_dir}/network-${cluster_name}.json" 2>&1 || true
        fi
    done
    
    # Container information
    docker ps --filter "label=io.x-k8s.kind.cluster" > "${kind_network_dir}/kind-containers.txt" 2>&1 || true
    
    debug "KinD network information collected"
}

# Collect Minikube-specific network information
collect_minikube_network_info() {
    local network_dir="$1"
    
    local minikube_network_dir="${network_dir}/minikube"
    ensure_directory "${minikube_network_dir}"
    
    # Minikube status for all profiles
    for cluster_name in "${!CREATED_CLUSTERS[@]}"; do
        if [[ "${CREATED_CLUSTERS[${cluster_name}]}" == "minikube" ]]; then
            local profile
            profile=$(get_minikube_profile_name "${cluster_name}")
            minikube status -p "${profile}" > "${minikube_network_dir}/status-${profile}.txt" 2>&1 || true
            minikube ip -p "${profile}" > "${minikube_network_dir}/ip-${profile}.txt" 2>&1 || true
        fi
    done
    
    # Minikube tunnel status
    pgrep -f "minikube.*tunnel" > "${minikube_network_dir}/tunnel-processes.txt" 2>&1 || true
    
    debug "Minikube network information collected"
}

# Collect Istio configuration dumps
collect_istio_config_dumps() {
    local context="$1"
    local debug_dir="$2"
    
    info "Collecting Istio configuration dumps"
    
    local istio_config_dir="${debug_dir}/istio-config"
    ensure_directory "${istio_config_dir}"
    
    # Get all Istio resources
    local istio_resources=("gateways" "virtualservices" "destinationrules" "serviceentries" "sidecars" "authorizationpolicies" "peerauthentications")
    
    for resource in "${istio_resources[@]}"; do
        kubectl --context="${context}" get "${resource}" --all-namespaces -o yaml > "${istio_config_dir}/${resource}.yaml" 2>&1 || true
    done
    
    # Istio proxy configuration
    local istiod_pod
    istiod_pod=$(kubectl --context="${context}" get pods -n istio-system -l app=istiod -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    
    if [[ -n "${istiod_pod}" ]]; then
        # Proxy status
        kubectl --context="${context}" exec -n istio-system "${istiod_pod}" -- pilot-discovery proxy-status > "${istio_config_dir}/proxy-status.txt" 2>&1 || true
        
        # Cluster configuration
        kubectl --context="${context}" exec -n istio-system "${istiod_pod}" -- pilot-discovery proxy-config cluster > "${istio_config_dir}/proxy-config-cluster.txt" 2>&1 || true
    fi
    
    debug "Istio configuration dumps collected"
}

# Create debug summary
create_debug_summary() {
    local debug_dir="$1"
    
    info "Creating debug summary"
    
    local summary_file="${debug_dir}/debug-summary.txt"
    
    cat > "${summary_file}" <<EOF
=== Kiali Integration Test Suite Debug Summary ===
Generated: $(date)
Test Suite: ${TEST_SUITE}
Cluster Type: ${CLUSTER_TYPE}

=== Environment Information ===
$(uname -a)
Docker Version: $(docker --version 2>/dev/null || echo "Not available")
Kubectl Version: $(kubectl version --client --short 2>/dev/null || echo "Not available")
$(if [[ "${CLUSTER_TYPE}" == "kind" ]]; then echo "Kind Version: $(kind version 2>/dev/null || echo "Not available")"; fi)
$(if [[ "${CLUSTER_TYPE}" == "minikube" ]]; then echo "Minikube Version: $(minikube version --short 2>/dev/null || echo "Not available")"; fi)

=== Created Clusters ===
$(for cluster in "${!CREATED_CLUSTERS[@]}"; do echo "- ${cluster}: ${CREATED_CLUSTERS[${cluster}]}"; done)

=== Installed Components ===
$(for component in "${!INSTALLED_COMPONENTS[@]}"; do echo "- ${component}: ${INSTALLED_COMPONENTS[${component}]}"; done)

=== Service URLs ===
$(env | grep -E "^(KIALI_URL|CYPRESS_BASE_URL|ISTIO_INGRESS_URL)" | sort)

=== Debug Files Structure ===
$(find "${debug_dir}" -type f -name "*.txt" -o -name "*.log" -o -name "*.yaml" | head -20)
$(if [[ $(find "${debug_dir}" -type f | wc -l) -gt 20 ]]; then echo "... and $(($(find "${debug_dir}" -type f | wc -l) - 20)) more files"; fi)

=== End Debug Summary ===
EOF
    
    info "Debug summary created: ${summary_file}"
}

# Main debug collection function
collect_debug_information() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "=== Starting Debug Information Collection ==="
    
    local debug_dir="${SCRIPT_DIR}/../debug-output"
    ensure_directory "${debug_dir}"
    
    # Collect all debug information
    collect_cluster_info "${cluster_type}" "${debug_dir}"
    collect_service_logs "${cluster_type}" "${debug_dir}"
    collect_network_info "${cluster_type}" "${debug_dir}"
    
    # Collect Istio-specific information if installed
    if [[ "${INSTALLED_COMPONENTS[istio]:-}" == "true" ]]; then
        local primary_context
        primary_context=$(get_primary_cluster_context "${cluster_type}")
        collect_istio_config_dumps "${primary_context}" "${debug_dir}"
    fi
    
    # Create summary
    create_debug_summary "${debug_dir}"
    
    # Create compressed archive
    local archive_name="kiali-debug-$(date +%Y%m%d-%H%M%S).tar.gz"
    local archive_path="${SCRIPT_DIR}/../${archive_name}"
    
    if tar -czf "${archive_path}" -C "$(dirname "${debug_dir}")" "$(basename "${debug_dir}")" 2>/dev/null; then
        info "Debug archive created: ${archive_path}"
    else
        warn "Failed to create debug archive"
    fi
    
    info "=== Debug Information Collection Completed ==="
    info "Debug information available at: ${debug_dir}"
}

# Export debug collector functions
export -f collect_cluster_info collect_service_logs collect_istio_logs
export -f collect_kiali_logs collect_system_logs collect_application_logs
export -f collect_network_info test_network_connectivity
export -f collect_kind_network_info collect_minikube_network_info
export -f collect_istio_config_dumps create_debug_summary
export -f collect_debug_information
