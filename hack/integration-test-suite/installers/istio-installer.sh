#!/bin/bash
#
# Istio Installer for Kiali Integration Test Suite
# Handles Istio installation with network awareness
#

# Source required libraries
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils.sh"
fi

if ! declare -f detect_optimal_service_type >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/network-manager.sh"
fi

# Istio configuration
readonly ISTIO_VERSION_DEFAULT="1.20.1"
readonly ISTIO_NAMESPACE="istio-system"
readonly ISTIO_DOWNLOAD_URL="https://github.com/istio/istio/releases/download"

# Install Istio
install_istio() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Istio"
    
    # Get Istio version
    local istio_version="${ISTIO_VERSION:-${ISTIO_VERSION_DEFAULT}}"
    if [[ -n "${suite_config}" ]]; then
        istio_version=$(get_yaml_value "${suite_config}" ".components.istio.version" "${istio_version}")
    fi
    
    # Get Istio mode
    local istio_mode="standard"
    if [[ -n "${suite_config}" ]]; then
        istio_mode=$(get_yaml_value "${suite_config}" ".components.istio.mode" "${istio_mode}")
    fi
    
    # Install istioctl if not available
    if ! ensure_istioctl "${istio_version}"; then
        error "Failed to ensure istioctl is available"
        return 1
    fi
    
    # Get primary cluster context
    local primary_context
    if ! primary_context=$(get_primary_cluster_context "${cluster_type}"); then
        error "Cannot determine primary cluster context"
        return 1
    fi
    
    # Install Istio based on mode
    case "${istio_mode}" in
        "standard")
            install_istio_standard "${primary_context}" "${cluster_type}"
            ;;
        "ambient")
            install_istio_ambient "${primary_context}" "${cluster_type}"
            ;;
        "multicluster")
            install_istio_multicluster "${suite_config}" "${cluster_type}"
            ;;
        "external-controlplane")
            install_istio_external_controlplane "${suite_config}" "${cluster_type}"
            ;;
        *)
            error "Unsupported Istio mode: ${istio_mode}"
            return 1
            ;;
    esac
}

# Ensure istioctl is available
ensure_istioctl() {
    local version="$1"
    
    if command -v istioctl >/dev/null 2>&1; then
        local current_version
        current_version=$(istioctl version --short --remote=false 2>/dev/null | cut -d':' -f2 | tr -d ' ' || echo "unknown")
        if [[ "${current_version}" == "${version}" ]]; then
            debug "istioctl ${version} is already available"
            return 0
        fi
    fi
    
    info "Downloading istioctl ${version}"
    
    # Create temporary directory for istioctl
    local istio_dir
    istio_dir=$(create_temp_file "istio-${version}")
    rm -f "${istio_dir}"
    mkdir -p "${istio_dir}"
    
    # Download and extract Istio
    local platform
    platform=$(get_platform)
    local download_url="${ISTIO_DOWNLOAD_URL}/${version}/istio-${version}-${platform}.tar.gz"
    
    info "Downloading Istio from: ${download_url}"
    
    if ! curl -L "${download_url}" | tar xz -C "${istio_dir}" --strip-components=1; then
        error "Failed to download Istio ${version}"
        return 1
    fi
    
    # Make istioctl available in PATH
    local istioctl_path="${istio_dir}/bin/istioctl"
    if [[ ! -f "${istioctl_path}" ]]; then
        error "istioctl binary not found in downloaded package"
        return 1
    fi
    
    chmod +x "${istioctl_path}"
    export PATH="${istio_dir}/bin:${PATH}"
    
    # Verify istioctl works
    if ! istioctl version --short --remote=false >/dev/null 2>&1; then
        error "Downloaded istioctl is not working"
        return 1
    fi
    
    info "istioctl ${version} is ready"
    return 0
}

