#!/bin/bash
#
# Utility functions for the Kiali Integration Test Suite
#

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Log levels
readonly LOG_ERROR=1
readonly LOG_WARN=2
readonly LOG_INFO=3
readonly LOG_DEBUG=4

# Global log level (set by setup_logging)
LOG_LEVEL=${LOG_INFO}

# Logging functions
error() {
    if [[ ${LOG_LEVEL} -ge ${LOG_ERROR} ]]; then
        echo -e "${RED}[ERROR]${NC} $*" >&2
    fi
}

warn() {
    if [[ ${LOG_LEVEL} -ge ${LOG_WARN} ]]; then
        echo -e "${YELLOW}[WARN]${NC} $*" >&2
    fi
}

info() {
    if [[ ${LOG_LEVEL} -ge ${LOG_INFO} ]]; then
        echo -e "${GREEN}[INFO]${NC} $*"
    fi
}

debug() {
    if [[ ${LOG_LEVEL} -ge ${LOG_DEBUG} ]]; then
        echo -e "${BLUE}[DEBUG]${NC} $*"
    fi
}

# Setup logging based on debug flag
setup_logging() {
    local debug_flag="${1:-false}"
    
    if [[ "${debug_flag}" == "true" ]]; then
        LOG_LEVEL=${LOG_DEBUG}
        set -x  # Enable bash debug output
    else
        LOG_LEVEL=${LOG_INFO}
    fi
    
    export LOG_LEVEL
    debug "Debug logging enabled"
}

# Array utility functions
array_contains() {
    local element="$1"
    shift
    local array=("$@")
    
    for item in "${array[@]}"; do
        if [[ "${item}" == "${element}" ]]; then
            return 0
        fi
    done
    return 1
}

# Boolean validation
validate_boolean() {
    local var_name="$1"
    local value="$2"
    
    if [[ "${value}" != "true" && "${value}" != "false" ]]; then
        error "Invalid boolean value for ${var_name}: ${value} (must be 'true' or 'false')"
        return 1
    fi
    return 0
}

# Wait for condition with timeout
wait_for_condition() {
    local description="$1"
    local condition_cmd="$2"
    local timeout="${3:-300}"
    local interval="${4:-5}"
    
    info "Waiting for: ${description}"
    debug "Condition command: ${condition_cmd}"
    debug "Timeout: ${timeout}s, Interval: ${interval}s"
    
    local elapsed=0
    while [[ ${elapsed} -lt ${timeout} ]]; do
        if eval "${condition_cmd}"; then
            info "Condition met: ${description}"
            return 0
        fi
        
        sleep "${interval}"
        elapsed=$((elapsed + interval))
        
        if [[ $((elapsed % 30)) -eq 0 ]]; then
            info "Still waiting for: ${description} (${elapsed}s elapsed)"
        fi
    done
    
    error "Timeout waiting for: ${description} (${elapsed}s elapsed)"
    return 1
}

# Retry function with exponential backoff
retry_with_backoff() {
    local description="$1"
    local command="$2"
    local max_attempts="${3:-3}"
    local base_delay="${4:-2}"
    
    local attempt=1
    local delay=${base_delay}
    
    while [[ ${attempt} -le ${max_attempts} ]]; do
        info "Attempt ${attempt}/${max_attempts}: ${description}"
        
        if eval "${command}"; then
            info "Success: ${description}"
            return 0
        fi
        
        if [[ ${attempt} -lt ${max_attempts} ]]; then
            warn "Failed attempt ${attempt}/${max_attempts}: ${description}"
            info "Retrying in ${delay} seconds..."
            sleep "${delay}"
            delay=$((delay * 2))
        fi
        
        ((attempt++))
    done
    
    error "Failed after ${max_attempts} attempts: ${description}"
    return 1
}

# File and directory utilities
ensure_directory() {
    local dir="$1"
    if [[ ! -d "${dir}" ]]; then
        debug "Creating directory: ${dir}"
        mkdir -p "${dir}"
    fi
}

# Temporary file management
TEMP_FILES=()
create_temp_file() {
    local prefix="${1:-kiali-test}"
    local temp_file
    temp_file=$(mktemp -t "${prefix}.XXXXXX")
    TEMP_FILES+=("${temp_file}")
    echo "${temp_file}"
}

cleanup_temp_files() {
    for temp_file in "${TEMP_FILES[@]}"; do
        if [[ -f "${temp_file}" ]]; then
            debug "Removing temp file: ${temp_file}"
            rm -f "${temp_file}"
        fi
    done
    TEMP_FILES=()
}

# Cleanup trap setup
CLEANUP_FUNCTIONS=()
add_cleanup_function() {
    local func="$1"
    CLEANUP_FUNCTIONS+=("${func}")
}

