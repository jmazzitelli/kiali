#!/bin/bash
#
# Test Suite Runner - Core Orchestrator for Kiali Integration Test Suite
# Coordinates cluster creation, component installation, and test execution
#

# Source required libraries
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/utils.sh"
fi

source "$(dirname "${BASH_SOURCE[0]}")/validation.sh"
source "$(dirname "${BASH_SOURCE[0]}")/network-manager.sh"
source "$(dirname "${BASH_SOURCE[0]}")/service-resolver.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../providers/kind-provider.sh"
source "$(dirname "${BASH_SOURCE[0]}")/../providers/minikube-provider.sh"

# Test suite runner state
declare -A CREATED_CLUSTERS
declare -A INSTALLED_COMPONENTS
declare -A TEST_RESULTS

# Main test suite runner
run_test_suite() {
    local test_suite="$1"
    local cluster_type="$2"
    
    info "Starting test suite execution: ${test_suite} on ${cluster_type}"
    
    # Load test suite configuration
    local suite_config
    if ! suite_config=$(load_test_suite_config "${test_suite}"); then
        error "Failed to load test suite configuration: ${test_suite}"
        return 1
    fi
    
    # Validate test suite configuration
    if ! validate_test_suite_config "${test_suite}"; then
        error "Test suite configuration validation failed: ${test_suite}"
        return 1
    fi
    
    # Execute test suite phases
    if [[ "${TESTS_ONLY}" != "true" ]]; then
        # Phase 1: Setup infrastructure
        if ! setup_infrastructure "${suite_config}" "${cluster_type}"; then
            error "Infrastructure setup failed"
            return 1
        fi
        
        # Phase 2: Install components
        if ! install_components "${suite_config}" "${cluster_type}"; then
            error "Component installation failed"
            return 1
        fi
        
        # Phase 3: Validate environment
        if ! validate_test_environment "${suite_config}" "${cluster_type}"; then
            error "Environment validation failed"
            return 1
        fi
    fi
    
    # Phase 4: Execute tests (unless setup-only)
    if [[ "${SETUP_ONLY}" != "true" ]]; then
        if ! execute_tests "${suite_config}" "${cluster_type}"; then
            error "Test execution failed"
            return 1
        fi
    fi
    
    # Phase 5: Cleanup (if requested)
    if [[ "${CLEANUP}" == "true" ]]; then
        cleanup_test_environment "${suite_config}" "${cluster_type}"
    fi
    
    info "Test suite execution completed successfully: ${test_suite}"
    return 0
}

# Load test suite configuration
load_test_suite_config() {
    local test_suite="$1"
    local config_file="${INTEGRATION_TEST_DIR}/config/suites/${test_suite}.yaml"
    
    if [[ ! -f "${config_file}" ]]; then
        error "Test suite configuration file not found: ${config_file}"
        return 1
    fi
    
    debug "Loading test suite configuration: ${config_file}"
    echo "${config_file}"
    return 0
}

# Setup infrastructure (clusters)
setup_infrastructure() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Setting up infrastructure for ${cluster_type}"
    
    # Get cluster configuration
    local cluster_count
    cluster_count=$(get_yaml_value "${suite_config}" ".clusters.count" "1")
    
    if [[ "${cluster_count}" -eq 1 ]]; then
        setup_single_cluster "${suite_config}" "${cluster_type}"
    else
        setup_multicluster "${suite_config}" "${cluster_type}"
    fi
}

# Setup single cluster
setup_single_cluster() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Setting up single cluster"
    
    local cluster_name="primary"
    
    # Create cluster
    if ! create_cluster "${cluster_name}" "${cluster_type}" "${suite_config}"; then
        error "Failed to create cluster: ${cluster_name}"
        return 1
    fi
    
    # Mark cluster as created
    CREATED_CLUSTERS[${cluster_name}]="${cluster_type}"
    
    info "Single cluster setup completed"
    return 0
}

# Setup multicluster
setup_multicluster() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Setting up multicluster environment"
    
    # Get cluster configurations
    local cluster_configs
    cluster_configs=$(get_yaml_value "${suite_config}" ".clusters.configs" "[]")
    
    if [[ "${cluster_configs}" == "[]" || "${cluster_configs}" == "null" ]]; then
        error "No cluster configurations found in suite config"
        return 1
    fi
    
    # Extract cluster names (this is a simplified approach)
    local cluster_names=()
    local cluster_count
    cluster_count=$(get_yaml_value "${suite_config}" ".clusters.count" "2")
    
    # For now, use standard naming convention
    if [[ "${cluster_count}" -eq 2 ]]; then
        cluster_names=("primary" "remote")
    else
        for ((i=1; i<=cluster_count; i++)); do
            cluster_names+=("cluster${i}")
        done
    fi
    
    if [[ "${PARALLEL_SETUP}" == "true" ]]; then
        setup_clusters_parallel "${cluster_names[@]}" "${cluster_type}" "${suite_config}"
    else
        setup_clusters_sequential "${cluster_names[@]}" "${cluster_type}" "${suite_config}"
    fi
}