# Install Istio in standard mode
install_istio_standard() {
    local context="$1"
    local cluster_type="$2"
    
    info "Installing Istio in standard mode"
    
    # Install Istio
    if ! istioctl install --context="${context}" --set values.pilot.env.PILOT_ENABLE_CROSS_CLUSTER_WORKLOAD_ENTRY=true -y; then
        error "Failed to install Istio"
        return 1
    fi
    
    # Configure ingress gateway service type
    configure_istio_ingress_gateway "${context}" "${cluster_type}"
    
    # Wait for Istio to be ready
    if ! wait_for_istio_ready "${context}"; then
        error "Istio is not ready"
        return 1
    fi
    
    # Install Istio addons
    install_istio_addons "${context}" "${cluster_type}"
    
    info "Istio standard installation completed"
    return 0
}

# Install Istio in ambient mode
install_istio_ambient() {
    local context="$1"
    local cluster_type="$2"
    
    info "Installing Istio in ambient mode"
    
    # Install Istio with ambient profile
    if ! istioctl install --context="${context}" --set values.pilot.env.PILOT_ENABLE_AMBIENT=true -y; then
        error "Failed to install Istio in ambient mode"
        return 1
    fi
    
    # Install CNI plugin for ambient
    if ! kubectl --context="${context}" apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/addons/ztunnel.yaml; then
        warn "Failed to install ztunnel, ambient mode may not work properly"
    fi
    
    # Configure ingress gateway service type
    configure_istio_ingress_gateway "${context}" "${cluster_type}"
    
    # Wait for Istio to be ready
    if ! wait_for_istio_ready "${context}"; then
        error "Istio ambient is not ready"
        return 1
    fi
    
    # Install Istio addons
    install_istio_addons "${context}" "${cluster_type}"
    
    info "Istio ambient installation completed"
    return 0
}

# Install Istio in multicluster mode
install_istio_multicluster() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Istio in multicluster mode"
    
    # Get cluster topology
    local topology
    topology=$(get_yaml_value "${suite_config}" ".components.istio.topology" "primary-remote")
    
    case "${topology}" in
        "primary-remote")
            install_istio_primary_remote "${suite_config}" "${cluster_type}"
            ;;
        "multi-primary")
            install_istio_multi_primary "${suite_config}" "${cluster_type}"
            ;;
        *)
            error "Unsupported multicluster topology: ${topology}"
            return 1
            ;;
    esac
}

# Install Istio primary-remote multicluster
install_istio_primary_remote() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Istio in primary-remote topology"
    
    # Get cluster contexts
    local primary_context
    primary_context=$(get_cluster_context "primary" "${cluster_type}")
    local remote_context
    remote_context=$(get_cluster_context "remote" "${cluster_type}")
    
    # Install on primary cluster
    info "Installing Istio on primary cluster"
    if ! istioctl install --context="${primary_context}" \
        --set values.pilot.env.PILOT_ENABLE_CROSS_CLUSTER_WORKLOAD_ENTRY=true \
        --set values.istiodRemote.enabled=false -y; then
        error "Failed to install Istio on primary cluster"
        return 1
    fi
    
    # Install on remote cluster
    info "Installing Istio on remote cluster"
    if ! istioctl install --context="${remote_context}" \
        --set values.pilot.env.PILOT_ENABLE_CROSS_CLUSTER_WORKLOAD_ENTRY=true \
        --set values.istiodRemote.enabled=true \
        --set values.pilot.env.EXTERNAL_ISTIOD=true -y; then
        error "Failed to install Istio on remote cluster"
        return 1
    fi
    
    # Configure cross-cluster secrets
    setup_multicluster_secrets "${primary_context}" "${remote_context}"
    
    # Configure ingress gateways
    configure_istio_ingress_gateway "${primary_context}" "${cluster_type}"
    configure_istio_ingress_gateway "${remote_context}" "${cluster_type}"
    
    # Wait for both clusters to be ready
    wait_for_istio_ready "${primary_context}"
    wait_for_istio_ready "${remote_context}"
    
    # Install addons on primary cluster
    install_istio_addons "${primary_context}" "${cluster_type}"
    
    info "Istio primary-remote installation completed"
    return 0
}

