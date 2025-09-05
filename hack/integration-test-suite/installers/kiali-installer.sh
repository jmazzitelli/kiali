#!/bin/bash
#
# Kiali Installer for Kiali Integration Test Suite
# Handles Kiali installation with network awareness
#

# Source required libraries
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils.sh"
fi

if ! declare -f detect_optimal_service_type >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/network-manager.sh"
fi

# Kiali configuration
readonly KIALI_NAMESPACE="istio-system"
readonly KIALI_OPERATOR_NAMESPACE="kiali-operator"
readonly KIALI_VERSION_DEFAULT="v1.80.0"

# Install Kiali
install_kiali() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Kiali"
    
    # Get deployment type
    local deployment_type="in-cluster"
    if [[ -n "${suite_config}" ]]; then
        deployment_type=$(get_yaml_value "${suite_config}" ".components.kiali.deployment_type" "${deployment_type}")
    fi
    
    # Get primary cluster context
    local primary_context
    if ! primary_context=$(get_primary_cluster_context "${cluster_type}"); then
        error "Cannot determine primary cluster context"
        return 1
    fi
    
    # Install Kiali based on deployment type
    case "${deployment_type}" in
        "in-cluster")
            install_kiali_in_cluster "${primary_context}" "${cluster_type}" "${suite_config}"
            ;;
        "external")
            install_kiali_external "${suite_config}" "${cluster_type}"
            ;;
        "local")
            install_kiali_local "${suite_config}" "${cluster_type}"
            ;;
        *)
            error "Unsupported Kiali deployment type: ${deployment_type}"
            return 1
            ;;
    esac
}

# Install Kiali in-cluster
install_kiali_in_cluster() {
    local context="$1"
    local cluster_type="$2"
    local suite_config="$3"
    
    info "Installing Kiali in-cluster"
    
    # Determine service type
    local service_type
    service_type=$(detect_optimal_service_type "${cluster_type}" "kiali")
    
    # Get Kiali version
    local kiali_version="${KIALI_VERSION_DEFAULT}"
    if [[ -n "${suite_config}" ]]; then
        kiali_version=$(get_yaml_value "${suite_config}" ".components.kiali.version" "${kiali_version}")
    fi
    
    # Install using Helm (preferred) or operator
    if command -v helm >/dev/null 2>&1; then
        install_kiali_helm "${context}" "${cluster_type}" "${service_type}" "${kiali_version}"
    else
        install_kiali_operator "${context}" "${cluster_type}" "${service_type}" "${kiali_version}"
    fi
    
    # Wait for Kiali to be ready
    if ! wait_for_kiali_ready "${context}"; then
        error "Kiali is not ready"
        return 1
    fi
    
    info "Kiali in-cluster installation completed"
    return 0
}

# Install Kiali using Helm
install_kiali_helm() {
    local context="$1"
    local cluster_type="$2"
    local service_type="$3"
    local kiali_version="$4"
    
    info "Installing Kiali using Helm"
    
    # Add Kiali Helm repository
    if ! helm repo add kiali https://kiali.org/helm-charts 2>/dev/null; then
        warn "Failed to add Kiali Helm repo, it may already exist"
    fi
    
    helm repo update kiali
    
    # Create values for Helm installation
    local values_file
    values_file=$(create_temp_file "kiali-values")
    
    cat > "${values_file}" <<EOF
deployment:
  service_type: "${service_type}"
  image_name: "quay.io/kiali/kiali"
  image_version: "${kiali_version}"
  
auth:
  strategy: "anonymous"
  
external_services:
  prometheus:
    url: "http://prometheus:9090"
  grafana:
    enabled: false
  tracing:
    enabled: false
    
server:
  web_root: "/kiali"
  port: 20001
EOF
    
    # Install Kiali
    if ! helm upgrade --install kiali kiali/kiali-server \
        --kube-context="${context}" \
        --namespace="${KIALI_NAMESPACE}" \
        --create-namespace \
        --values="${values_file}" \
        --wait \
        --timeout=600s; then
        error "Failed to install Kiali using Helm"
        return 1
    fi
    
    debug "Kiali Helm installation completed"
    return 0
}

