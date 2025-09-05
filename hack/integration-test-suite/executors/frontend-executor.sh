#!/bin/bash
#
# Frontend Test Executor for Kiali Integration Test Suite
# Executes Cypress E2E tests for the Kiali frontend
#

# Source required libraries
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils.sh"
fi

if ! declare -f setup_cypress_environment >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/service-resolver.sh"
fi

# Frontend test configuration
readonly FRONTEND_TEST_TIMEOUT_DEFAULT="1800s"
readonly CYPRESS_CONFIG_FILE="cypress.config.ts"
readonly CYPRESS_DEFAULT_PATTERN="**/*.feature"

# Execute frontend tests
execute_frontend_tests() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Executing frontend integration tests"
    
    # Setup Cypress environment
    if ! setup_cypress_environment "${cluster_type}" "${TEST_SUITE}"; then
        error "Failed to setup Cypress environment"
        return 1
    fi
    
    # Validate prerequisites
    if ! validate_frontend_test_prerequisites; then
        error "Frontend test prerequisites validation failed"
        return 1
    fi
    
    # Get test configuration
    local test_timeout="${TEST_TIMEOUT:-${FRONTEND_TEST_TIMEOUT_DEFAULT}}"
    local cypress_config
    cypress_config=$(get_yaml_value "${suite_config}" ".test_execution.config_file" "${CYPRESS_CONFIG_FILE}")
    local test_pattern
    test_pattern=$(get_yaml_value "${suite_config}" ".test_execution.pattern" "${CYPRESS_DEFAULT_PATTERN}")
    
    # Change to the frontend directory
    local frontend_dir="${SCRIPT_DIR}/../frontend"
    cd "${frontend_dir}" || {
        error "Cannot change to frontend directory: ${frontend_dir}"
        return 1
    }
    
    # Ensure dependencies are installed
    if ! ensure_frontend_dependencies; then
        error "Failed to ensure frontend dependencies"
        return 1
    fi
    
    # Execute tests based on test suite type
    case "${TEST_SUITE}" in
        "frontend")
            execute_standard_frontend_tests "${test_timeout}" "${cypress_config}" "${test_pattern}"
            ;;
        "frontend-ambient")
            execute_ambient_frontend_tests "${test_timeout}" "${cypress_config}" "${test_pattern}"
            ;;
        "frontend-multicluster-"*)
            execute_multicluster_frontend_tests "${test_timeout}" "${cypress_config}" "${test_pattern}"
            ;;
        "frontend-external-kiali")
            execute_external_kiali_tests "${test_timeout}" "${cypress_config}" "${test_pattern}"
            ;;
        "frontend-tempo")
            execute_tempo_frontend_tests "${test_timeout}" "${cypress_config}" "${test_pattern}"
            ;;
        "local")
            execute_local_mode_tests "${test_timeout}" "${cypress_config}" "${test_pattern}"
            ;;
        *)
            error "Unsupported frontend test suite: ${TEST_SUITE}"
            return 1
            ;;
    esac
}

# Validate frontend test prerequisites
validate_frontend_test_prerequisites() {
    info "Validating frontend test prerequisites"
    
    # Check Node.js
    if ! command -v node >/dev/null 2>&1; then
        error "Node.js not found in PATH"
        return 1
    fi
    
    # Check Yarn
    if ! command -v yarn >/dev/null 2>&1; then
        error "Yarn not found in PATH"
        return 1
    fi
    
    # Check if we're in the frontend directory
    if [[ ! -f "package.json" ]] || ! grep -q "cypress" package.json 2>/dev/null; then
        error "Not in frontend directory or package.json not found"
        return 1
    fi
    
    # Check Cypress base URL
    if [[ -z "${CYPRESS_BASE_URL:-}" ]]; then
        error "CYPRESS_BASE_URL not set"
        return 1
    fi
    
    info "Frontend test prerequisites validated"
    return 0
}

# Ensure frontend dependencies are installed
ensure_frontend_dependencies() {
    info "Ensuring frontend dependencies are installed"
    
    if [[ ! -d "node_modules" ]] || [[ ! -f "yarn.lock" ]]; then
        info "Installing frontend dependencies"
        if ! yarn install --frozen-lockfile; then
            error "Failed to install frontend dependencies"
            return 1
        fi
    else
        debug "Frontend dependencies already installed"
    fi
    
    # Verify Cypress is available
    if ! npx cypress version >/dev/null 2>&1; then
        error "Cypress is not available after dependency installation"
        return 1
    fi
    
    return 0
}