# Install Istio multi-primary multicluster
install_istio_multi_primary() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Istio in multi-primary topology"
    
    # Get cluster contexts
    local cluster1_context
    cluster1_context=$(get_cluster_context "cluster1" "${cluster_type}")
    local cluster2_context
    cluster2_context=$(get_cluster_context "cluster2" "${cluster_type}")
    
    # Install on both clusters as primaries
    info "Installing Istio on cluster1"
    if ! istioctl install --context="${cluster1_context}" \
        --set values.pilot.env.PILOT_ENABLE_CROSS_CLUSTER_WORKLOAD_ENTRY=true -y; then
        error "Failed to install Istio on cluster1"
        return 1
    fi
    
    info "Installing Istio on cluster2"
    if ! istioctl install --context="${cluster2_context}" \
        --set values.pilot.env.PILOT_ENABLE_CROSS_CLUSTER_WORKLOAD_ENTRY=true -y; then
        error "Failed to install Istio on cluster2"
        return 1
    fi
    
    # Configure cross-cluster secrets for both directions
    setup_multicluster_secrets "${cluster1_context}" "${cluster2_context}"
    setup_multicluster_secrets "${cluster2_context}" "${cluster1_context}"
    
    # Configure ingress gateways
    configure_istio_ingress_gateway "${cluster1_context}" "${cluster_type}"
    configure_istio_ingress_gateway "${cluster2_context}" "${cluster_type}"
    
    # Wait for both clusters to be ready
    wait_for_istio_ready "${cluster1_context}"
    wait_for_istio_ready "${cluster2_context}"
    
    # Install addons on both clusters
    install_istio_addons "${cluster1_context}" "${cluster_type}"
    install_istio_addons "${cluster2_context}" "${cluster_type}"
    
    info "Istio multi-primary installation completed"
    return 0
}

# Install Istio external controlplane
install_istio_external_controlplane() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Istio with external controlplane"
    
    # Get cluster contexts
    local controlplane_context
    controlplane_context=$(get_cluster_context "controlplane" "${cluster_type}")
    local dataplane_context
    dataplane_context=$(get_cluster_context "dataplane" "${cluster_type}")
    
    # Install controlplane
    info "Installing Istio controlplane"
    if ! istioctl install --context="${controlplane_context}" \
        --set values.pilot.env.PILOT_ENABLE_CROSS_CLUSTER_WORKLOAD_ENTRY=true \
        --set values.istiodRemote.enabled=false -y; then
        error "Failed to install Istio controlplane"
        return 1
    fi
    
    # Install dataplane (gateway only)
    info "Installing Istio dataplane"
    if ! istioctl install --context="${dataplane_context}" \
        --set components.pilot.enabled=false \
        --set components.istiodRemote.enabled=true -y; then
        error "Failed to install Istio dataplane"
        return 1
    fi
    
    # Configure external controlplane access
    setup_external_controlplane_access "${controlplane_context}" "${dataplane_context}" "${cluster_type}"
    
    # Wait for components to be ready
    wait_for_istio_ready "${controlplane_context}"
    wait_for_istio_ready "${dataplane_context}"
    
    info "Istio external controlplane installation completed"
    return 0
}

# Configure Istio ingress gateway service type
configure_istio_ingress_gateway() {
    local context="$1"
    local cluster_type="$2"
    
    info "Configuring Istio ingress gateway for ${cluster_type}"
    
    local service_type
    service_type=$(detect_optimal_service_type "${cluster_type}" "istio-gateway")
    
    info "Setting ingress gateway service type to: ${service_type}"
    
    # Update service type
    kubectl --context="${context}" patch svc istio-ingressgateway -n "${ISTIO_NAMESPACE}" \
        -p "{\"spec\":{\"type\":\"${service_type}\"}}"
    
    # Wait for service to be ready
    wait_for_service_ready "istio-ingressgateway" "${ISTIO_NAMESPACE}" "${cluster_type}" "${context}"
}