# Install Kiali using Operator
install_kiali_operator() {
    local context="$1"
    local cluster_type="$2"
    local service_type="$3"
    local kiali_version="$4"
    
    info "Installing Kiali using Operator"
    
    # Install Kiali Operator
    kubectl --context="${context}" create namespace "${KIALI_OPERATOR_NAMESPACE}" --dry-run=client -o yaml | \
        kubectl --context="${context}" apply -f -
    
    # Apply operator manifests
    kubectl --context="${context}" apply -f https://raw.githubusercontent.com/kiali/kiali-operator/master/deploy/kiali-operator.yaml
    
    # Wait for operator to be ready
    kubectl --context="${context}" wait --for=condition=available deployment/kiali-operator \
        -n "${KIALI_OPERATOR_NAMESPACE}" --timeout=300s
    
    # Create Kiali CR
    local kiali_cr
    kiali_cr=$(create_temp_file "kiali-cr")
    
    cat > "${kiali_cr}" <<EOF
apiVersion: kiali.io/v1alpha1
kind: Kiali
metadata:
  name: kiali
  namespace: ${KIALI_NAMESPACE}
spec:
  deployment:
    service_type: "${service_type}"
    image_name: "quay.io/kiali/kiali"
    image_version: "${kiali_version}"
  
  auth:
    strategy: "anonymous"
  
  external_services:
    prometheus:
      url: "http://prometheus:9090"
    grafana:
      enabled: false
    tracing:
      enabled: false
      
  server:
    web_root: "/kiali"
    port: 20001
EOF
    
    # Apply Kiali CR
    kubectl --context="${context}" apply -f "${kiali_cr}"
    
    debug "Kiali Operator installation completed"
    return 0
}

# Install Kiali external (on management cluster)
install_kiali_external() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Kiali external"
    
    # Get management and workload cluster contexts
    local management_context
    management_context=$(get_cluster_context "management" "${cluster_type}")
    local workload_context
    workload_context=$(get_cluster_context "workload" "${cluster_type}")
    
    # Install Kiali on management cluster
    install_kiali_in_cluster "${management_context}" "${cluster_type}" "${suite_config}"
    
    # Configure access to workload cluster
    setup_external_cluster_access "${management_context}" "${workload_context}" "${cluster_type}"
    
    info "Kiali external installation completed"
    return 0
}

# Install Kiali local mode
install_kiali_local() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Kiali in local mode"
    
    # Kiali local mode runs outside the cluster
    # We need to configure it to access the cluster remotely
    
    # Create Kiali configuration for local mode
    local kiali_config
    kiali_config=$(create_temp_file "kiali-config")
    
    cat > "${kiali_config}" <<EOF
server:
  port: 20001
  web_root: "/kiali"
  
auth:
  strategy: "anonymous"
  
external_services:
  prometheus:
    url: "http://localhost:9090"
  grafana:
    enabled: false
  tracing:
    enabled: false

deployment:
  cluster_wide_access: true
EOF
    
    # Start Kiali in local mode (this would typically be done by the test)
    info "Kiali local mode configuration created: ${kiali_config}"
    export KIALI_LOCAL_CONFIG="${kiali_config}"
    
    # For local mode, we'll start port forwarding to access cluster services
    setup_local_mode_port_forwards "${cluster_type}"
    
    info "Kiali local mode setup completed"
    return 0
}

# Setup external cluster access for Kiali
setup_external_cluster_access() {
    local management_context="$1"
    local workload_context="$2"
    local cluster_type="$3"
    
    info "Setting up external cluster access"
    
    # Get workload cluster endpoint
    local workload_endpoint
    workload_endpoint=$(kubectl --context="${workload_context}" config view --minify -o jsonpath='{.clusters[0].cluster.server}')
    
    # Create secret with workload cluster access
    local kubeconfig_secret
    kubeconfig_secret=$(create_temp_file "workload-kubeconfig")
    
    # Extract workload cluster kubeconfig
    kubectl --context="${workload_context}" config view --minify --raw > "${kubeconfig_secret}"
    
    # Create secret in management cluster
    kubectl --context="${management_context}" create secret generic workload-cluster-access \
        -n "${KIALI_NAMESPACE}" \
        --from-file=kubeconfig="${kubeconfig_secret}" \
        --dry-run=client -o yaml | \
        kubectl --context="${management_context}" apply -f -
    
    debug "External cluster access configured"
}