# Setup clusters in parallel
setup_clusters_parallel() {
    local cluster_names=("$@")
    local cluster_type="${cluster_names[-2]}"
    local suite_config="${cluster_names[-1]}"
    unset cluster_names[-1] cluster_names[-1]  # Remove last two elements
    
    info "Creating clusters in parallel: ${cluster_names[*]}"
    
    local pids=()
    for cluster_name in "${cluster_names[@]}"; do
        (
            create_cluster "${cluster_name}" "${cluster_type}" "${suite_config}"
        ) &
        pids+=($!)
    done
    
    # Wait for all clusters
    local failed_clusters=()
    for i in "${!pids[@]}"; do
        local pid="${pids[i]}"
        local cluster_name="${cluster_names[i]}"
        
        if wait "${pid}"; then
            CREATED_CLUSTERS[${cluster_name}]="${cluster_type}"
            info "✓ Cluster created: ${cluster_name}"
        else
            failed_clusters+=("${cluster_name}")
            error "✗ Cluster creation failed: ${cluster_name}"
        fi
    done
    
    if [[ ${#failed_clusters[@]} -gt 0 ]]; then
        error "Failed to create clusters: ${failed_clusters[*]}"
        return 1
    fi
    
    # Setup cross-cluster networking if needed
    if [[ ${#cluster_names[@]} -gt 1 ]]; then
        setup_cross_cluster_networking "${cluster_names[@]}" "${cluster_type}"
    fi
    
    info "Parallel cluster setup completed"
    return 0
}

# Setup clusters sequentially
setup_clusters_sequential() {
    local cluster_names=("$@")
    local cluster_type="${cluster_names[-2]}"
    local suite_config="${cluster_names[-1]}"
    unset cluster_names[-1] cluster_names[-1]  # Remove last two elements
    
    info "Creating clusters sequentially: ${cluster_names[*]}"
    
    for cluster_name in "${cluster_names[@]}"; do
        if ! create_cluster "${cluster_name}" "${cluster_type}" "${suite_config}"; then
            error "Failed to create cluster: ${cluster_name}"
            return 1
        fi
        CREATED_CLUSTERS[${cluster_name}]="${cluster_type}"
    done
    
    # Setup cross-cluster networking if needed
    if [[ ${#cluster_names[@]} -gt 1 ]]; then
        setup_cross_cluster_networking "${cluster_names[@]}" "${cluster_type}"
    fi
    
    info "Sequential cluster setup completed"
    return 0
}

# Create individual cluster
create_cluster() {
    local cluster_name="$1"
    local cluster_type="$2"
    local suite_config="$3"
    
    info "Creating ${cluster_type} cluster: ${cluster_name}"
    
    case "${cluster_type}" in
        "kind")
            kind_create_cluster "${cluster_name}" "${suite_config}"
            ;;
        "minikube")
            minikube_create_cluster "${cluster_name}" "${suite_config}"
            ;;
        *)
            error "Unsupported cluster type: ${cluster_type}"
            return 1
            ;;
    esac
}

# Setup cross-cluster networking
setup_cross_cluster_networking() {
    local cluster_names=("$@")
    local cluster_type="${cluster_names[-1]}"
    unset cluster_names[-1]  # Remove cluster type from array
    
    info "Setting up cross-cluster networking for ${cluster_type}"
    
    local primary_cluster="${cluster_names[0]}"
    local remote_clusters=("${cluster_names[@]:1}")
    
    case "${cluster_type}" in
        "kind")
            kind_setup_cross_cluster_networking "${primary_cluster}" "${remote_clusters[@]}"
            ;;
        "minikube")
            minikube_setup_cross_cluster_networking "${primary_cluster}" "${remote_clusters[@]}"
            ;;
        *)
            error "Unsupported cluster type for cross-cluster networking: ${cluster_type}"
            return 1
            ;;
    esac
}

# Install components
install_components() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Installing components"
    
    # Source component installers
    source "${INTEGRATION_TEST_DIR}/installers/istio-installer.sh"
    source "${INTEGRATION_TEST_DIR}/installers/kiali-installer.sh"
    
    # Install Istio if required
    local istio_required
    istio_required=$(get_yaml_value "${suite_config}" ".components.istio.required" "false")
    
    if [[ "${istio_required}" == "true" ]]; then
        if ! install_istio "${suite_config}" "${cluster_type}"; then
            error "Istio installation failed"
            return 1
        fi
        INSTALLED_COMPONENTS["istio"]="true"
    fi
    
    # Install Kiali if required
    local kiali_required
    kiali_required=$(get_yaml_value "${suite_config}" ".components.kiali.required" "false")
    
    if [[ "${kiali_required}" == "true" ]]; then
        if ! install_kiali "${suite_config}" "${cluster_type}"; then
            error "Kiali installation failed"
            return 1
        fi
        INSTALLED_COMPONENTS["kiali"]="true"
    fi
    
    # Install additional components based on test suite
    install_additional_components "${suite_config}" "${cluster_type}"
    
    info "Component installation completed"
    return 0
}

# Install additional components
install_additional_components() {
    local suite_config="$1"
    local cluster_type="$2"
    local test_suite="${TEST_SUITE}"
    
    case "${test_suite}" in
        *keycloak*|*auth*)
            if ! install_keycloak "${suite_config}" "${cluster_type}"; then
                warn "Keycloak installation failed"
            else
                INSTALLED_COMPONENTS["keycloak"]="true"
            fi
            ;;
        *tempo*)
            if ! install_tempo "${suite_config}" "${cluster_type}"; then
                warn "Tempo installation failed"
            else
                INSTALLED_COMPONENTS["tempo"]="true"
            fi
            ;;
        *grafana*)
            if ! install_grafana "${suite_config}" "${cluster_type}"; then
                warn "Grafana installation failed"
            else
                INSTALLED_COMPONENTS["grafana"]="true"
            fi
            ;;
    esac
}

