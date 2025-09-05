#!/bin/bash
#
# Service Resolver for Kiali Integration Test Suite
# Resolves service URLs and handles service-specific networking logic
#

# Source required libraries
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/utils.sh"
fi

if ! declare -f get_service_url >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/network-manager.sh"
fi

# Service configuration
declare -A SERVICE_PORTS
SERVICE_PORTS[kiali]="20001"
SERVICE_PORTS[istio-ingressgateway]="80"
SERVICE_PORTS[keycloak]="8080"
SERVICE_PORTS[tempo]="3200"
SERVICE_PORTS[grafana]="3000"

declare -A SERVICE_NAMESPACES
SERVICE_NAMESPACES[kiali]="istio-system"
SERVICE_NAMESPACES[istio-ingressgateway]="istio-system"
SERVICE_NAMESPACES[keycloak]="keycloak"
SERVICE_NAMESPACES[tempo]="tempo"
SERVICE_NAMESPACES[grafana]="grafana"

# Resolve Kiali service URL
resolve_kiali_url() {
    local cluster_type="$1"
    local namespace="${2:-istio-system}"
    local context="${3:-}"
    
    info "Resolving Kiali URL for ${cluster_type} cluster"
    
    local kiali_url
    kiali_url=$(get_service_url "kiali" "${namespace}" "http" "${cluster_type}" "${context}")
    
    if [[ -z "${kiali_url}" ]]; then
        error "Failed to resolve Kiali URL"
        return 1
    fi
    
    # Kiali typically runs on /kiali path
    if [[ "${kiali_url}" != *"/kiali" ]]; then
        kiali_url="${kiali_url}/kiali"
    fi
    
    export KIALI_URL="${kiali_url}"
    export CYPRESS_BASE_URL="${kiali_url%/kiali}"  # Base URL without /kiali path
    
    info "Kiali URL resolved: ${KIALI_URL}"
    debug "Cypress base URL: ${CYPRESS_BASE_URL}"
    
    # Verify Kiali is accessible
    if ! verify_service_accessibility "${KIALI_URL}" 300; then
        error "Kiali is not accessible at: ${KIALI_URL}"
        return 1
    fi
    
    return 0
}

# Resolve Istio Ingress Gateway URL
resolve_istio_gateway_url() {
    local cluster_type="$1"
    local namespace="${2:-istio-system}"
    local context="${3:-}"
    
    info "Resolving Istio Ingress Gateway URL for ${cluster_type} cluster"
    
    local gateway_url
    gateway_url=$(get_service_url "istio-ingressgateway" "${namespace}" "http" "${cluster_type}" "${context}")
    
    if [[ -z "${gateway_url}" ]]; then
        error "Failed to resolve Istio Ingress Gateway URL"
        return 1
    fi
    
    export ISTIO_INGRESS_URL="${gateway_url}"
    
    info "Istio Ingress Gateway URL resolved: ${ISTIO_INGRESS_URL}"
    
    return 0
}

# Resolve Keycloak URL
resolve_keycloak_url() {
    local cluster_type="$1"
    local namespace="${2:-keycloak}"
    local context="${3:-}"
    
    info "Resolving Keycloak URL for ${cluster_type} cluster"
    
    local keycloak_url
    keycloak_url=$(get_service_url "keycloak" "${namespace}" "http" "${cluster_type}" "${context}")
    
    if [[ -z "${keycloak_url}" ]]; then
        error "Failed to resolve Keycloak URL"
        return 1
    fi
    
    export KEYCLOAK_URL="${keycloak_url}"
    
    info "Keycloak URL resolved: ${KEYCLOAK_URL}"
    
    return 0
}

# Resolve Tempo URL
resolve_tempo_url() {
    local cluster_type="$1"
    local namespace="${2:-tempo}"
    local context="${3:-}"
    
    info "Resolving Tempo URL for ${cluster_type} cluster"
    
    local tempo_url
    tempo_url=$(get_service_url "tempo" "${namespace}" "http" "${cluster_type}" "${context}")
    
    if [[ -z "${tempo_url}" ]]; then
        error "Failed to resolve Tempo URL"
        return 1
    fi
    
    export TEMPO_URL="${tempo_url}"
    
    info "Tempo URL resolved: ${TEMPO_URL}"
    
    return 0
}

