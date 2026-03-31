#!/bin/bash
#
# gwapi-kiali-demo.sh — Gateway API + Kiali Observability Demo
#
# For OSSM-12190: exploring what metrics the OpenShift ingress Gateway API
# produces and how Kiali can observe them — without a full Istio mesh.
#
# Installs:
#   - OSSM 3.x operator (provides Istio/Sail CRDs needed by the Ingress Operator)
#   - GatewayClass + Gateway (triggers a lightweight Istio in openshift-ingress)
#   - Plain demo app (no sidecars) + HTTPRoute
#   - User workload monitoring (enables it if not already active)
#   - PodMonitor in openshift-ingress so the *platform* Prometheus scrapes
#     Istio/Envoy metrics from the gateway proxy.  This is an NE-1113
#     workaround — see https://redhat.atlassian.net/browse/NE-1113
#   - Kiali operator + CR configured to query thanos-querier via bearer token
#   - ClusterRoleBinding (cluster-monitoring-view) so Kiali's service account
#     can read metrics from thanos-querier
#
# Usage:
#   ./gwapi-kiali-demo.sh install      Install all components
#   ./gwapi-kiali-demo.sh traffic      Generate clean traffic (try: traffic --help)
#   ./gwapi-kiali-demo.sh scrape       Dump raw Envoy metrics from the gateway proxy
#   ./gwapi-kiali-demo.sh prom        Query Prometheus for gateway metrics (alias: prometheus)
#   ./gwapi-kiali-demo.sh status       Show status of all components
#   ./gwapi-kiali-demo.sh urls         Print access URLs
#   ./gwapi-kiali-demo.sh uninstall    Remove all components
#
# Requirements:
#   - oc CLI logged into an OpenShift 4.19+ cluster
#   - Cluster monitoring must be enabled
#

# ── Configuration ──────────────────────────────────────────────────────────────

APP_NAMESPACE="gwapi-demo"
KIALI_NAMESPACE="istio-system"
INGRESS_NAMESPACE="openshift-ingress"
MONITORING_NAMESPACE="openshift-monitoring"
GATEWAY_NAME="demo-gateway"
GATEWAY_CLASS_NAME="openshift-default"
HTTPROUTE_NAME="demo-route"
APP_NAME="demo-app"
KIALI_NAME="kiali"

CLUSTER_DOMAIN=""
GATEWAY_HOSTNAME=""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# ── Utility Functions ──────────────────────────────────────────────────────────

info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[ OK ]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()   { echo -e "${RED}[ERR ]${NC} $*"; }
header(){ echo -e "\n${BOLD}=== $* ===${NC}"; }
die()   { err "$@"; exit 1; }

wait_for_deployment() {
    local name="$1" ns="$2" timeout="${3:-300}" elapsed=0
    info "Waiting for deployment $name in $ns (up to ${timeout}s)..."
    while [[ $elapsed -lt $timeout ]]; do
        local ready
        ready=$(oc get deployment "$name" -n "$ns" \
            -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "")
        if [[ "$ready" =~ ^[0-9]+$ ]] && [[ "$ready" -ge 1 ]]; then
            ok "Deployment $name is ready"
            return 0
        fi
        sleep 5
        elapsed=$((elapsed + 5))
    done
    err "Timeout waiting for deployment $name in $ns"
    return 1
}

wait_for_csv() {
    local prefix="$1" ns="$2" timeout="${3:-600}" elapsed=0
    info "Waiting for operator CSV matching '$prefix' (up to ${timeout}s)..."
    while [[ $elapsed -lt $timeout ]]; do
        local phase
        phase=$(oc get csv -n "$ns" --no-headers 2>/dev/null \
            | grep "$prefix" | awk '{print $NF}' | head -1)
        if [[ "$phase" == "Succeeded" ]]; then
            ok "Operator $prefix is ready"
            return 0
        fi
        sleep 10
        elapsed=$((elapsed + 10))
        if (( elapsed % 60 == 0 )); then
            info "  Still waiting... (${elapsed}s elapsed, phase=${phase:-not found})"
        fi
    done
    err "Timeout waiting for operator $prefix"
    return 1
}

wait_for_crd() {
    local crd="$1" timeout="${2:-300}" elapsed=0
    info "Waiting for CRD $crd (up to ${timeout}s)..."
    while [[ $elapsed -lt $timeout ]]; do
        if oc get crd "$crd" &>/dev/null; then
            ok "CRD $crd exists"
            return 0
        fi
        sleep 5
        elapsed=$((elapsed + 5))
    done
    err "Timeout waiting for CRD $crd"
    return 1
}

