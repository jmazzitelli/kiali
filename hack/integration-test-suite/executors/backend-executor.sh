#!/bin/bash
#
# Backend Test Executor for Kiali Integration Test Suite
# Executes Go integration tests for the Kiali backend
#

# Source required libraries
if ! declare -f info >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/utils.sh"
fi

if ! declare -f setup_backend_test_environment >/dev/null 2>&1; then
    source "$(dirname "${BASH_SOURCE[0]}")/../lib/service-resolver.sh"
fi

# Backend test configuration
readonly BACKEND_TEST_TIMEOUT_DEFAULT="600s"
readonly KIALI_BINARY_PATH="${HOME}/go/bin/kiali"

# Execute backend tests
execute_backend_tests() {
    local suite_config="$1"
    local cluster_type="$2"
    
    info "Executing backend integration tests"
    
    # Setup backend test environment
    if ! setup_backend_test_environment "${cluster_type}"; then
        error "Failed to setup backend test environment"
        return 1
    fi
    
    # Validate prerequisites
    if ! validate_backend_test_prerequisites; then
        error "Backend test prerequisites validation failed"
        return 1
    fi
    
    # Get test configuration
    local test_timeout="${TEST_TIMEOUT:-${BACKEND_TEST_TIMEOUT_DEFAULT}}"
    local test_pattern
    test_pattern=$(get_yaml_value "${suite_config}" ".test_execution.pattern" "./tests/integration/...")
    
    # Change to the Kiali source directory
    local kiali_source_dir="${SCRIPT_DIR}/.."
    cd "${kiali_source_dir}" || {
        error "Cannot change to Kiali source directory: ${kiali_source_dir}"
        return 1
    }
    
    # Execute tests based on test suite type
    case "${TEST_SUITE}" in
        "backend")
            execute_standard_backend_tests "${test_timeout}" "${test_pattern}"
            ;;
        "backend-external-controlplane")
            execute_external_controlplane_tests "${test_timeout}" "${test_pattern}"
            ;;
        *)
            error "Unsupported backend test suite: ${TEST_SUITE}"
            return 1
            ;;
    esac
}

# Validate backend test prerequisites
validate_backend_test_prerequisites() {
    info "Validating backend test prerequisites"
    
    # Check Kiali binary
    if [[ ! -f "${KIALI_BINARY_PATH}" ]]; then
        error "Kiali binary not found: ${KIALI_BINARY_PATH}"
        return 1
    fi
    
    if [[ ! -x "${KIALI_BINARY_PATH}" ]]; then
        error "Kiali binary is not executable: ${KIALI_BINARY_PATH}"
        return 1
    fi
    
    # Check Go installation
    if ! command -v go >/dev/null 2>&1; then
        error "Go not found in PATH"
        return 1
    fi
    
    # Check kubeconfig access
    if ! kubectl cluster-info >/dev/null 2>&1; then
        error "Cannot access Kubernetes cluster"
        return 1
    fi
    
    # Check if we're in the right directory
    if [[ ! -f "go.mod" ]] || ! grep -q "github.com/kiali/kiali" go.mod 2>/dev/null; then
        error "Not in Kiali source directory or go.mod not found"
        return 1
    fi
    
    info "Backend test prerequisites validated"
    return 0
}

# Execute standard backend tests
execute_standard_backend_tests() {
    local timeout="$1"
    local pattern="$2"
    
    info "Executing standard backend integration tests"
    info "Timeout: ${timeout}"
    info "Pattern: ${pattern}"
    
    # Set test environment variables
    export KIALI_BINARY="${KIALI_BINARY_PATH}"
    export TEST_TIMEOUT="${timeout}"
    
    # Run the integration tests
    info "Running: make test-integration-controller"
    
    # Capture test output
    local test_output
    test_output=$(create_temp_file "backend-test-output")
    
    # Run tests with timeout
    if timeout "${timeout}" make test-integration-controller 2>&1 | tee "${test_output}"; then
        info "Backend integration tests PASSED"
        
        # Display test summary
        display_backend_test_summary "${test_output}"
        
        return 0
    else
        local exit_code=$?
        error "Backend integration tests FAILED (exit code: ${exit_code})"
        
        # Display test failure details
        display_backend_test_failures "${test_output}"
        
        return ${exit_code}
    fi
}