# Resolve Grafana URL
resolve_grafana_url() {
    local cluster_type="$1"
    local namespace="${2:-grafana}"
    local context="${3:-}"
    
    info "Resolving Grafana URL for ${cluster_type} cluster"
    
    local grafana_url
    grafana_url=$(get_service_url "grafana" "${namespace}" "http" "${cluster_type}" "${context}")
    
    if [[ -z "${grafana_url}" ]]; then
        error "Failed to resolve Grafana URL"
        return 1
    fi
    
    export GRAFANA_URL="${grafana_url}"
    
    info "Grafana URL resolved: ${GRAFANA_URL}"
    
    return 0
}

# Resolve all service URLs for a test suite
resolve_all_service_urls() {
    local cluster_type="$1"
    local test_suite="$2"
    local context="${3:-}"
    
    info "Resolving all service URLs for test suite: ${test_suite}"
    
    # Always resolve Kiali URL
    if ! resolve_kiali_url "${cluster_type}" "istio-system" "${context}"; then
        return 1
    fi
    
    # Always resolve Istio gateway URL
    if ! resolve_istio_gateway_url "${cluster_type}" "istio-system" "${context}"; then
        return 1
    fi
    
    # Resolve additional URLs based on test suite
    case "${test_suite}" in
        *keycloak*|*auth*)
            resolve_keycloak_url "${cluster_type}" "keycloak" "${context}" || warn "Keycloak URL resolution failed"
            ;;
        *tempo*)
            resolve_tempo_url "${cluster_type}" "tempo" "${context}" || warn "Tempo URL resolution failed"
            ;;
        *grafana*)
            resolve_grafana_url "${cluster_type}" "grafana" "${context}" || warn "Grafana URL resolution failed"
            ;;
    esac
    
    # Export resolved URLs for debugging
    debug "=== Resolved Service URLs ==="
    debug "KIALI_URL: ${KIALI_URL:-not set}"
    debug "CYPRESS_BASE_URL: ${CYPRESS_BASE_URL:-not set}"
    debug "ISTIO_INGRESS_URL: ${ISTIO_INGRESS_URL:-not set}"
    debug "KEYCLOAK_URL: ${KEYCLOAK_URL:-not set}"
    debug "TEMPO_URL: ${TEMPO_URL:-not set}"
    debug "GRAFANA_URL: ${GRAFANA_URL:-not set}"
    debug "=========================="
    
    return 0
}

# Verify service accessibility
verify_service_accessibility() {
    local service_url="$1"
    local timeout="${2:-60}"
    local expected_status="${3:-200}"
    
    info "Verifying service accessibility: ${service_url}"
    
    local max_attempts=$((timeout / 5))
    local attempt=0
    
    while [[ ${attempt} -lt ${max_attempts} ]]; do
        if is_url_accessible "${service_url}" 10 "${expected_status}"; then
            info "Service is accessible: ${service_url}"
            return 0
        fi
        
        sleep 5
        ((attempt++))
        
        if [[ $((attempt % 6)) -eq 0 ]]; then
            info "Still waiting for service to be accessible: ${service_url} (${attempt}/${max_attempts})"
        fi
    done
    
    error "Service is not accessible after ${timeout}s: ${service_url}"
    return 1
}

# Get Kiali authentication strategy
get_auth_strategy() {
    local kiali_url="${1:-${KIALI_URL}}"
    
    if [[ -z "${kiali_url}" ]]; then
        error "Kiali URL not provided or resolved"
        return 1
    fi
    
    info "Detecting Kiali authentication strategy"
    
    # Try to get Kiali config
    local config_url="${kiali_url}/api/config"
    local config_response
    
    if config_response=$(curl -s --connect-timeout 10 "${config_url}" 2>/dev/null); then
        # Try to extract auth strategy from config
        if command -v jq >/dev/null 2>&1; then
            local auth_strategy
            auth_strategy=$(echo "${config_response}" | jq -r '.auth.strategy // "anonymous"' 2>/dev/null || echo "anonymous")
            
            if [[ "${auth_strategy}" != "null" && -n "${auth_strategy}" ]]; then
                info "Detected auth strategy: ${auth_strategy}"
                echo "${auth_strategy}"
                return 0
            fi
        fi
    fi
    
    # Default to anonymous if detection fails
    warn "Could not detect auth strategy, defaulting to 'anonymous'"
    echo "anonymous"
    return 0
}