report_crds() {
    local crds
    crds=$(oc get crd --no-headers 2>/dev/null \
        | grep -E 'istio|sail|gateway|kiali' | awk '{print $1}')
    if [[ -z "$crds" ]]; then
        echo "  (none found)"
        return
    fi
    while read -r crd; do
        local owner="" created
        created=$(oc get crd "$crd" -o jsonpath='{.metadata.creationTimestamp}' 2>/dev/null)
        local olm_labels
        olm_labels=$(oc get crd "$crd" -o json 2>/dev/null \
            | python3 -c "
import sys, json
labels = json.load(sys.stdin).get('metadata',{}).get('labels',{})
owners = [k.split('/')[1] for k in labels if k.startswith('operators.coreos.com/')]
print(', '.join(owners) if owners else '')
" 2>/dev/null)
        if [[ -n "$olm_labels" ]]; then
            owner="installed by OLM operator: $olm_labels"
        elif [[ "$crd" == *.gateway.networking.k8s.io ]]; then
            owner="Gateway API (likely shipped with OpenShift)"
        elif [[ "$crd" == *.sailoperator.io ]]; then
            owner="OSSM/Sail operator"
        elif [[ "$crd" == *.kiali.io ]]; then
            owner="Kiali operator"
        elif [[ "$crd" == *.istio.io ]]; then
            owner="Istio (installed by OSSM)"
        fi
        echo "    $crd"
        echo "      created: $created | $owner"
    done <<< "$crds"
}

detect_cluster_domain() {
    CLUSTER_DOMAIN=$(oc get ingresses.config.openshift.io cluster \
        -o jsonpath='{.spec.domain}' 2>/dev/null)
    [[ -z "$CLUSTER_DOMAIN" ]] && die "Cannot detect cluster domain"
    GATEWAY_HOSTNAME="demo.gwapi.${CLUSTER_DOMAIN}"
}

check_prerequisites() {
    info "Checking prerequisites..."
    command -v oc &>/dev/null || die "'oc' CLI not found in PATH"
    oc whoami &>/dev/null     || die "Not logged into an OpenShift cluster"

    local user version
    user=$(oc whoami)
    version=$(oc get clusterversion version \
        -o jsonpath='{.status.desired.version}' 2>/dev/null || echo "unknown")
    detect_cluster_domain

    info "Cluster: $version | User: $user | Domain: $CLUSTER_DOMAIN"

    if oc get co monitoring &>/dev/null; then
        ok "Cluster monitoring is available"
    else
        die "Cluster monitoring is NOT available. It must be enabled before running this script."
    fi

    ok "Prerequisites passed"
}

# ── Install Functions ──────────────────────────────────────────────────────────

install_ossm_operator() {
    header "OSSM 3.x Operator"

    if oc get csv -n openshift-operators --no-headers 2>/dev/null \
         | grep -q "servicemeshoperator3"; then
        warn "OSSM 3.x operator already installed — skipping"
        return 0
    fi

    info "Creating OSSM operator subscription..."
    oc apply -f - <<'YAML'
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: servicemeshoperator3
  namespace: openshift-operators
spec:
  channel: stable
  installPlanApproval: Automatic
  name: servicemeshoperator3
  source: redhat-operators
  sourceNamespace: openshift-marketplace
YAML

    wait_for_csv "servicemeshoperator3" "openshift-operators" 900
}

ensure_gateway_api_crds() {
    header "Gateway API CRDs"

    if oc get crd gatewayclasses.gateway.networking.k8s.io &>/dev/null; then
        ok "Gateway API CRDs already present"
        return 0
    fi

    info "Gateway API CRDs not found — installing from upstream..."
    oc apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml
    wait_for_crd "gatewayclasses.gateway.networking.k8s.io" 60
}

create_gateway_class() {
    header "GatewayClass"

    if oc get gatewayclass "$GATEWAY_CLASS_NAME" &>/dev/null; then
        warn "GatewayClass $GATEWAY_CLASS_NAME already exists — skipping"
    else
        oc apply -f - <<YAML
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: ${GATEWAY_CLASS_NAME}
spec:
  controllerName: openshift.io/gateway-controller/v1
YAML
    fi

    info "Waiting for the Ingress Operator to provision the gateway Istio..."
    info "(This triggers OSSM installation in openshift-ingress — may take several minutes)"
    wait_for_deployment "istiod-openshift-gateway" "$INGRESS_NAMESPACE" 900
}

create_gateway() {
    header "Gateway"

    oc apply -f - <<YAML
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ${GATEWAY_NAME}
  namespace: ${INGRESS_NAMESPACE}
spec:
  gatewayClassName: ${GATEWAY_CLASS_NAME}
  listeners:
  - name: http
    hostname: "*.gwapi.${CLUSTER_DOMAIN}"
    port: 80
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: All
YAML

    local deploy_name="${GATEWAY_NAME}-${GATEWAY_CLASS_NAME}"
    wait_for_deployment "$deploy_name" "$INGRESS_NAMESPACE" 300
}

deploy_demo_app() {
    header "Demo Application (no sidecar)"

    oc create namespace "$APP_NAMESPACE" --dry-run=client -o yaml | oc apply -f -

    oc apply -f - <<YAML
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${APP_NAME}-content
  namespace: ${APP_NAMESPACE}
data:
  index.html: |
    <!DOCTYPE html>
    <html><body>
    <h1>Gateway API Demo</h1>
    <p>Traffic reached the backend through the OpenShift ingress gateway.</p>
    </body></html>
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${APP_NAME}
  namespace: ${APP_NAMESPACE}
  labels:
    app: ${APP_NAME}
    version: v1
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ${APP_NAME}
      version: v1
  template:
    metadata:
      labels:
        app: ${APP_NAME}
        version: v1
    spec:
      containers:
      - name: httpd
        image: registry.access.redhat.com/ubi9/httpd-24:latest
        ports:
        - containerPort: 8080
          name: http
        resources:
          requests:
            cpu: 10m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
        volumeMounts:
        - name: content
          mountPath: /var/www/html
      volumes:
      - name: content
        configMap:
          name: ${APP_NAME}-content
---
apiVersion: v1
kind: Service
metadata:
  name: ${APP_NAME}
  namespace: ${APP_NAMESPACE}
  labels:
    app: ${APP_NAME}
spec:
  selector:
    app: ${APP_NAME}
  ports:
  - name: http
    port: 8080
    targetPort: 8080
YAML

    wait_for_deployment "$APP_NAME" "$APP_NAMESPACE" 300
}

create_httproute() {
    header "HTTPRoute"

    oc apply -f - <<YAML
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ${HTTPROUTE_NAME}
  namespace: ${APP_NAMESPACE}
spec:
  parentRefs:
  - name: ${GATEWAY_NAME}
    namespace: ${INGRESS_NAMESPACE}
  hostnames:
  - "${GATEWAY_HOSTNAME}"
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /
    backendRefs:
    - name: ${APP_NAME}
      port: 8080
YAML

    ok "HTTPRoute $HTTPROUTE_NAME created"
}

test_gateway() {
    header "Gateway Connectivity Test"

    local gateway_svc
    gateway_svc=$(oc get svc -n "$INGRESS_NAMESPACE" \
        -l "gateway.networking.k8s.io/gateway-name=$GATEWAY_NAME" \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    if [[ -z "$gateway_svc" ]]; then
        warn "Gateway service not found — skipping connectivity test"
        return 0
    fi

    info "Port-forwarding to $gateway_svc..."
    oc port-forward -n "$INGRESS_NAMESPACE" "svc/$gateway_svc" 18080:80 &>/dev/null &
    local pf_pid=$!
    sleep 3

    if ! kill -0 "$pf_pid" 2>/dev/null; then
        warn "Port-forward failed — skipping connectivity test"
        return 0
    fi

    local code
    code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
        -H "Host: $GATEWAY_HOSTNAME" http://localhost:18080/ 2>/dev/null || echo "000")

    kill "$pf_pid" 2>/dev/null; wait "$pf_pid" 2>/dev/null || true

    if [[ "$code" == "200" ]]; then
        ok "Gateway returned HTTP $code — traffic flows end to end"
    else
        warn "Gateway returned HTTP $code — may need a moment to converge"
    fi
}

setup_monitoring() {
    header "Monitoring"

    info "Enabling user workload monitoring..."
    local existing
    existing=$(oc get configmap cluster-monitoring-config -n "$MONITORING_NAMESPACE" \
        -o jsonpath='{.data.config\.yaml}' 2>/dev/null || echo "")

    if echo "$existing" | grep -q "enableUserWorkload: true"; then
        ok "User workload monitoring already enabled"
    else
        oc apply -f - <<YAML
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: ${MONITORING_NAMESPACE}
data:
  config.yaml: |
    enableUserWorkload: true
YAML
        info "Waiting for user workload monitoring..."
        local elapsed=0
        while [[ $elapsed -lt 180 ]]; do
            if oc get pods -n openshift-user-workload-monitoring --no-headers 2>/dev/null \
                | grep -q "Running"; then
                ok "User workload monitoring is running"
                break
            fi
            sleep 10; elapsed=$((elapsed + 10))
        done
    fi

    setup_ne1113_gateway_metrics
}

# ── NE-1113 Workaround ───────────────────────────────────────────────────────
#
# NE-1113 (https://redhat.atlassian.net/browse/NE-1113) plans to have the
# cluster-ingress-operator ship a ServiceMonitor that scrapes Istio/Envoy
# metrics from gateway proxy pods in openshift-ingress into the platform
# Prometheus.  Until that is delivered, this function creates the equivalent
# PodMonitor.  The monitor must live in openshift-ingress so that the
# *platform* Prometheus (not user-workload) picks it up — user-workload
# monitoring cannot scrape pods in openshift-* namespaces.
#
# Once NE-1113 lands, this function (and its uninstall counterpart) can be
# removed entirely.

setup_ne1113_gateway_metrics() {
    header "Gateway Metrics Collection (NE-1113)"

    info "Creating PodMonitor in ${INGRESS_NAMESPACE} for platform Prometheus..."

    oc apply -f - <<YAML
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: gateway-istio-monitor
  namespace: ${INGRESS_NAMESPACE}
  labels:
    app.kubernetes.io/managed-by: gwapi-kiali-demo
spec:
  selector:
    matchLabels:
      gateway.networking.k8s.io/gateway-name: ${GATEWAY_NAME}
  podMetricsEndpoints:
  - port: metrics
    path: /stats/prometheus
    interval: 15s
    metricRelabelings:
    - sourceLabels: [__name__]
      regex: istio_.*|envoy_cluster_upstream_cx_active|envoy_cluster_upstream_rq_total|envoy_listener_downstream_cx_active|envoy_listener_http_downstream_rq|envoy_server_memory_allocated|envoy_server_memory_heap_size|envoy_server_uptime
      action: keep
YAML

    ok "PodMonitor gateway-istio-monitor created in ${INGRESS_NAMESPACE}"
    info "Platform Prometheus will begin scraping gateway proxy metrics"
}

install_kiali() {
    header "Kiali Operator"

    if oc get csv -n openshift-operators --no-headers 2>/dev/null \
         | grep -q "kiali"; then
        warn "Kiali operator already installed — skipping subscription"
    else
        oc apply -f - <<'YAML'
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: kiali-ossm
  namespace: openshift-operators
spec:
  channel: stable
  installPlanApproval: Automatic
  name: kiali-ossm
  source: redhat-operators
  sourceNamespace: openshift-marketplace
YAML

        info "Waiting for Kiali install plan..."
        local ip_elapsed=0
        while [[ $ip_elapsed -lt 120 ]]; do
            local ip_name
            ip_name=$(oc get installplan -n openshift-operators --no-headers 2>/dev/null \
                | grep "kiali" | grep -v "true$" | awk '{print $1}' | head -1)
            if [[ -n "$ip_name" ]]; then
                info "Approving install plan $ip_name..."
                oc patch installplan "$ip_name" -n openshift-operators \
                    --type merge -p '{"spec":{"approved":true}}' 2>/dev/null
                break
            fi
            sleep 5; ip_elapsed=$((ip_elapsed + 5))
        done

        wait_for_csv "kiali" "openshift-operators" 600
    fi

    header "Kiali CR"

    oc create namespace "$KIALI_NAMESPACE" --dry-run=client -o yaml | oc apply -f -

    oc apply -f - <<YAML
apiVersion: kiali.io/v1alpha1
kind: Kiali
metadata:
  name: ${KIALI_NAME}
  namespace: ${KIALI_NAMESPACE}
spec:
  deployment:
    cluster_wide_access: true
    discovery_selectors:
      default:
      - matchExpressions:
        - key: kubernetes.io/metadata.name
          operator: In
          values:
          - ${INGRESS_NAMESPACE}
          - ${APP_NAMESPACE}
  external_services:
    grafana:
      enabled: false
    prometheus:
      auth:
        type: bearer
        use_kiali_token: true
      thanos_proxy:
        enabled: true
      url: https://thanos-querier.openshift-monitoring.svc.cluster.local:9091
    tracing:
      enabled: false
    istio:
      gateway_api_classes:
      - class_name: ${GATEWAY_CLASS_NAME}
        name: OpenShift Default
YAML

    wait_for_deployment "$KIALI_NAME" "$KIALI_NAMESPACE" 300

    header "Kiali Monitoring Access"

    local kiali_sa="${KIALI_NAME}-service-account"
    if oc get clusterrolebinding kiali-monitoring-view &>/dev/null; then
        warn "ClusterRoleBinding kiali-monitoring-view already exists — skipping"
    else
        oc apply -f - <<CRB
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kiali-monitoring-view
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-monitoring-view
subjects:
- kind: ServiceAccount
  name: ${kiali_sa}
  namespace: ${KIALI_NAMESPACE}
CRB
        ok "Granted cluster-monitoring-view to ${kiali_sa} in ${KIALI_NAMESPACE}"
    fi
}

# ── Main Commands ──────────────────────────────────────────────────────────────

do_install() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║  Gateway API + Kiali Observability Demo         ║${NC}"
    echo -e "${BOLD}║  Ingress gateway only — no full Istio mesh      ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""

    set -e

    check_prerequisites
    install_ossm_operator
    ensure_gateway_api_crds
    create_gateway_class
    create_gateway
    deploy_demo_app
    create_httproute
    test_gateway
    setup_monitoring
    install_kiali

    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}${BOLD}║  Installation complete!                          ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
    do_urls
    echo ""
    info "Next steps:"
    info "  1. Generate traffic:  $0 traffic"
    info "  2. Raw proxy scrape:  $0 scrape"
    info "  3. Prometheus query:  $0 prom"
    info "  4. Open Kiali UI and explore the graph"
}