# Validate test environment
validate_test_environment() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Validating test environment"
    
    # Wait for all services to be ready
    if ! wait_for_all_services_ready "${cluster_type}" "${TEST_SUITE}"; then
        error "Services are not ready"
        return 1
    fi
    
    # Resolve service URLs
    if ! resolve_all_service_urls "${cluster_type}" "${TEST_SUITE}"; then
        error "Failed to resolve service URLs"
        return 1
    fi
    
    # Perform health checks
    if ! perform_service_health_check "${cluster_type}" "${TEST_SUITE}"; then
        error "Service health checks failed"
        return 1
    fi
    
    # Validate network connectivity
    if ! validate_network_connectivity; then
        warn "Network connectivity validation failed"
    fi
    
    info "Test environment validation completed"
    return 0
}

# Execute tests
execute_tests() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Executing tests for suite: ${TEST_SUITE}"
    
    # Get test execution type
    local test_type
    test_type=$(get_yaml_value "${suite_config}" ".test_execution.type" "")
    
    if [[ -z "${test_type}" ]]; then
        error "Test execution type not specified in configuration"
        return 1
    fi
    
    # Source test executors
    source "${INTEGRATION_TEST_DIR}/executors/backend-executor.sh"
    source "${INTEGRATION_TEST_DIR}/executors/frontend-executor.sh"
    
    # Execute tests based on type
    case "${test_type}" in
        "go"|"backend")
            execute_backend_tests "${suite_config}" "${cluster_type}"
            ;;
        "cypress"|"frontend")
            execute_frontend_tests "${suite_config}" "${cluster_type}"
            ;;
        *)
            error "Unsupported test execution type: ${test_type}"
            return 1
            ;;
    esac
    
    local test_result=$?
    TEST_RESULTS[${TEST_SUITE}]="${test_result}"
    
    if [[ ${test_result} -eq 0 ]]; then
        info "Tests passed for suite: ${TEST_SUITE}"
    else
        error "Tests failed for suite: ${TEST_SUITE}"
        
        # Collect debug information on failure
        if [[ "${DEBUG}" == "true" ]]; then
            collect_debug_information "${suite_config}" "${cluster_type}"
        fi
    fi
    
    return ${test_result}
}

# Collect debug information
collect_debug_information() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Collecting debug information"
    
    # Create debug output directory
    local debug_dir="${SCRIPT_DIR}/../debug-output"
    ensure_directory "${debug_dir}"
    
    # Source debug collector
    source "${INTEGRATION_TEST_DIR}/lib/debug-collector.sh"
    
    # Collect debug info
    collect_cluster_info "${cluster_type}" "${debug_dir}"
    collect_service_logs "${cluster_type}" "${debug_dir}"
    collect_network_info "${cluster_type}" "${debug_dir}"
    
    info "Debug information collected in: ${debug_dir}"
}