# Setup multicluster secrets
setup_multicluster_secrets() {
    local source_context="$1"
    local target_context="$2"
    
    info "Setting up multicluster secrets from ${source_context} to ${target_context}"
    
    # Create namespace in target cluster
    kubectl --context="${target_context}" create namespace "${ISTIO_NAMESPACE}" --dry-run=client -o yaml | \
        kubectl --context="${target_context}" apply -f -
    
    # Get source cluster secret
    kubectl --context="${source_context}" get secret cacerts -n "${ISTIO_NAMESPACE}" -o yaml | \
        kubectl --context="${target_context}" apply -f -
    
    # Label namespace for multicluster
    kubectl --context="${target_context}" label namespace "${ISTIO_NAMESPACE}" \
        topology.istio.io/network="${target_context}" --overwrite
}

# Setup external controlplane access
setup_external_controlplane_access() {
    local controlplane_context="$1"
    local dataplane_context="$2"
    local cluster_type="$3"
    
    info "Setting up external controlplane access"
    
    # Get controlplane service URL
    local controlplane_url
    controlplane_url=$(get_service_url "istiod" "${ISTIO_NAMESPACE}" "https" "${cluster_type}" "${controlplane_context}")
    
    if [[ -z "${controlplane_url}" ]]; then
        error "Cannot get controlplane service URL"
        return 1
    fi
    
    # Configure dataplane to use external controlplane
    kubectl --context="${dataplane_context}" create configmap external-istiod -n "${ISTIO_NAMESPACE}" \
        --from-literal=root-cert.pem="$(kubectl --context="${controlplane_context}" get configmap istio-ca-root-cert -n "${ISTIO_NAMESPACE}" -o jsonpath='{.data.root-cert\.pem}')" \
        --from-literal=cert-chain.pem="$(kubectl --context="${controlplane_context}" get secret cacerts -n "${ISTIO_NAMESPACE}" -o jsonpath='{.data.cert-chain\.pem}' | base64 -d)" \
        --dry-run=client -o yaml | kubectl --context="${dataplane_context}" apply -f -
}

# Wait for Istio to be ready
wait_for_istio_ready() {
    local context="$1"
    
    info "Waiting for Istio to be ready in context: ${context}"
    
    # Wait for istiod deployment
    if ! kubectl --context="${context}" wait --for=condition=available deployment/istiod \
        -n "${ISTIO_NAMESPACE}" --timeout=600s; then
        error "istiod deployment is not ready"
        return 1
    fi
    
    # Wait for ingress gateway deployment
    if ! kubectl --context="${context}" wait --for=condition=available deployment/istio-ingressgateway \
        -n "${ISTIO_NAMESPACE}" --timeout=300s; then
        error "istio-ingressgateway deployment is not ready"
        return 1
    fi
    
    # Verify Istio installation
    if ! istioctl --context="${context}" verify-install; then
        error "Istio installation verification failed"
        return 1
    fi
    
    info "Istio is ready in context: ${context}"
    return 0
}

# Install Istio addons
install_istio_addons() {
    local context="$1"
    local cluster_type="$2"
    
    info "Installing Istio addons"
    
    # Install Prometheus
    kubectl --context="${context}" apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/addons/prometheus.yaml
    
    # Wait for Prometheus
    kubectl --context="${context}" wait --for=condition=available deployment/prometheus \
        -n "${ISTIO_NAMESPACE}" --timeout=300s || warn "Prometheus addon not ready"
    
    info "Istio addons installation completed"
}

# Uninstall Istio
uninstall_istio() {
    local context="$1"
    
    info "Uninstalling Istio from context: ${context}"
    
    # Remove Istio installation
    istioctl --context="${context}" uninstall --purge -y 2>/dev/null || true
    
    # Remove namespace
    kubectl --context="${context}" delete namespace "${ISTIO_NAMESPACE}" --ignore-not-found=true 2>/dev/null || true
    
    info "Istio uninstalled from context: ${context}"
}

# Export Istio installer functions
export -f install_istio ensure_istioctl install_istio_standard install_istio_ambient
export -f install_istio_multicluster install_istio_primary_remote install_istio_multi_primary
export -f install_istio_external_controlplane configure_istio_ingress_gateway
export -f setup_multicluster_secrets setup_external_controlplane_access
export -f wait_for_istio_ready install_istio_addons uninstall_istio