do_traffic() {
    local duration=10
    local rate=2
    local include_errors=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d|--duration)    duration="$2"; shift; shift ;;
            -r|--rate)        rate="$2"; shift; shift ;;
            -e|--errors)      include_errors=true; shift ;;
            -h|--help)
                echo "Usage: $(basename "$0") traffic [options]"
                echo ""
                echo "Options:"
                echo "  -d, --duration <sec>  Duration in seconds (default: 10)"
                echo "  -r, --rate <rps>      Requests per second (default: 2)"
                echo "  -e, --errors          Include requests to bad paths (404s)"
                echo ""
                echo "Examples:"
                echo "  $(basename "$0") traffic                     # 10s, 2 rps, clean traffic"
                echo "  $(basename "$0") traffic -d 60 -r 5          # 60s, 5 rps, clean traffic"
                echo "  $(basename "$0") traffic -d 30 -e            # 30s with some 404 errors"
                return 0
                ;;
            *)  die "Unknown option: $1 (try: $0 traffic --help)" ;;
        esac
    done

    detect_cluster_domain

    local gateway_svc
    gateway_svc=$(oc get svc -n "$INGRESS_NAMESPACE" \
        -l "gateway.networking.k8s.io/gateway-name=$GATEWAY_NAME" \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    [[ -z "$gateway_svc" ]] && die "Gateway service not found. Run '$0 install' first."

    local paths=("/" "/index.html")
    if [[ "$include_errors" == "true" ]]; then
        paths+=("/noexist" "/icons/" "/api/health")
        info "Generating traffic for ${duration}s at ~${rate} req/s (with error paths)"
    else
        info "Generating traffic for ${duration}s at ~${rate} req/s (clean)"
    fi
    info "Target: $GATEWAY_HOSTNAME via port-forward to $gateway_svc"

    # Kill any stale port-forward on this port from a previous run
    local stale_pf
    stale_pf=$(lsof -ti :18080 2>/dev/null || true)
    [[ -n "$stale_pf" ]] && kill $stale_pf 2>/dev/null && sleep 1

    oc port-forward -n "$INGRESS_NAMESPACE" "svc/$gateway_svc" 18080:80 &>/dev/null &
    local pf_pid=$!
    trap "kill $pf_pid 2>/dev/null; wait $pf_pid 2>/dev/null || true" EXIT
    sleep 2

    kill -0 "$pf_pid" 2>/dev/null || die "Port-forward failed to start"

    local count=0 errors=0
    local end_time=$((SECONDS + duration))
    local sleep_interval
    sleep_interval=$(awk "BEGIN {printf \"%.3f\", 1/$rate}")

    echo ""
    while [[ $SECONDS -lt $end_time ]]; do
        local idx=$((RANDOM % ${#paths[@]}))
        local code
        code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
            -H "Host: $GATEWAY_HOSTNAME" \
            "http://localhost:18080${paths[$idx]}" 2>/dev/null || echo "000")
        count=$((count + 1))
        [[ "$code" != "200" ]] && errors=$((errors + 1))
        printf "\r  Requests: %d | Non-200: %d | Last: %s | Remaining: %ds  " \
            "$count" "$errors" "$code" "$((end_time - SECONDS))"
        sleep "$sleep_interval"
    done

    echo ""
    echo ""
    ok "Traffic complete: $count requests ($errors non-200)"

    kill "$pf_pid" 2>/dev/null; wait "$pf_pid" 2>/dev/null || true
    trap - EXIT
}

do_scrape() {
    local show_kiali=true
    local show_other=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --kiali)
                [[ -z "${2:-}" || "$2" == --* ]] && die "--kiali requires a value: show|hide"
                case "$2" in
                    show) show_kiali=true ;;
                    hide) show_kiali=false ;;
                    *)    die "--kiali value must be 'show' or 'hide'" ;;
                esac
                shift; shift ;;
            --other)
                [[ -z "${2:-}" || "$2" == --* ]] && die "--other requires a value: show|hide"
                case "$2" in
                    show) show_other=true ;;
                    hide) show_other=false ;;
                    *)    die "--other value must be 'show' or 'hide'" ;;
                esac
                shift; shift ;;
            -h|--help)
                echo "Usage: $(basename "$0") scrape [options]"
                echo ""
                echo "Options:"
                echo "  --kiali <show|hide>   Show/hide Kiali-relevant metrics (default: show)"
                echo "  --other <show|hide>   Show/hide other metrics (default: hide)"
                echo ""
                echo "Examples:"
                echo "  $(basename "$0") scrape                        # Kiali-relevant only"
                echo "  $(basename "$0") scrape --other show           # both sections"
                echo "  $(basename "$0") scrape --kiali hide --other show  # other only"
                return 0
                ;;
            *)  die "Unknown option: $1 (try: $0 scrape --help)" ;;
        esac
    done

    info "Fetching raw Envoy metrics from gateway proxy..."

    local gateway_pod
    gateway_pod=$(oc get pods -n "$INGRESS_NAMESPACE" \
        -l "gateway.networking.k8s.io/gateway-name=$GATEWAY_NAME" \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    [[ -z "$gateway_pod" ]] && die "Gateway pod not found. Run '$0 install' first."

    info "Pod: $gateway_pod"
    info "Port-forwarding to metrics endpoint (15090)..."

    local metrics_tmpfile
    metrics_tmpfile=$(mktemp)

    oc port-forward -n "$INGRESS_NAMESPACE" "$gateway_pod" 15090:15090 &>/dev/null &
    local pf_pid=$!
    sleep 2

    if ! kill -0 "$pf_pid" 2>/dev/null; then
        rm -f "$metrics_tmpfile"
        die "Port-forward to metrics endpoint failed"
    fi

    curl -s http://localhost:15090/stats/prometheus > "$metrics_tmpfile" 2>/dev/null

    kill "$pf_pid" 2>/dev/null; wait "$pf_pid" 2>/dev/null || true

    local kiali_envoy="^envoy_cluster_upstream_cx_active|^envoy_cluster_upstream_rq_total|^envoy_listener_downstream_cx_active|^envoy_listener_http_downstream_rq|^envoy_server_memory_allocated|^envoy_server_memory_heap_size|^envoy_server_uptime"
    local kiali_grep="^istio_requests_total|^istio_request_duration|^istio_request_bytes|^istio_response_bytes|^istio_tcp_sent|^istio_tcp_received|${kiali_envoy}"

    local total_metrics kiali_metrics
    total_metrics=$(grep -v "^#" "$metrics_tmpfile" | grep -v "^$" \
        | cut -d'{' -f1 | cut -d' ' -f1 | sort -u | wc -l)
    kiali_metrics=$(grep -E "${kiali_grep}" "$metrics_tmpfile" \
        | cut -d'{' -f1 | cut -d' ' -f1 | sort -u | wc -l)

    local kiali_label="Hiding Kiali" other_label="Hiding Others"
    [[ "$show_kiali" == "true" ]] && kiali_label="Showing Kiali"
    [[ "$show_other" == "true" ]] && other_label="Showing Others"

    echo ""
    echo -e "${BOLD}── Summary ──${NC}"
    info "Metrics output: $kiali_label; $other_label"
    info "Total unique metric names: $total_metrics"
    info "Kiali-relevant metric names: $kiali_metrics"

    if [[ "$show_kiali" == "true" ]]; then
        echo ""
        echo -e "${BOLD}── Kiali-Relevant Metrics (istio_* + envoy_*) ──${NC}"
        grep -E "${kiali_grep}" "$metrics_tmpfile" \
            | cut -d'{' -f1 | cut -d' ' -f1 | sort -u | while read -r name; do
                local count
                count=$(grep -c "^${name}" "$metrics_tmpfile")
                echo "  $name ($count timeseries)"
            done
    fi

    if [[ "$show_other" == "true" ]]; then
        echo ""
        echo -e "${BOLD}── Other Metrics (not used by Kiali) ──${NC}"
        local kiali_names
        kiali_names=$(grep -E "${kiali_grep}" "$metrics_tmpfile" \
            | cut -d'{' -f1 | cut -d' ' -f1 | sort -u)
        local all_names
        all_names=$(grep -v "^#" "$metrics_tmpfile" | grep -v "^$" \
            | cut -d'{' -f1 | cut -d' ' -f1 | sort -u)
        local other
        other=$(comm -23 <(echo "$all_names") <(echo "$kiali_names"))
        local other_count
        other_count=$(echo "$other" | grep -c . || echo "0")
        echo "  ($other_count metrics not used by Kiali)"
        echo "$other" | while read -r name; do echo "  $name"; done
    fi

    echo ""
    echo -e "${BOLD}── Sample: istio_requests_total ──${NC}"
    grep "^istio_requests_total{" "$metrics_tmpfile" | head -5

    rm -f "$metrics_tmpfile"

    echo ""
    info "Full raw dump:"
    info "  oc port-forward -n $INGRESS_NAMESPACE $gateway_pod 15090:15090 &"
    info "  curl -s http://localhost:15090/stats/prometheus"
    echo ""
    info "Query Prometheus for stored metrics:"
    info "  $0 prom                          # list all gateway metrics in Prometheus"
    info "  $0 prom <metric_name>            # show labels + timeseries data"
    info "  $0 prom istio_requests_total     # example"
}