# Execute external controlplane tests
execute_external_controlplane_tests() {
    local timeout="$1"
    local pattern="$2"
    
    info "Executing external controlplane backend tests"
    
    # Set additional environment variables for external controlplane
    export KIALI_BINARY="${KIALI_BINARY_PATH}"
    export TEST_TIMEOUT="${timeout}"
    export EXTERNAL_CONTROLPLANE="true"
    
    # Get controlplane and dataplane contexts
    local controlplane_context
    controlplane_context=$(get_cluster_context "controlplane" "${CLUSTER_TYPE}")
    local dataplane_context
    dataplane_context=$(get_cluster_context "dataplane" "${CLUSTER_TYPE}")
    
    export CONTROLPLANE_CONTEXT="${controlplane_context}"
    export DATAPLANE_CONTEXT="${dataplane_context}"
    
    info "Controlplane context: ${controlplane_context}"
    info "Dataplane context: ${dataplane_context}"
    
    # Run external controlplane specific tests
    info "Running external controlplane integration tests"
    
    local test_output
    test_output=$(create_temp_file "external-controlplane-test-output")
    
    # Run tests with external controlplane configuration
    if timeout "${timeout}" make test-integration-external-controlplane 2>&1 | tee "${test_output}"; then
        info "External controlplane tests PASSED"
        display_backend_test_summary "${test_output}"
        return 0
    else
        local exit_code=$?
        error "External controlplane tests FAILED (exit code: ${exit_code})"
        display_backend_test_failures "${test_output}"
        return ${exit_code}
    fi
}

# Display backend test summary
display_backend_test_summary() {
    local test_output="$1"
    
    info "=== Backend Test Summary ==="
    
    # Extract test results
    if [[ -f "${test_output}" ]]; then
        # Look for Go test output patterns
        local passed_tests
        passed_tests=$(grep -c "PASS:" "${test_output}" 2>/dev/null || echo "0")
        local failed_tests
        failed_tests=$(grep -c "FAIL:" "${test_output}" 2>/dev/null || echo "0")
        local total_tests=$((passed_tests + failed_tests))
        
        info "Total tests: ${total_tests}"
        info "Passed: ${passed_tests}"
        info "Failed: ${failed_tests}"
        
        # Extract test duration
        local duration
        duration=$(grep "^DONE" "${test_output}" | tail -1 | awk '{print $NF}' 2>/dev/null || echo "unknown")
        info "Duration: ${duration}"
        
        # Show coverage if available
        local coverage
        coverage=$(grep "coverage:" "${test_output}" | tail -1 | awk '{print $2}' 2>/dev/null || echo "")
        if [[ -n "${coverage}" ]]; then
            info "Coverage: ${coverage}"
        fi
    fi
    
    info "=========================="
}

# Display backend test failures
display_backend_test_failures() {
    local test_output="$1"
    
    error "=== Backend Test Failures ==="
    
    if [[ -f "${test_output}" ]]; then
        # Show failed test details
        if grep -q "FAIL:" "${test_output}"; then
            error "Failed tests:"
            grep "FAIL:" "${test_output}" | while read -r line; do
                error "  ${line}"
            done
        fi
        
        # Show panic or error messages
        if grep -q "panic:" "${test_output}"; then
            error "Panic messages:"
            grep -A 5 "panic:" "${test_output}" | while read -r line; do
                error "  ${line}"
            done
        fi
        
        # Show timeout messages
        if grep -q "timeout" "${test_output}"; then
            error "Timeout messages:"
            grep "timeout" "${test_output}" | while read -r line; do
                error "  ${line}"
            done
        fi
        
        # Show last few lines of output
        error "Last 20 lines of test output:"
        tail -20 "${test_output}" | while read -r line; do
            error "  ${line}"
        done
    fi
    
    error "============================="
}