# Setup environment variables for Cypress
setup_cypress_environment() {
    local cluster_type="$1"
    local test_suite="$2"
    local context="${3:-}"
    
    info "Setting up Cypress environment for ${test_suite}"
    
    # Resolve service URLs first
    if ! resolve_all_service_urls "${cluster_type}" "${test_suite}" "${context}"; then
        error "Failed to resolve service URLs for Cypress"
        return 1
    fi
    
    # Basic Cypress environment
    export CI="1"
    export TERM="xterm"
    
    # Get auth strategy
    local auth_strategy
    auth_strategy=$(get_auth_strategy "${KIALI_URL}")
    export CYPRESS_AUTH_STRATEGY="${auth_strategy}"
    
    # Set auth provider based on strategy
    case "${auth_strategy}" in
        "openid")
            export CYPRESS_AUTH_PROVIDER="my_htpasswd_provider"
            ;;
        *)
            export CYPRESS_AUTH_PROVIDER="anonymous"
            ;;
    esac
    
    # Set cluster-specific timeouts
    case "${cluster_type}" in
        "kind")
            export CYPRESS_COMMAND_TIMEOUT="30000"
            export CYPRESS_REQUEST_TIMEOUT="10000"
            export CYPRESS_PAGE_LOAD_TIMEOUT="30000"
            ;;
        "minikube")
            # Minikube may be slower, especially with NodePort
            export CYPRESS_COMMAND_TIMEOUT="60000"
            export CYPRESS_REQUEST_TIMEOUT="20000"
            export CYPRESS_PAGE_LOAD_TIMEOUT="60000"
            ;;
    esac
    
    # Test isolation
    export CYPRESS_TEST_ISOLATION="false"
    
    # Export environment for debugging
    debug "=== Cypress Environment ==="
    debug "CI: ${CI}"
    debug "TERM: ${TERM}"
    debug "CYPRESS_BASE_URL: ${CYPRESS_BASE_URL}"
    debug "CYPRESS_AUTH_STRATEGY: ${CYPRESS_AUTH_STRATEGY}"
    debug "CYPRESS_AUTH_PROVIDER: ${CYPRESS_AUTH_PROVIDER}"
    debug "CYPRESS_COMMAND_TIMEOUT: ${CYPRESS_COMMAND_TIMEOUT}"
    debug "CYPRESS_REQUEST_TIMEOUT: ${CYPRESS_REQUEST_TIMEOUT}"
    debug "CYPRESS_PAGE_LOAD_TIMEOUT: ${CYPRESS_PAGE_LOAD_TIMEOUT}"
    debug "CYPRESS_TEST_ISOLATION: ${CYPRESS_TEST_ISOLATION}"
    debug "=========================="
    
    info "Cypress environment setup completed"
    return 0
}

# Setup environment variables for backend tests
setup_backend_test_environment() {
    local cluster_type="$1"
    local context="${2:-}"
    
    info "Setting up backend test environment"
    
    # Setup kubeconfig for the test
    setup_kubeconfig_for_tests "${cluster_type}" "${context}"
    
    # Set cluster-specific timeouts
    case "${cluster_type}" in
        "kind")
            export TEST_TIMEOUT="600s"
            ;;
        "minikube")
            export TEST_TIMEOUT="900s"  # Longer timeout for Minikube
            ;;
    esac
    
    # Ensure Kiali binary is available and executable
    if [[ ! -x "${HOME}/go/bin/kiali" ]]; then
        error "Kiali binary not found or not executable: ${HOME}/go/bin/kiali"
        return 1
    fi
    
    debug "=== Backend Test Environment ==="
    debug "TEST_TIMEOUT: ${TEST_TIMEOUT}"
    debug "KUBECONFIG: ${KUBECONFIG:-default}"
    debug "Kiali binary: ${HOME}/go/bin/kiali"
    debug "=========================="
    
    info "Backend test environment setup completed"
    return 0
}

# Setup kubeconfig for tests
setup_kubeconfig_for_tests() {
    local cluster_type="$1"
    local context="${2:-}"
    
    if [[ -n "${context}" ]]; then
        # Use specific context
        export KUBECONFIG="${HOME}/.kube/config"
        kubectl config use-context "${context}"
        info "Using Kubernetes context: ${context}"
    else
        # Use current context
        local current_context
        current_context=$(kubectl config current-context 2>/dev/null || echo "none")
        info "Using current Kubernetes context: ${current_context}"
    fi
}