do_prometheus() {
    local metric_name=""
    local timeseries_count=5

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --timeseries)
                [[ -z "${2:-}" || "$2" == --* ]] && die "--timeseries requires a value: <n> or 'all'"
                timeseries_count="$2"; shift; shift ;;
            -h|--help)
                echo "Usage: $(basename "$0") prom [options] [metric_name]"
                echo ""
                echo "Options:"
                echo "  --timeseries <n|all>   Number of timeseries to show (default: 5, 0=none, all=all)"
                echo ""
                echo "Examples:"
                echo "  $(basename "$0") prom                                      # list gateway metrics"
                echo "  $(basename "$0") prom istio_requests_total                 # labels + 5 timeseries"
                echo "  $(basename "$0") prom --timeseries 10 istio_requests_total    # 10 timeseries"
                echo "  $(basename "$0") prom --timeseries all istio_requests_total   # all timeseries"
                echo "  $(basename "$0") prom --timeseries 0 istio_requests_total     # labels only"
                return 0
                ;;
            -*)  die "Unknown option: $1 (try: $0 prom --help)" ;;
            *)   metric_name="$1"; shift ;;
        esac
    done

    local thanos_url
    thanos_url=$(oc get route thanos-querier -n "$MONITORING_NAMESPACE" \
        -o jsonpath='https://{.spec.host}' 2>/dev/null)
    [[ -z "$thanos_url" ]] && die "thanos-querier route not found in $MONITORING_NAMESPACE"

    local token
    token=$(oc whoami -t 2>/dev/null)
    [[ -z "$token" ]] && die "Cannot obtain bearer token (oc whoami -t failed)"

    # Metric names Kiali cares about — must match the PodMonitor keep regex
    local envoy_names="envoy_cluster_upstream_cx_active|envoy_cluster_upstream_rq_total|envoy_listener_downstream_cx_active|envoy_listener_http_downstream_rq|envoy_server_memory_allocated|envoy_server_memory_heap_size|envoy_server_uptime"

    if [[ -z "$metric_name" ]]; then
        # ── List mode: show all gateway-related metric names in Prometheus ──
        info "Querying thanos-querier for gateway metric names..."
        local tmpfile
        tmpfile=$(mktemp)

        local http_code
        http_code=$(curl -sk -o "$tmpfile" -w '%{http_code}' \
            -H "Authorization: Bearer $token" \
            "${thanos_url}/api/v1/label/__name__/values" 2>/dev/null)

        if [[ "$http_code" != "200" ]]; then
            rm -f "$tmpfile"
            die "thanos-querier returned HTTP $http_code (expected 200)"
        fi

        local names
        names=$(python3 -c "
import sys, json, re
data = json.load(sys.stdin)
envoy_re = re.compile(r'^($envoy_names)$')
for name in sorted(data.get('data', [])):
    if name.startswith('istio_') or envoy_re.match(name):
        print(name)
" < "$tmpfile")
        rm -f "$tmpfile"

        local count
        count=$(echo "$names" | grep -c . || echo "0")

        echo ""
        if [[ "$count" -eq 0 ]]; then
            warn "No gateway metrics found in Prometheus"
            echo ""
            info "Possible causes:"
            info "  - No traffic generated yet (run: $0 traffic)"
            info "  - PodMonitor not scraped yet (wait ~15-30s after install)"
            info "  - PodMonitor missing (check: $0 status)"
        else
            echo -e "${BOLD}Gateway metrics in Prometheus ($count found):${NC}"
            echo ""
            echo "$names" | while read -r n; do echo "  $n"; done
        fi
        echo ""
    else
        # ── Detail mode: show labels and timeseries for a specific metric ──
        info "Querying thanos-querier for metric: $metric_name"
        local tmpfile
        tmpfile=$(mktemp)

        local http_code
        http_code=$(curl -sk -o "$tmpfile" -w '%{http_code}' \
            -H "Authorization: Bearer $token" \
            --data-urlencode "query=$metric_name" \
            "${thanos_url}/api/v1/query" 2>/dev/null)

        if [[ "$http_code" != "200" ]]; then
            rm -f "$tmpfile"
            die "thanos-querier returned HTTP $http_code (expected 200)"
        fi

        local result_count
        result_count=$(python3 -c "
import sys, json
data = json.load(sys.stdin)
print(len(data.get('data', {}).get('result', [])))
" < "$tmpfile")

        if [[ "$result_count" -eq 0 ]]; then
            rm -f "$tmpfile"
            echo ""
            warn "No timeseries found for '$metric_name'"
            info "Check spelling or run: $0 prom  (to list available metrics)"
            echo ""
            return 0
        fi

        echo ""
        echo -e "${BOLD}$metric_name — $result_count timeseries${NC}"
        echo ""

        echo -e "${BOLD}Labels and distinct values:${NC}"
        python3 -c "
import sys, json
data = json.load(sys.stdin)
results = data.get('data', {}).get('result', [])
skip = {'__name__', 'job', 'instance', 'container', 'endpoint', 'namespace',
        'pod', 'prometheus', 'prometheus_replica', 'uid', 'service'}
labels = {}
for r in results:
    for k, v in r.get('metric', {}).items():
        if k not in skip:
            labels.setdefault(k, set()).add(v)
for k in sorted(labels):
    vals = sorted(labels[k])
    vstr = ', '.join(vals[:10] if len(vals) <= 10 else vals[:8])
    if len(vals) <= 10:
        print(f'  {k}: {vstr}')
    else:
        print(f'  {k}: {vstr} ... ({len(vals)} total)')
" < "$tmpfile"

        if [[ "$timeseries_count" == "all" || "$timeseries_count" -gt 0 ]]; then
            local ts_label="$timeseries_count"
            [[ "$timeseries_count" == "all" ]] && ts_label="all $result_count"
            echo ""
            echo -e "${BOLD}Timeseries data ($ts_label):${NC}"
            local ts_slice="$timeseries_count"
            [[ "$timeseries_count" == "all" ]] && ts_slice=""
            python3 -c "
import sys, json
limit = '$ts_slice'
data = json.load(sys.stdin)
results = data.get('data', {}).get('result', [])
subset = results if not limit else results[:int(limit)]
for r in subset:
    m = r.get('metric', {})
    val = r.get('value', [None, ''])[1]
    lbl = ', '.join(f'{k}=\"{v}\"' for k, v in sorted(m.items()) if k != '__name__')
    name = m.get('__name__', '?')
    print(f'  {name}{{{lbl}}} {val}')
" < "$tmpfile"
        fi

        rm -f "$tmpfile"
        echo ""
    fi
}

do_status() {
    echo ""
    echo -e "${BOLD}Component Status${NC}"

    header "OSSM Operator"
    oc get csv -n openshift-operators --no-headers 2>/dev/null \
        | grep -E "servicemesh|kiali" || echo "  (not installed)"

    header "GatewayClass"
    oc get gatewayclass --no-headers 2>/dev/null || echo "  (not found)"

    header "Gateway"
    oc get gateway -n "$INGRESS_NAMESPACE" --no-headers 2>/dev/null || echo "  (not found)"

    header "openshift-ingress Pods"
    oc get pods -n "$INGRESS_NAMESPACE" --no-headers 2>/dev/null

    header "HTTPRoute"
    oc get httproute -n "$APP_NAMESPACE" --no-headers 2>/dev/null || echo "  (not found)"

    header "Demo App ($APP_NAMESPACE)"
    oc get pods -n "$APP_NAMESPACE" --no-headers 2>/dev/null || echo "  (namespace not found)"

    header "Kiali"
    oc get kiali -n "$KIALI_NAMESPACE" --no-headers 2>/dev/null || echo "  (not found)"
    oc get pods -n "$KIALI_NAMESPACE" -l app=kiali --no-headers 2>/dev/null
    echo "  Monitoring access (cluster-monitoring-view):"
    if oc get clusterrolebinding kiali-monitoring-view &>/dev/null; then
        echo "    ClusterRoleBinding kiali-monitoring-view exists"
    else
        echo "    NOT configured — Kiali cannot query thanos-querier"
    fi

    header "Monitoring"
    echo "  Cluster Monitoring:"
    if oc get co monitoring &>/dev/null; then
        echo "    enabled"
    else
        echo "    NOT enabled"
    fi
    echo "  User Workload Monitoring:"
    if oc get pods -n openshift-user-workload-monitoring --no-headers 2>/dev/null \
        | grep -q "Running"; then
        echo "    enabled"
    else
        echo "    NOT enabled"
    fi
    echo "  Gateway Metrics PodMonitor (NE-1113):"
    if oc get podmonitor gateway-istio-monitor -n "$INGRESS_NAMESPACE" &>/dev/null; then
        echo "    gateway-istio-monitor exists in $INGRESS_NAMESPACE"
    else
        echo "    NOT configured — gateway metrics not scraped into Prometheus"
    fi

    header "CRDs"
    report_crds
    echo ""
}

do_urls() {
    detect_cluster_domain 2>/dev/null || true

    local kiali_url
    kiali_url=$(oc get route kiali -n "$KIALI_NAMESPACE" \
        -o jsonpath='https://{.spec.host}' 2>/dev/null)

    local gateway_svc
    gateway_svc=$(oc get svc -n "$INGRESS_NAMESPACE" \
        -l "gateway.networking.k8s.io/gateway-name=$GATEWAY_NAME" \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

    echo ""
    echo -e "${BOLD}Access URLs${NC}"
    echo ""
    echo "  Kiali UI:       ${kiali_url:-(not installed)}"
    echo "  Gateway host:   ${GATEWAY_HOSTNAME:-unknown}"
    echo ""
    if [[ -n "$gateway_svc" ]]; then
        echo "  To reach the demo app through the gateway:"
        echo "    oc port-forward -n $INGRESS_NAMESPACE svc/$gateway_svc 8080:80 &"
        echo "    curl -H 'Host: ${GATEWAY_HOSTNAME:-HOSTNAME}' http://localhost:8080/"
    else
        echo "  Gateway service not found. Run '$0 install' first."
    fi
    echo ""
}

do_uninstall() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║  Uninstalling Gateway API + Kiali Demo           ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""

    set +e

    info "Removing Kiali monitoring access..."
    oc delete clusterrolebinding kiali-monitoring-view --ignore-not-found 2>/dev/null

    info "Removing Kiali CR..."
    oc delete kiali "$KIALI_NAME" -n "$KIALI_NAMESPACE" --ignore-not-found --timeout=60s 2>/dev/null
    sleep 5

    info "Removing Kiali operator..."
    oc delete subscription kiali-ossm -n openshift-operators --ignore-not-found 2>/dev/null
    local csv
    csv=$(oc get csv -n openshift-operators --no-headers 2>/dev/null \
        | grep kiali | awk '{print $1}')
    [[ -n "$csv" ]] && oc delete csv "$csv" -n openshift-operators 2>/dev/null

    info "Removing Kiali namespace..."
    oc delete namespace "$KIALI_NAMESPACE" --ignore-not-found --timeout=60s 2>/dev/null

    info "Removing gateway metrics PodMonitor (NE-1113 workaround)..."
    oc delete podmonitor gateway-istio-monitor -n "$INGRESS_NAMESPACE" --ignore-not-found 2>/dev/null

    info "Removing HTTPRoute..."
    oc delete httproute "$HTTPROUTE_NAME" -n "$APP_NAMESPACE" --ignore-not-found 2>/dev/null

    info "Removing demo app namespace..."
    oc delete namespace "$APP_NAMESPACE" --ignore-not-found --timeout=60s 2>/dev/null

    info "Removing Gateway..."
    oc delete gateway "$GATEWAY_NAME" -n "$INGRESS_NAMESPACE" --ignore-not-found 2>/dev/null
    sleep 5

    info "Removing GatewayClass..."
    oc delete gatewayclass "$GATEWAY_CLASS_NAME" --ignore-not-found 2>/dev/null
    info "Waiting for gateway components to clean up in $INGRESS_NAMESPACE..."
    local elapsed=0
    while [[ $elapsed -lt 120 ]]; do
        oc get deployment istiod-openshift-gateway -n "$INGRESS_NAMESPACE" &>/dev/null || break
        sleep 5; elapsed=$((elapsed + 5))
    done

    info "Removing OSSM operator..."
    oc delete subscription servicemeshoperator3 -n openshift-operators --ignore-not-found 2>/dev/null
    csv=$(oc get csv -n openshift-operators --no-headers 2>/dev/null \
        | grep servicemesh | awk '{print $1}')
    [[ -n "$csv" ]] && oc delete csv "$csv" -n openshift-operators 2>/dev/null

    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}${BOLD}║  Uninstall complete                              ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""

    local remaining_crds
    remaining_crds=$(oc get crd --no-headers 2>/dev/null \
        | grep -E 'istio|sail|gateway|kiali' | awk '{print $1}')
    if [[ -n "$remaining_crds" ]]; then
        warn "These CRDs remain on the cluster:"
        report_crds
        echo ""
        info "To remove them:"
        info "  oc get crd | grep -E 'istio|sail|gateway|kiali' | awk '{print \$1}' | xargs oc delete crd"
    else
        ok "No leftover CRDs found"
    fi

    echo ""
    if oc get configmap cluster-monitoring-config -n "$MONITORING_NAMESPACE" &>/dev/null; then
        info "If you enabled user workload monitoring for this demo, optionally revert:"
        info "  oc delete configmap cluster-monitoring-config -n $MONITORING_NAMESPACE"
    fi
    echo ""
}