# Execute standard frontend tests
execute_standard_frontend_tests() {
    local timeout="$1"
    local config_file="$2"
    local pattern="$3"
    
    info "Executing standard frontend tests"
    info "Timeout: ${timeout}"
    info "Config: ${config_file}"
    info "Pattern: ${pattern}"
    info "Base URL: ${CYPRESS_BASE_URL}"
    
    # Create Cypress output directories
    ensure_directory "cypress/screenshots"
    ensure_directory "cypress/videos"
    
    # Run Cypress tests
    local test_output
    test_output=$(create_temp_file "frontend-test-output")
    
    local cypress_cmd="npx cypress run --config-file ${config_file} --spec 'cypress/e2e/${pattern}'"
    
    info "Running: ${cypress_cmd}"
    
    if timeout "${timeout}" ${cypress_cmd} 2>&1 | tee "${test_output}"; then
        info "Frontend tests PASSED"
        display_frontend_test_summary "${test_output}"
        return 0
    else
        local exit_code=$?
        error "Frontend tests FAILED (exit code: ${exit_code})"
        display_frontend_test_failures "${test_output}"
        return ${exit_code}
    fi
}

# Execute ambient mode frontend tests
execute_ambient_frontend_tests() {
    local timeout="$1"
    local config_file="$2"
    local pattern="$3"
    
    info "Executing ambient mode frontend tests"
    
    # Set ambient-specific environment variables
    export CYPRESS_AMBIENT_MODE="true"
    
    # Run tests with ambient configuration
    execute_standard_frontend_tests "${timeout}" "${config_file}" "${pattern}"
}

# Execute multicluster frontend tests
execute_multicluster_frontend_tests() {
    local timeout="$1"
    local config_file="$2"
    local pattern="$3"
    
    info "Executing multicluster frontend tests"
    
    # Set multicluster-specific environment variables
    export CYPRESS_MULTICLUSTER="true"
    
    # Determine multicluster type
    case "${TEST_SUITE}" in
        "frontend-multicluster-primary-remote")
            export CYPRESS_MULTICLUSTER_TYPE="primary-remote"
            ;;
        "frontend-multicluster-multi-primary")
            export CYPRESS_MULTICLUSTER_TYPE="multi-primary"
            ;;
    esac
    
    # Use longer timeout for multicluster tests
    local mc_timeout="${timeout}"
    if [[ "${timeout}" == "${FRONTEND_TEST_TIMEOUT_DEFAULT}" ]]; then
        mc_timeout="2400s"  # 40 minutes for multicluster
    fi
    
    execute_standard_frontend_tests "${mc_timeout}" "${config_file}" "${pattern}"
}

# Execute external Kiali tests
execute_external_kiali_tests() {
    local timeout="$1"
    local config_file="$2"
    local pattern="$3"
    
    info "Executing external Kiali frontend tests"
    
    # Set external Kiali environment variables
    export CYPRESS_EXTERNAL_KIALI="true"
    
    execute_standard_frontend_tests "${timeout}" "${config_file}" "${pattern}"
}

# Execute Tempo tracing tests
execute_tempo_frontend_tests() {
    local timeout="$1"
    local config_file="$2"
    local pattern="$3"
    
    info "Executing Tempo tracing frontend tests"
    
    # Set Tempo-specific environment variables
    export CYPRESS_TEMPO_ENABLED="true"
    export CYPRESS_TRACING_ENABLED="true"
    
    # Verify Tempo URL is available
    if [[ -n "${TEMPO_URL:-}" ]]; then
        export CYPRESS_TEMPO_URL="${TEMPO_URL}"
        info "Tempo URL: ${TEMPO_URL}"
    else
        warn "TEMPO_URL not set, some tests may fail"
    fi
    
    execute_standard_frontend_tests "${timeout}" "${config_file}" "${pattern}"
}

# Execute local mode tests
execute_local_mode_tests() {
    local timeout="$1"
    local config_file="$2"
    local pattern="$3"
    
    info "Executing local mode frontend tests"
    
    # Set local mode environment variables
    export CYPRESS_LOCAL_MODE="true"
    
    # For local mode, we need to start Kiali locally
    if ! start_kiali_local_mode; then
        error "Failed to start Kiali in local mode"
        return 1
    fi
    
    # Update base URL for local Kiali
    export CYPRESS_BASE_URL="http://localhost:20001"
    
    # Execute tests
    local result
    execute_standard_frontend_tests "${timeout}" "${config_file}" "${pattern}"
    result=$?
    
    # Stop local Kiali
    stop_kiali_local_mode
    
    return ${result}
}