# Wait for all services to be ready
wait_for_all_services_ready() {
    local cluster_type="$1"
    local test_suite="$2"
    local context="${3:-}"
    
    info "Waiting for all services to be ready for test suite: ${test_suite}"
    
    # Always wait for Kiali
    if ! wait_for_service_ready "kiali" "istio-system" "${cluster_type}" "${context}"; then
        error "Kiali service is not ready"
        return 1
    fi
    
    # Always wait for Istio gateway
    if ! wait_for_service_ready "istio-ingressgateway" "istio-system" "${cluster_type}" "${context}"; then
        error "Istio Ingress Gateway service is not ready"
        return 1
    fi
    
    # Wait for additional services based on test suite
    case "${test_suite}" in
        *keycloak*|*auth*)
            if ! wait_for_service_ready "keycloak" "keycloak" "${cluster_type}" "${context}"; then
                warn "Keycloak service is not ready"
            fi
            ;;
        *tempo*)
            if ! wait_for_service_ready "tempo" "tempo" "${cluster_type}" "${context}"; then
                warn "Tempo service is not ready"
            fi
            ;;
        *grafana*)
            if ! wait_for_service_ready "grafana" "grafana" "${cluster_type}" "${context}"; then
                warn "Grafana service is not ready"
            fi
            ;;
    esac
    
    info "All required services are ready"
    return 0
}

# Get service endpoint for health checks
get_service_health_endpoint() {
    local service_name="$1"
    local base_url="$2"
    
    case "${service_name}" in
        "kiali")
            echo "${base_url}/api/healthz"
            ;;
        "keycloak")
            echo "${base_url}/auth/realms/master"
            ;;
        "tempo")
            echo "${base_url}/ready"
            ;;
        "grafana")
            echo "${base_url}/api/health"
            ;;
        *)
            echo "${base_url}"
            ;;
    esac
}

# Perform comprehensive service health check
perform_service_health_check() {
    local cluster_type="$1"
    local test_suite="$2"
    local context="${3:-}"
    
    info "Performing comprehensive service health check"
    
    local failed_services=()
    
    # Check Kiali health
    if [[ -n "${KIALI_URL:-}" ]]; then
        local kiali_health_url
        kiali_health_url=$(get_service_health_endpoint "kiali" "${KIALI_URL}")
        if ! is_url_accessible "${kiali_health_url}" 10; then
            failed_services+=("kiali")
        else
            debug "✓ Kiali health check passed"
        fi
    fi
    
    # Check additional services based on test suite
    case "${test_suite}" in
        *keycloak*|*auth*)
            if [[ -n "${KEYCLOAK_URL:-}" ]]; then
                local keycloak_health_url
                keycloak_health_url=$(get_service_health_endpoint "keycloak" "${KEYCLOAK_URL}")
                if ! is_url_accessible "${keycloak_health_url}" 10; then
                    failed_services+=("keycloak")
                else
                    debug "✓ Keycloak health check passed"
                fi
            fi
            ;;
        *tempo*)
            if [[ -n "${TEMPO_URL:-}" ]]; then
                local tempo_health_url
                tempo_health_url=$(get_service_health_endpoint "tempo" "${TEMPO_URL}")
                if ! is_url_accessible "${tempo_health_url}" 10; then
                    failed_services+=("tempo")
                else
                    debug "✓ Tempo health check passed"
                fi
            fi
            ;;
        *grafana*)
            if [[ -n "${GRAFANA_URL:-}" ]]; then
                local grafana_health_url
                grafana_health_url=$(get_service_health_endpoint "grafana" "${GRAFANA_URL}")
                if ! is_url_accessible "${grafana_health_url}" 10; then
                    failed_services+=("grafana")
                else
                    debug "✓ Grafana health check passed"
                fi
            fi
            ;;
    esac
    
    if [[ ${#failed_services[@]} -gt 0 ]]; then
        error "Health check failed for services: ${failed_services[*]}"
        return 1
    fi
    
    info "All service health checks passed"
    return 0
}

# Export service resolver functions
export -f resolve_kiali_url resolve_istio_gateway_url resolve_keycloak_url
export -f resolve_tempo_url resolve_grafana_url resolve_all_service_urls
export -f verify_service_accessibility get_auth_strategy
export -f setup_cypress_environment setup_backend_test_environment
export -f setup_kubeconfig_for_tests wait_for_all_services_ready
export -f get_service_health_endpoint perform_service_health_check
