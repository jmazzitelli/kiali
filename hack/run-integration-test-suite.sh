#!/bin/bash
#
# Kiali Integration Test Suite
# Unified entry point for all Kiali integration testing scenarios
#
# This script provides a single interface for running all types of Kiali
# integration tests with support for different cluster types (KinD, Minikube)
# and various test configurations (single-cluster, multi-cluster, etc.).
#

set -euo pipefail

# Script directory and source core libraries
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INTEGRATION_TEST_DIR="${SCRIPT_DIR}/integration-test-suite"

# Source core libraries
source "${INTEGRATION_TEST_DIR}/lib/utils.sh"
source "${INTEGRATION_TEST_DIR}/lib/validation.sh"
source "${INTEGRATION_TEST_DIR}/lib/test-suite-runner.sh"

# Default values
DEFAULT_CLUSTER_TYPE="kind"
DEFAULT_SETUP_ONLY="false"
DEFAULT_TESTS_ONLY="false"
DEFAULT_CLEANUP="true"
DEFAULT_DEBUG="false"
DEFAULT_TIMEOUT="3600"
DEFAULT_PARALLEL_SETUP="true"

# Global variables
TEST_SUITE=""
CLUSTER_TYPE="${DEFAULT_CLUSTER_TYPE}"
ISTIO_VERSION=""
SETUP_ONLY="${DEFAULT_SETUP_ONLY}"
TESTS_ONLY="${DEFAULT_TESTS_ONLY}"
CLEANUP="${DEFAULT_CLEANUP}"
DEBUG="${DEFAULT_DEBUG}"
TIMEOUT="${DEFAULT_TIMEOUT}"
PARALLEL_SETUP="${DEFAULT_PARALLEL_SETUP}"

# Usage information
usage() {
    cat << EOF
Usage: $0 --test-suite <suite> --cluster-type <type> [OPTIONS]

Unified Kiali integration test suite runner supporting both CI and local environments.

REQUIRED ARGUMENTS:
  --test-suite <suite>           Test suite to run:
                                   backend
                                   backend-external-controlplane
                                   frontend
                                   frontend-ambient
                                   frontend-multicluster-primary-remote
                                   frontend-multicluster-multi-primary
                                   frontend-external-kiali
                                   frontend-tempo
                                   local

  --cluster-type <type>          Kubernetes cluster type:
                                   kind     (for CI environments)
                                   minikube (for local development)

OPTIONAL ARGUMENTS:
  --istio-version <version>      Istio version (format: "#.#.#" or "#.#-dev")
  --setup-only <true|false>      Only setup environment, skip tests (default: ${DEFAULT_SETUP_ONLY})
  --tests-only <true|false>      Only run tests, skip setup (default: ${DEFAULT_TESTS_ONLY})
  --cleanup <true|false>         Clean up after tests (default: ${DEFAULT_CLEANUP})
  --debug <true|false>           Enable debug output (default: ${DEFAULT_DEBUG})
  --timeout <seconds>            Test timeout in seconds (default: ${DEFAULT_TIMEOUT})
  --parallel-setup <true|false>  Enable parallel setup (default: ${DEFAULT_PARALLEL_SETUP})

EXAMPLES:
  # Run backend tests with KinD
  $0 --test-suite backend --cluster-type kind

  # Run frontend tests with Minikube, setup only
  $0 --test-suite frontend --cluster-type minikube --setup-only true

  # Run multicluster tests with specific Istio version
  $0 --test-suite frontend-multicluster-primary-remote --cluster-type kind --istio-version 1.20.0

  # Local development with debug enabled
  $0 --test-suite frontend --cluster-type minikube --debug true --cleanup false

EOF
}

# Parse command line arguments
parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --test-suite)
                TEST_SUITE="$2"
                shift 2
                ;;
            --cluster-type)
                CLUSTER_TYPE="$2"
                shift 2
                ;;
            --istio-version)
                ISTIO_VERSION="$2"
                shift 2
                ;;
            --setup-only)
                SETUP_ONLY="$2"
                shift 2
                ;;
            --tests-only)
                TESTS_ONLY="$2"
                shift 2
                ;;
            --cleanup)
                CLEANUP="$2"
                shift 2
                ;;
            --debug)
                DEBUG="$2"
                shift 2
                ;;
            --timeout)
                TIMEOUT="$2"
                shift 2
                ;;
            --parallel-setup)
                PARALLEL_SETUP="$2"
                shift 2
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                error "Unknown argument: $1"
                usage
                exit 1
                ;;
        esac
    done
}