# Setup port forwards for local mode
setup_local_mode_port_forwards() {
    local cluster_type="$1"
    
    info "Setting up port forwards for local mode"
    
    # Get primary cluster context
    local primary_context
    primary_context=$(get_primary_cluster_context "${cluster_type}")
    
    # Port forward Prometheus
    if kubectl --context="${primary_context}" get svc prometheus -n "${KIALI_NAMESPACE}" >/dev/null 2>&1; then
        start_port_forward "prometheus" "${KIALI_NAMESPACE}" "9090" "9090" "${primary_context}"
    fi
    
    # Port forward other services as needed
    debug "Port forwards setup for local mode"
}

# Wait for Kiali to be ready
wait_for_kiali_ready() {
    local context="$1"
    
    info "Waiting for Kiali to be ready"
    
    # Wait for Kiali deployment
    if ! kubectl --context="${context}" wait --for=condition=available deployment/kiali \
        -n "${KIALI_NAMESPACE}" --timeout=600s; then
        error "Kiali deployment is not ready"
        return 1
    fi
    
    # Wait for Kiali service
    if ! wait_for_service_ready "kiali" "${KIALI_NAMESPACE}" "kind" "${context}" "300s"; then
        error "Kiali service is not ready"
        return 1
    fi
    
    info "Kiali is ready"
    return 0
}

# Configure Kiali for multicluster
configure_kiali_multicluster() {
    local primary_context="$1"
    local remote_contexts=("${@:2}")
    
    info "Configuring Kiali for multicluster"
    
    # Create cluster secrets for remote clusters
    for remote_context in "${remote_contexts[@]}"; do
        create_kiali_cluster_secret "${primary_context}" "${remote_context}"
    done
    
    # Update Kiali configuration for multicluster
    update_kiali_multicluster_config "${primary_context}" "${remote_contexts[@]}"
    
    info "Kiali multicluster configuration completed"
}

# Create cluster secret for Kiali
create_kiali_cluster_secret() {
    local primary_context="$1"
    local remote_context="$2"
    
    local cluster_name
    cluster_name=$(echo "${remote_context}" | sed 's/.*-//')
    
    info "Creating cluster secret for: ${cluster_name}"
    
    # Get remote cluster kubeconfig
    local remote_kubeconfig
    remote_kubeconfig=$(create_temp_file "remote-kubeconfig-${cluster_name}")
    kubectl --context="${remote_context}" config view --minify --raw > "${remote_kubeconfig}"
    
    # Create secret in primary cluster
    kubectl --context="${primary_context}" create secret generic "kiali-cluster-${cluster_name}" \
        -n "${KIALI_NAMESPACE}" \
        --from-file=kubeconfig="${remote_kubeconfig}" \
        --dry-run=client -o yaml | \
        kubectl --context="${primary_context}" apply -f -
    
    # Label the secret
    kubectl --context="${primary_context}" label secret "kiali-cluster-${cluster_name}" \
        -n "${KIALI_NAMESPACE}" \
        kiali.io/cluster="${cluster_name}"
}

# Update Kiali configuration for multicluster
update_kiali_multicluster_config() {
    local primary_context="$1"
    local remote_contexts=("${@:2}")
    
    # Create cluster configuration
    local clusters_config=""
    for remote_context in "${remote_contexts[@]}"; do
        local cluster_name
        cluster_name=$(echo "${remote_context}" | sed 's/.*-//')
        
        clusters_config="${clusters_config}
  - name: \"${cluster_name}\"
    secret_name: \"kiali-cluster-${cluster_name}\"
    enabled: true"
    done
    
    # Create ConfigMap with multicluster configuration
    kubectl --context="${primary_context}" create configmap kiali-multicluster-config \
        -n "${KIALI_NAMESPACE}" \
        --from-literal=clusters="${clusters_config}" \
        --dry-run=client -o yaml | \
        kubectl --context="${primary_context}" apply -f -
    
    # Restart Kiali to pick up new configuration
    kubectl --context="${primary_context}" rollout restart deployment/kiali -n "${KIALI_NAMESPACE}"
    
    # Wait for rollout to complete
    kubectl --context="${primary_context}" rollout status deployment/kiali -n "${KIALI_NAMESPACE}" --timeout=300s
}

# Get Kiali logs
get_kiali_logs() {
    local context="$1"
    local lines="${2:-100}"
    
    info "Getting Kiali logs"
    kubectl --context="${context}" logs -n "${KIALI_NAMESPACE}" deployment/kiali --tail="${lines}"
}