# Start Kiali in local mode
start_kiali_local_mode() {
    info "Starting Kiali in local mode"
    
    # Check if Kiali binary is available
    if [[ ! -x "${HOME}/go/bin/kiali" ]]; then
        error "Kiali binary not found or not executable: ${HOME}/go/bin/kiali"
        return 1
    fi
    
    # Use the local configuration if available
    local kiali_config="${KIALI_LOCAL_CONFIG:-}"
    if [[ -z "${kiali_config}" ]]; then
        # Create minimal local configuration
        kiali_config=$(create_temp_file "kiali-local-config")
        cat > "${kiali_config}" <<EOF
server:
  port: 20001
  web_root: "/kiali"
auth:
  strategy: "anonymous"
external_services:
  prometheus:
    url: "http://localhost:9090"
deployment:
  cluster_wide_access: true
EOF
    fi
    
    # Start Kiali in background
    local kiali_pid
    kiali_pid=$(start_background_process "kiali-local" "${HOME}/go/bin/kiali -config ${kiali_config}")
    
    # Wait for Kiali to be ready
    if wait_for_condition "Kiali local mode to be ready" "is_url_accessible http://localhost:20001/kiali 5" 60 2; then
        info "Kiali local mode is ready"
        export KIALI_LOCAL_PID="${kiali_pid}"
        return 0
    else
        error "Kiali local mode failed to start"
        kill_process_tree "${kiali_pid}"
        return 1
    fi
}

# Stop Kiali local mode
stop_kiali_local_mode() {
    info "Stopping Kiali local mode"
    
    if [[ -n "${KIALI_LOCAL_PID:-}" ]]; then
        kill_process_tree "${KIALI_LOCAL_PID}"
        unset KIALI_LOCAL_PID
    fi
    
    # Kill any remaining Kiali processes on port 20001
    local kiali_processes
    kiali_processes=$(lsof -ti:20001 2>/dev/null || echo "")
    for pid in ${kiali_processes}; do
        kill_process_tree "${pid}"
    done
}

# Display frontend test summary
display_frontend_test_summary() {
    local test_output="$1"
    
    info "=== Frontend Test Summary ==="
    
    if [[ -f "${test_output}" ]]; then
        # Extract Cypress test results
        local total_tests
        total_tests=$(grep -E "^\s+[0-9]+ passing" "${test_output}" | awk '{sum += $1} END {print sum+0}')
        local failed_tests
        failed_tests=$(grep -E "^\s+[0-9]+ failing" "${test_output}" | awk '{sum += $1} END {print sum+0}')
        local pending_tests
        pending_tests=$(grep -E "^\s+[0-9]+ pending" "${test_output}" | awk '{sum += $1} END {print sum+0}')
        
        info "Total passing: ${total_tests:-0}"
        info "Total failing: ${failed_tests:-0}"
        info "Total pending: ${pending_tests:-0}"
        
        # Extract duration
        local duration
        duration=$(grep "All specs passed!" "${test_output}" | grep -oE '\([0-9]+:[0-9]+:[0-9]+\)' | tr -d '()' || echo "unknown")
        if [[ "${duration}" != "unknown" ]]; then
            info "Duration: ${duration}"
        fi
        
        # Show browser info
        local browser
        browser=$(grep "Running:" "${test_output}" | head -1 | awk '{print $NF}' || echo "unknown")
        info "Browser: ${browser}"
    fi
    
    info "=========================="
}

# Display frontend test failures
display_frontend_test_failures() {
    local test_output="$1"
    
    error "=== Frontend Test Failures ==="
    
    if [[ -f "${test_output}" ]]; then
        # Show failed test details
        if grep -q "failing" "${test_output}"; then
            error "Failed tests:"
            grep -A 10 "failing" "${test_output}" | while read -r line; do
                if [[ "${line}" =~ ^[[:space:]]*[0-9]+\) ]]; then
                    error "  ${line}"
                fi
            done
        fi
        
        # Show error messages
        if grep -q "Error:" "${test_output}"; then
            error "Error messages:"
            grep -A 3 "Error:" "${test_output}" | while read -r line; do
                error "  ${line}"
            done
        fi
        
        # Show timeout messages
        if grep -q "Timed out" "${test_output}"; then
            error "Timeout messages:"
            grep "Timed out" "${test_output}" | while read -r line; do
                error "  ${line}"
            done
        fi
    fi
    
    # List screenshot and video files
    if [[ -d "cypress/screenshots" ]]; then
        local screenshots
        screenshots=$(find cypress/screenshots -name "*.png" 2>/dev/null | head -5)
        if [[ -n "${screenshots}" ]]; then
            error "Screenshot files (first 5):"
            echo "${screenshots}" | while read -r screenshot; do
                error "  ${screenshot}"
            done
        fi
    fi
    
    if [[ -d "cypress/videos" ]]; then
        local videos
        videos=$(find cypress/videos -name "*.mp4" 2>/dev/null | head -5)
        if [[ -n "${videos}" ]]; then
            error "Video files (first 5):"
            echo "${videos}" | while read -r video; do
                error "  ${video}"
            done
        fi
    fi
    
    error "============================="
}