cleanup_trap() {
    debug "Running cleanup functions..."
    
    # Run cleanup functions in reverse order
    for ((i=${#CLEANUP_FUNCTIONS[@]}-1; i>=0; i--)); do
        local func="${CLEANUP_FUNCTIONS[i]}"
        debug "Running cleanup function: ${func}"
        eval "${func}" || warn "Cleanup function failed: ${func}"
    done
    
    # Clean up temp files
    cleanup_temp_files
    
    debug "Cleanup completed"
}

setup_cleanup_trap() {
    trap cleanup_trap EXIT INT TERM
    add_cleanup_function cleanup_temp_files
}

# Process management
kill_process_tree() {
    local pid="$1"
    local signal="${2:-TERM}"
    
    if [[ -z "${pid}" ]]; then
        return 0
    fi
    
    debug "Killing process tree for PID ${pid} with signal ${signal}"
    
    # Kill child processes first
    local children
    children=$(pgrep -P "${pid}" 2>/dev/null || true)
    for child in ${children}; do
        kill_process_tree "${child}" "${signal}"
    done
    
    # Kill the main process
    if kill -0 "${pid}" 2>/dev/null; then
        kill -"${signal}" "${pid}" 2>/dev/null || true
        
        # Wait for process to die
        local count=0
        while kill -0 "${pid}" 2>/dev/null && [[ ${count} -lt 10 ]]; do
            sleep 1
            ((count++))
        done
        
        # Force kill if still alive
        if kill -0 "${pid}" 2>/dev/null; then
            warn "Process ${pid} didn't respond to ${signal}, force killing"
            kill -KILL "${pid}" 2>/dev/null || true
        fi
    fi
}

# Background process management
BACKGROUND_PIDS=()
start_background_process() {
    local description="$1"
    local command="$2"
    
    info "Starting background process: ${description}"
    debug "Command: ${command}"
    
    eval "${command}" &
    local pid=$!
    BACKGROUND_PIDS+=("${pid}")
    
    debug "Started background process: ${description} (PID: ${pid})"
    echo "${pid}"
}

cleanup_background_processes() {
    for pid in "${BACKGROUND_PIDS[@]}"; do
        if kill -0 "${pid}" 2>/dev/null; then
            debug "Stopping background process: ${pid}"
            kill_process_tree "${pid}"
        fi
    done
    BACKGROUND_PIDS=()
}

# Add background process cleanup to cleanup functions
add_cleanup_function cleanup_background_processes

# Kubernetes utilities
kubectl_wait_for_resource() {
    local resource="$1"
    local condition="$2"
    local timeout="${3:-300s}"
    local namespace="${4:-}"
    
    local kubectl_cmd="kubectl wait --for=${condition} ${resource} --timeout=${timeout}"
    if [[ -n "${namespace}" ]]; then
        kubectl_cmd="${kubectl_cmd} --namespace=${namespace}"
    fi
    
    debug "Waiting for Kubernetes resource: ${kubectl_cmd}"
    
    if ${kubectl_cmd}; then
        debug "Kubernetes resource ready: ${resource}"
        return 0
    else
        error "Kubernetes resource not ready: ${resource}"
        return 1
    fi
}

# URL utilities
is_url_accessible() {
    local url="$1"
    local timeout="${2:-10}"
    local expected_status="${3:-200}"
    
    debug "Checking URL accessibility: ${url}"
    
    local status_code
    status_code=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout "${timeout}" "${url}" 2>/dev/null || echo "000")
    
    if [[ "${status_code}" == "${expected_status}" ]]; then
        debug "URL accessible: ${url} (status: ${status_code})"
        return 0
    else
        debug "URL not accessible: ${url} (status: ${status_code}, expected: ${expected_status})"
        return 1
    fi
}

# YAML/JSON utilities (requires yq)
get_yaml_value() {
    local file="$1"
    local path="$2"
    local default_value="${3:-}"
    
    if [[ ! -f "${file}" ]]; then
        echo "${default_value}"
        return 1
    fi
    
    local value
    if command -v yq >/dev/null 2>&1; then
        value=$(yq eval "${path}" "${file}" 2>/dev/null || echo "${default_value}")
        if [[ "${value}" == "null" ]]; then
            echo "${default_value}"
        else
            echo "${value}"
        fi
    else
        warn "yq not found, cannot parse YAML file: ${file}"
        echo "${default_value}"
        return 1
    fi
}

# Environment detection
is_ci_environment() {
    [[ "${CI:-false}" == "true" ]] || [[ -n "${GITHUB_ACTIONS:-}" ]]
}

is_local_environment() {
    ! is_ci_environment
}

# Platform detection
get_platform() {
    local os
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch
    arch=$(uname -m)
    
    case "${arch}" in
        x86_64) arch="amd64" ;;
        aarch64) arch="arm64" ;;
        armv7l) arch="arm" ;;
    esac
    
    echo "${os}-${arch}"
}

# Version comparison
version_compare() {
    local version1="$1"
    local version2="$2"
    
    # Remove 'v' prefix if present
    version1=${version1#v}
    version2=${version2#v}
    
    # Use sort -V for version comparison
    local result
    result=$(printf '%s\n%s\n' "${version1}" "${version2}" | sort -V | head -n1)
    
    if [[ "${result}" == "${version1}" ]]; then
        if [[ "${version1}" == "${version2}" ]]; then
            echo "0"  # Equal
        else
            echo "-1" # version1 < version2
        fi
    else
        echo "1"      # version1 > version2
    fi
}

# Resource monitoring
monitor_resource_usage() {
    local namespace="${1:-}"
    local interval="${2:-30}"
    local duration="${3:-300}"
    
    info "Monitoring resource usage (interval: ${interval}s, duration: ${duration}s)"
    
    local end_time=$((SECONDS + duration))
    while [[ ${SECONDS} -lt ${end_time} ]]; do
        debug "=== Resource Usage ($(date)) ==="
        
        if [[ -n "${namespace}" ]]; then
            kubectl top pods -n "${namespace}" 2>/dev/null | debug || debug "kubectl top pods failed"
        else
            kubectl top nodes 2>/dev/null | debug || debug "kubectl top nodes failed"
        fi
        
        sleep "${interval}"
    done
}

# Export all functions
export -f error warn info debug
export -f setup_logging
export -f array_contains validate_boolean
export -f wait_for_condition retry_with_backoff
export -f ensure_directory create_temp_file cleanup_temp_files
export -f setup_cleanup_trap cleanup_trap add_cleanup_function
export -f kill_process_tree start_background_process cleanup_background_processes
export -f kubectl_wait_for_resource is_url_accessible
export -f get_yaml_value is_ci_environment is_local_environment
export -f get_platform version_compare monitor_resource_usage