usage() {
    echo ""
    echo "Usage: $(basename "$0") <command> [options]"
    echo ""
    echo "Commands:"
    echo "  install              Install all components"
    echo "  traffic [options]    Generate traffic (try: traffic --help)"
    echo "  scrape [options]     Dump raw Envoy metrics (try: scrape --help)"
    echo "  prom [options] [metric]  Query Prometheus for gateway metrics (try: prom --help)"
    echo "  status               Show status of all components"
    echo "  urls                 Print access URLs"
    echo "  uninstall            Remove all components"
    echo ""
    echo "What gets installed (minimal — no full Istio mesh):"
    echo "  - OSSM 3.x operator (provides CRDs + Istio control plane)"
    echo "  - GatewayClass + Gateway in openshift-ingress"
    echo "  - Plain demo app (no sidecar) + HTTPRoute"
    echo "  - User workload monitoring (if not already enabled)"
    echo "  - PodMonitor in openshift-ingress for gateway metrics (NE-1113 workaround)"
    echo "  - Kiali operator + CR (queries thanos-querier for metrics)"
    echo "  - ClusterRoleBinding for Kiali to access platform monitoring"
    echo ""
    echo "Examples:"
    echo "  $(basename "$0") install                    # install everything"
    echo "  $(basename "$0") traffic -d 120 -r 5        # 120s at 5 req/s, clean"
    echo "  $(basename "$0") traffic -d 30 -e           # 30s with 404 error paths"
    echo "  $(basename "$0") scrape                     # Kiali-relevant metrics only"
    echo "  $(basename "$0") scrape --other show        # include other proxy metrics"
    echo "  $(basename "$0") prom                       # list gateway metrics in Prometheus"
    echo "  $(basename "$0") prom istio_requests_total   # show labels + timeseries data"
    echo "  $(basename "$0") uninstall                  # clean up"
    echo ""
}

# ── Entry Point ────────────────────────────────────────────────────────────────

case "${1:-help}" in
    install)          do_install ;;
    traffic)          shift; do_traffic "$@" ;;
    scrape)           shift; do_scrape "$@" ;;
    prom|prometheus)  shift; do_prometheus "$@" ;;
    status)           do_status ;;
    urls)             do_urls ;;
    uninstall)        do_uninstall ;;
    help|*)           usage ;;
esac