# Run specific frontend test
run_specific_frontend_test() {
    local test_spec="$1"
    local timeout="${2:-600s}"
    
    info "Running specific frontend test: ${test_spec}"
    
    # Change to frontend directory
    local frontend_dir="${SCRIPT_DIR}/../frontend"
    cd "${frontend_dir}" || {
        error "Cannot change to frontend directory: ${frontend_dir}"
        return 1
    }
    
    # Run the specific test
    if timeout "${timeout}" npx cypress run --spec "${test_spec}"; then
        info "Test PASSED: ${test_spec}"
        return 0
    else
        error "Test FAILED: ${test_spec}"
        return 1
    fi
}

# Open Cypress interactive mode
open_cypress_interactive() {
    info "Opening Cypress in interactive mode"
    
    # Change to frontend directory
    local frontend_dir="${SCRIPT_DIR}/../frontend"
    cd "${frontend_dir}" || {
        error "Cannot change to frontend directory: ${frontend_dir}"
        return 1
    fi
    
    # Open Cypress
    npx cypress open
}

# Setup frontend test data
setup_frontend_test_data() {
    local cluster_type="$1"
    
    info "Setting up frontend test data"
    
    # Get primary cluster context
    local primary_context
    primary_context=$(get_primary_cluster_context "${cluster_type}")
    
    # Apply frontend test workloads
    apply_frontend_test_workloads "${primary_context}"
    
    # Wait for workloads to be ready
    wait_for_frontend_test_workloads "${primary_context}"
    
    info "Frontend test data setup completed"
}

# Apply frontend test workloads
apply_frontend_test_workloads() {
    local context="$1"
    
    info "Applying frontend test workloads"
    
    # Apply bookinfo sample application
    if ! kubectl --context="${context}" apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/bookinfo/platform/kube/bookinfo.yaml; then
        warn "Failed to apply bookinfo application"
        return 1
    fi
    
    # Apply bookinfo gateway
    if ! kubectl --context="${context}" apply -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/bookinfo/networking/bookinfo-gateway.yaml; then
        warn "Failed to apply bookinfo gateway"
        return 1
    fi
    
    debug "Frontend test workloads applied"
}

# Wait for frontend test workloads
wait_for_frontend_test_workloads() {
    local context="$1"
    
    info "Waiting for frontend test workloads to be ready"
    
    # Wait for bookinfo deployments
    local deployments=("productpage-v1" "details-v1" "ratings-v1" "reviews-v1" "reviews-v2" "reviews-v3")
    
    for deployment in "${deployments[@]}"; do
        if ! kubectl --context="${context}" wait --for=condition=available deployment/"${deployment}" \
            --timeout=300s; then
            warn "Deployment ${deployment} is not ready"
        fi
    done
    
    debug "Frontend test workloads are ready"
    return 0
}

# Cleanup frontend test data
cleanup_frontend_test_data() {
    local cluster_type="$1"
    
    info "Cleaning up frontend test data"
    
    # Get primary cluster context
    local primary_context
    primary_context=$(get_primary_cluster_context "${cluster_type}")
    
    # Delete bookinfo application
    kubectl --context="${primary_context}" delete -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/bookinfo/platform/kube/bookinfo.yaml --ignore-not-found=true 2>/dev/null || true
    kubectl --context="${primary_context}" delete -f https://raw.githubusercontent.com/istio/istio/release-1.20/samples/bookinfo/networking/bookinfo-gateway.yaml --ignore-not-found=true 2>/dev/null || true
    
    debug "Frontend test data cleanup completed"
}

# Export frontend executor functions
export -f execute_frontend_tests validate_frontend_test_prerequisites
export -f ensure_frontend_dependencies execute_standard_frontend_tests
export -f execute_ambient_frontend_tests execute_multicluster_frontend_tests
export -f execute_external_kiali_tests execute_tempo_frontend_tests
export -f execute_local_mode_tests start_kiali_local_mode stop_kiali_local_mode
export -f display_frontend_test_summary display_frontend_test_failures
export -f run_specific_frontend_test open_cypress_interactive
export -f setup_frontend_test_data apply_frontend_test_workloads
export -f wait_for_frontend_test_workloads cleanup_frontend_test_data