# Validate required arguments
validate_arguments() {
    if [[ -z "${TEST_SUITE}" ]]; then
        error "Missing required argument: --test-suite"
        usage
        exit 1
    fi

    if [[ -z "${CLUSTER_TYPE}" ]]; then
        error "Missing required argument: --cluster-type"
        usage
        exit 1
    fi

    # Validate test suite
    local valid_suites=(
        "backend"
        "backend-external-controlplane"
        "frontend"
        "frontend-ambient"
        "frontend-multicluster-primary-remote"
        "frontend-multicluster-multi-primary"
        "frontend-external-kiali"
        "frontend-tempo"
        "local"
    )

    if ! array_contains "${TEST_SUITE}" "${valid_suites[@]}"; then
        error "Invalid test suite: ${TEST_SUITE}"
        info "Valid test suites: ${valid_suites[*]}"
        exit 1
    fi

    # Validate cluster type
    local valid_cluster_types=("kind" "minikube")
    if ! array_contains "${CLUSTER_TYPE}" "${valid_cluster_types[@]}"; then
        error "Invalid cluster type: ${CLUSTER_TYPE}"
        info "Valid cluster types: ${valid_cluster_types[*]}"
        exit 1
    fi

    # Validate boolean arguments
    validate_boolean "SETUP_ONLY" "${SETUP_ONLY}"
    validate_boolean "TESTS_ONLY" "${TESTS_ONLY}"
    validate_boolean "CLEANUP" "${CLEANUP}"
    validate_boolean "DEBUG" "${DEBUG}"
    validate_boolean "PARALLEL_SETUP" "${PARALLEL_SETUP}"

    # Validate timeout
    if ! [[ "${TIMEOUT}" =~ ^[0-9]+$ ]] || [[ "${TIMEOUT}" -lt 60 ]]; then
        error "Invalid timeout: ${TIMEOUT} (must be >= 60 seconds)"
        exit 1
    fi

    # Validate conflicting options
    if [[ "${SETUP_ONLY}" == "true" && "${TESTS_ONLY}" == "true" ]]; then
        error "Cannot specify both --setup-only and --tests-only"
        exit 1
    fi
}

# Setup logging and cleanup trap
setup_environment() {
    # Setup logging
    setup_logging "${DEBUG}"

    # Setup cleanup trap
    setup_cleanup_trap

    # Export global variables for use by other scripts
    export TEST_SUITE
    export CLUSTER_TYPE
    export ISTIO_VERSION
    export SETUP_ONLY
    export TESTS_ONLY
    export CLEANUP
    export DEBUG
    export TIMEOUT
    export PARALLEL_SETUP
    export INTEGRATION_TEST_DIR
    export SCRIPT_DIR

    info "=== Kiali Integration Test Suite ==="
    info "Test Suite: ${TEST_SUITE}"
    info "Cluster Type: ${CLUSTER_TYPE}"
    info "Istio Version: ${ISTIO_VERSION:-latest}"
    info "Setup Only: ${SETUP_ONLY}"
    info "Tests Only: ${TESTS_ONLY}"
    info "Cleanup: ${CLEANUP}"
    info "Debug: ${DEBUG}"
    info "Timeout: ${TIMEOUT}s"
    info "Parallel Setup: ${PARALLEL_SETUP}"
    info "======================================="
}

# Main execution function
main() {
    # Parse and validate arguments
    parse_arguments "$@"
    validate_arguments

    # Setup environment
    setup_environment

    # Validate prerequisites
    validate_prerequisites "${CLUSTER_TYPE}"

    # Run the test suite
    run_test_suite "${TEST_SUITE}" "${CLUSTER_TYPE}"

    info "Integration test suite completed successfully"
}

# Execute main function
main "$@"