# Uninstall Kiali
uninstall_kiali() {
    local context="$1"
    
    info "Uninstalling Kiali from context: ${context}"
    
    # Try Helm uninstall first
    if command -v helm >/dev/null 2>&1; then
        helm uninstall kiali --kube-context="${context}" -n "${KIALI_NAMESPACE}" 2>/dev/null || true
    fi
    
    # Remove Kiali CR if using operator
    kubectl --context="${context}" delete kiali kiali -n "${KIALI_NAMESPACE}" --ignore-not-found=true 2>/dev/null || true
    
    # Remove operator
    kubectl --context="${context}" delete namespace "${KIALI_OPERATOR_NAMESPACE}" --ignore-not-found=true 2>/dev/null || true
    
    # Clean up resources
    kubectl --context="${context}" delete all,secrets,configmaps,serviceaccounts,clusterroles,clusterrolebindings \
        -l app=kiali -n "${KIALI_NAMESPACE}" --ignore-not-found=true 2>/dev/null || true
    
    info "Kiali uninstalled from context: ${context}"
}

# Install additional components for specific test suites
install_keycloak() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Keycloak"
    
    local primary_context
    primary_context=$(get_primary_cluster_context "${cluster_type}")
    
    # Create Keycloak namespace
    kubectl --context="${primary_context}" create namespace keycloak --dry-run=client -o yaml | \
        kubectl --context="${primary_context}" apply -f -
    
    # Install Keycloak using Helm
    helm repo add bitnami https://charts.bitnami.com/bitnami 2>/dev/null || true
    helm repo update bitnami
    
    local service_type
    service_type=$(detect_optimal_service_type "${cluster_type}" "keycloak")
    
    helm upgrade --install keycloak bitnami/keycloak \
        --kube-context="${primary_context}" \
        --namespace=keycloak \
        --set service.type="${service_type}" \
        --set auth.adminUser=admin \
        --set auth.adminPassword=admin \
        --wait \
        --timeout=600s
    
    info "Keycloak installation completed"
    return 0
}

install_tempo() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Tempo"
    
    local primary_context
    primary_context=$(get_primary_cluster_context "${cluster_type}")
    
    # Create Tempo namespace
    kubectl --context="${primary_context}" create namespace tempo --dry-run=client -o yaml | \
        kubectl --context="${primary_context}" apply -f -
    
    # Install Tempo using manifests
    kubectl --context="${primary_context}" apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tempo
  namespace: tempo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tempo
  template:
    metadata:
      labels:
        app: tempo
    spec:
      containers:
      - name: tempo
        image: grafana/tempo:latest
        ports:
        - containerPort: 3200
        - containerPort: 14268
---
apiVersion: v1
kind: Service
metadata:
  name: tempo
  namespace: tempo
spec:
  selector:
    app: tempo
  ports:
  - name: http
    port: 3200
    targetPort: 3200
  - name: jaeger
    port: 14268
    targetPort: 14268
  type: $(detect_optimal_service_type "${cluster_type}" "tempo")
EOF
    
    # Wait for Tempo to be ready
    kubectl --context="${primary_context}" wait --for=condition=available deployment/tempo \
        -n tempo --timeout=300s
    
    info "Tempo installation completed"
    return 0
}

install_grafana() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing Grafana"
    
    local primary_context
    primary_context=$(get_primary_cluster_context "${cluster_type}")
    
    # Install Grafana using Helm
    helm repo add grafana https://grafana.github.io/helm-charts 2>/dev/null || true
    helm repo update grafana
    
    local service_type
    service_type=$(detect_optimal_service_type "${cluster_type}" "grafana")
    
    helm upgrade --install grafana grafana/grafana \
        --kube-context="${primary_context}" \
        --namespace=grafana \
        --create-namespace \
        --set service.type="${service_type}" \
        --set adminPassword=admin \
        --wait \
        --timeout=600s
    
    info "Grafana installation completed"
    return 0
}

# Export Kiali installer functions
export -f install_kiali install_kiali_in_cluster install_kiali_helm install_kiali_operator
export -f install_kiali_external install_kiali_local setup_external_cluster_access
export -f setup_local_mode_port_forwards wait_for_kiali_ready configure_kiali_multicluster
export -f create_kiali_cluster_secret update_kiali_multicluster_config get_kiali_logs
export -f uninstall_kiali install_keycloak install_tempo install_grafana