# Run specific backend test
run_specific_backend_test() {
    local test_name="$1"
    local timeout="${2:-300s}"
    
    info "Running specific backend test: ${test_name}"
    
    # Change to Kiali source directory
    local kiali_source_dir="${SCRIPT_DIR}/.."
    cd "${kiali_source_dir}" || {
        error "Cannot change to Kiali source directory: ${kiali_source_dir}"
        return 1
    }
    
    # Set environment variables
    export KIALI_BINARY="${KIALI_BINARY_PATH}"
    export TEST_TIMEOUT="${timeout}"
    
    # Run the specific test
    if timeout "${timeout}" go test -v "./tests/integration/${test_name}"; then
        info "Test PASSED: ${test_name}"
        return 0
    else
        error "Test FAILED: ${test_name}"
        return 1
    fi
}

# Setup test data for backend tests
setup_backend_test_data() {
    local cluster_type="$1"
    
    info "Setting up backend test data"
    
    # Get primary cluster context
    local primary_context
    primary_context=$(get_primary_cluster_context "${cluster_type}")
    
    # Apply test workloads
    apply_backend_test_workloads "${primary_context}"
    
    # Wait for workloads to be ready
    wait_for_backend_test_workloads "${primary_context}"
    
    info "Backend test data setup completed"
}

# Apply backend test workloads
apply_backend_test_workloads() {
    local context="$1"
    
    info "Applying backend test workloads"
    
    # Create test namespace
    kubectl --context="${context}" create namespace kiali-test --dry-run=client -o yaml | \
        kubectl --context="${context}" apply -f -
    
    # Label namespace for Istio injection
    kubectl --context="${context}" label namespace kiali-test istio-injection=enabled --overwrite
    
    # Apply simple test workload
    kubectl --context="${context}" apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: kiali-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: test-app
        image: nginx:alpine
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: test-app
  namespace: kiali-test
spec:
  selector:
    app: test-app
  ports:
  - port: 80
    targetPort: 80
EOF
    
    debug "Backend test workloads applied"
}

# Wait for backend test workloads
wait_for_backend_test_workloads() {
    local context="$1"
    
    info "Waiting for backend test workloads to be ready"
    
    # Wait for deployment to be ready
    if ! kubectl --context="${context}" wait --for=condition=available deployment/test-app \
        -n kiali-test --timeout=300s; then
        warn "Test workload deployment is not ready"
        return 1
    fi
    
    # Wait for pods to be ready
    if ! kubectl --context="${context}" wait --for=condition=ready pods \
        -l app=test-app -n kiali-test --timeout=300s; then
        warn "Test workload pods are not ready"
        return 1
    fi
    
    debug "Backend test workloads are ready"
    return 0
}

# Cleanup backend test data
cleanup_backend_test_data() {
    local cluster_type="$1"
    
    info "Cleaning up backend test data"
    
    # Get primary cluster context
    local primary_context
    primary_context=$(get_primary_cluster_context "${cluster_type}")
    
    # Delete test namespace
    kubectl --context="${primary_context}" delete namespace kiali-test --ignore-not-found=true 2>/dev/null || true
    
    debug "Backend test data cleanup completed"
}

# Get backend test logs
get_backend_test_logs() {
    local context="$1"
    
    info "Getting backend test logs"
    
    # Get Kiali logs
    kubectl --context="${context}" logs -n istio-system deployment/kiali --tail=100 || true
    
    # Get test workload logs
    kubectl --context="${context}" logs -n kiali-test -l app=test-app --tail=50 || true
}

# Export backend executor functions
export -f execute_backend_tests validate_backend_test_prerequisites
export -f execute_standard_backend_tests execute_external_controlplane_tests
export -f display_backend_test_summary display_backend_test_failures
export -f run_specific_backend_test setup_backend_test_data
export -f apply_backend_test_workloads wait_for_backend_test_workloads
export -f cleanup_backend_test_data get_backend_test_logs