# Cleanup test environment
cleanup_test_environment() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Cleaning up test environment"
    
    # Cleanup in reverse order of creation
    cleanup_components "${cluster_type}"
    cleanup_clusters "${cluster_type}"
    
    info "Test environment cleanup completed"
}

# Cleanup components
cleanup_components() {
    local cluster_type="$1"
    
    info "Cleaning up components"
    
    # Stop any port forwards
    cleanup_port_forwards
    
    # Stop any background processes (like minikube tunnel)
    cleanup_background_processes
    
    # Additional component-specific cleanup
    for component in "${!INSTALLED_COMPONENTS[@]}"; do
        case "${component}" in
            "keycloak")
                kubectl delete namespace keycloak --ignore-not-found=true 2>/dev/null || true
                ;;
            "tempo")
                kubectl delete namespace tempo --ignore-not-found=true 2>/dev/null || true
                ;;
            "grafana")
                kubectl delete namespace grafana --ignore-not-found=true 2>/dev/null || true
                ;;
        esac
    done
    
    debug "Component cleanup completed"
}

# Cleanup clusters
cleanup_clusters() {
    local cluster_type="$1"
    
    info "Cleaning up clusters"
    
    case "${cluster_type}" in
        "kind")
            for cluster_name in "${!CREATED_CLUSTERS[@]}"; do
                kind_delete_cluster "${cluster_name}"
            done
            ;;
        "minikube")
            for cluster_name in "${!CREATED_CLUSTERS[@]}"; do
                minikube_delete_cluster "${cluster_name}"
            done
            ;;
    esac
    
    debug "Cluster cleanup completed"
}

# Get cluster context name
get_cluster_context() {
    local cluster_name="$1"
    local cluster_type="$2"
    
    case "${cluster_type}" in
        "kind")
            echo "kind-${cluster_name}"
            ;;
        "minikube")
            local profile
            profile=$(get_minikube_profile_name "${cluster_name}")
            echo "minikube-${profile}"
            ;;
        *)
            error "Unknown cluster type: ${cluster_type}"
            return 1
            ;;
    esac
}

# Get primary cluster context
get_primary_cluster_context() {
    local cluster_type="$1"
    
    # Find the primary cluster
    for cluster_name in "${!CREATED_CLUSTERS[@]}"; do
        if [[ "${cluster_name}" == "primary" ]] || [[ "${cluster_name}" == "cluster1" ]]; then
            get_cluster_context "${cluster_name}" "${cluster_type}"
            return 0
        fi
    done
    
    # If no primary found, use the first cluster
    local first_cluster
    first_cluster=$(echo "${!CREATED_CLUSTERS[@]}" | cut -d' ' -f1)
    if [[ -n "${first_cluster}" ]]; then
        get_cluster_context "${first_cluster}" "${cluster_type}"
        return 0
    fi
    
    error "No clusters found"
    return 1
}

# Display test suite summary
display_test_summary() {
    info "=== Test Suite Summary ==="
    info "Test Suite: ${TEST_SUITE}"
    info "Cluster Type: ${CLUSTER_TYPE}"
    
    if [[ ${#CREATED_CLUSTERS[@]} -gt 0 ]]; then
        info "Created Clusters:"
        for cluster_name in "${!CREATED_CLUSTERS[@]}"; do
            info "  - ${cluster_name} (${CREATED_CLUSTERS[${cluster_name}]})"
        done
    fi
    
    if [[ ${#INSTALLED_COMPONENTS[@]} -gt 0 ]]; then
        info "Installed Components:"
        for component in "${!INSTALLED_COMPONENTS[@]}"; do
            info "  - ${component}"
        done
    fi
    
    if [[ ${#TEST_RESULTS[@]} -gt 0 ]]; then
        info "Test Results:"
        for test_suite in "${!TEST_RESULTS[@]}"; do
            local result="${TEST_RESULTS[${test_suite}]}"
            if [[ "${result}" -eq 0 ]]; then
                info "  - ${test_suite}: PASSED"
            else
                info "  - ${test_suite}: FAILED"
            fi
        done
    fi
    
    info "=========================="
}

# Add cleanup function for test suite runner
add_cleanup_function display_test_summary

# Export test suite runner functions
export -f run_test_suite load_test_suite_config setup_infrastructure
export -f setup_single_cluster setup_multicluster setup_clusters_parallel
export -f setup_clusters_sequential create_cluster setup_cross_cluster_networking
export -f install_components install_additional_components validate_test_environment
export -f execute_tests collect_debug_information cleanup_test_environment
export -f cleanup_components cleanup_clusters get_cluster_context
export -f get_primary_cluster_context display_test_summary
