#!/bin/bash
#
# gwapi-kiali-demo.sh — Gateway API + Kiali Observability Demo
#
# For OSSM-12190: exploring what metrics the OpenShift ingress Gateway API
# produces and how Kiali can observe them — without a full Istio mesh.
#
# Installs only:
#   - OSSM 3.x operator (needed for Gateway API CRDs)
#   - GatewayClass + Gateway (triggers lightweight Istio in openshift-ingress)
#   - Plain demo app (no sidecars) + HTTPRoute
#   - PodMonitor for the gateway proxy's Envoy metrics
#   - Kiali operator + CR (pointed at the gateway's istiod)
#
# Usage:
#   ./gwapi-kiali-demo.sh install      Install all components
#   ./gwapi-kiali-demo.sh traffic      Generate traffic through the gateway
#   ./gwapi-kiali-demo.sh metrics      Dump raw Envoy metrics from the gateway proxy
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

    create_gateway_podmonitor
}

create_gateway_podmonitor() {
    info "Creating PodMonitor for gateway proxy metrics..."

    # Placed in the app namespace so user-workload Prometheus picks it up.
    # namespaceSelector targets the gateway pods in openshift-ingress.
    local tmpfile
    tmpfile=$(mktemp)
    cat > "$tmpfile" <<YAML
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: gateway-proxy-monitor
  namespace: ${APP_NAMESPACE}
spec:
  namespaceSelector:
    matchNames:
    - ${INGRESS_NAMESPACE}
  selector:
    matchExpressions:
    - key: istio-prometheus-ignore
      operator: DoesNotExist
  podMetricsEndpoints:
  - path: /stats/prometheus
    interval: 15s
    relabelings:
    - action: keep
      sourceLabels: ["__meta_kubernetes_pod_container_name"]
      regex: "istio-proxy"
    - action: keep
      sourceLabels: ["__meta_kubernetes_pod_annotationpresent_prometheus_io_scrape"]
YAML

    # These relabeling rules contain dollar signs that must not be
    # interpreted by the shell — printf preserves them literally.
    printf '    - action: replace
      regex: (\\d+);(([A-Fa-f0-9]{1,4}::?){1,7}[A-Fa-f0-9]{1,4})
      replacement: "[$2]:$1"
      sourceLabels: ["__meta_kubernetes_pod_annotation_prometheus_io_port","__meta_kubernetes_pod_ip"]
      targetLabel: "__address__"
    - action: replace
      regex: (\\d+);((([0-9]+?)(\\.|$)){4})
      replacement: "$2:$1"
      sourceLabels: ["__meta_kubernetes_pod_annotation_prometheus_io_port","__meta_kubernetes_pod_ip"]
      targetLabel: "__address__"
    - sourceLabels: ["__meta_kubernetes_pod_label_app_kubernetes_io_name","__meta_kubernetes_pod_label_app"]
      separator: ";"
      targetLabel: "app"
      action: replace
      regex: "(.+);.*|.*;(.+)"
      replacement: "${1}${2}"
    - sourceLabels: ["__meta_kubernetes_pod_label_app_kubernetes_io_version","__meta_kubernetes_pod_label_version"]
      separator: ";"
      targetLabel: "version"
      action: replace
      regex: "(.+);.*|.*;(.+)"
      replacement: "${1}${2}"
    - sourceLabels: ["__meta_kubernetes_namespace"]
      action: replace
      targetLabel: namespace
    - action: replace
      replacement: "default"
      targetLabel: mesh_id
' >> "$tmpfile"

    oc apply -f "$tmpfile"
    rm -f "$tmpfile"

    ok "PodMonitor created in $APP_NAMESPACE (targeting $INGRESS_NAMESPACE)"
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
    info "  2. View raw metrics:  $0 metrics"
    info "  3. Open Kiali UI and explore the graph"
}

do_traffic() {
    local duration="${1:-10}"
    local rate="${2:-2}"

    detect_cluster_domain

    local gateway_svc
    gateway_svc=$(oc get svc -n "$INGRESS_NAMESPACE" \
        -l "gateway.networking.k8s.io/gateway-name=$GATEWAY_NAME" \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    [[ -z "$gateway_svc" ]] && die "Gateway service not found. Run '$0 install' first."

    info "Generating traffic for ${duration}s at ~${rate} req/s"
    info "Target: $GATEWAY_HOSTNAME via port-forward to $gateway_svc"

    # Kill any stale port-forward on this port from a previous run
    local stale_pf
    stale_pf=$(lsof -ti :18080 2>/dev/null || true)
    [[ -n "$stale_pf" ]] && kill $stale_pf 2>/dev/null && sleep 1

    oc port-forward -n "$INGRESS_NAMESPACE" "svc/$gateway_svc" 18080:80 &>/dev/null &
    local pf_pid=$!
    # Bake the PID into the trap string so it works after the function returns
    trap "kill $pf_pid 2>/dev/null; wait $pf_pid 2>/dev/null || true" EXIT
    sleep 2

    kill -0 "$pf_pid" 2>/dev/null || die "Port-forward failed to start"

    local paths=("/" "/index.html" "/noexist" "/icons/" "/api/health")
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

do_metrics() {
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

    local total_metrics istio_metrics
    total_metrics=$(grep -v "^#" "$metrics_tmpfile" | grep -v "^$" \
        | cut -d'{' -f1 | cut -d' ' -f1 | sort -u | wc -l)
    istio_metrics=$(grep -c "^istio_" "$metrics_tmpfile" || echo "0")

    echo ""
    echo -e "${BOLD}── Summary ──${NC}"
    info "Total unique metric names: $total_metrics"
    info "Istio metric lines (istio_*): $istio_metrics"

    echo ""
    echo -e "${BOLD}── Kiali-Relevant Metrics ──${NC}"
    grep "^istio_requests_total\|^istio_request_duration\|^istio_request_bytes\|^istio_response_bytes\|^istio_tcp_sent\|^istio_tcp_received" "$metrics_tmpfile" \
        | cut -d'{' -f1 | sort -u | while read -r name; do
            local count
            count=$(grep -c "^${name}" "$metrics_tmpfile")
            echo "  $name ($count series)"
        done

    echo ""
    echo -e "${BOLD}── All Istio Metric Families ──${NC}"
    grep "^istio_" "$metrics_tmpfile" | cut -d'{' -f1 | cut -d' ' -f1 | sort -u

    echo ""
    echo -e "${BOLD}── Sample: istio_requests_total ──${NC}"
    grep "^istio_requests_total{" "$metrics_tmpfile" | head -5

    rm -f "$metrics_tmpfile"

    echo ""
    info "Full raw dump:"
    info "  oc port-forward -n $INGRESS_NAMESPACE $gateway_pod 15090:15090 &"
    info "  curl -s http://localhost:15090/stats/prometheus"
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
    echo "  PodMonitors:"
    oc get podmonitor -n "$APP_NAMESPACE" --no-headers 2>/dev/null \
        | while read -r line; do echo "    [$APP_NAMESPACE] $line"; done

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

    info "Removing PodMonitor..."
    oc delete podmonitor gateway-proxy-monitor -n "$APP_NAMESPACE" --ignore-not-found 2>/dev/null

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
    echo "  traffic [dur] [rps]  Generate traffic (default: 10s at 2 req/s)"
    echo "  metrics              Dump raw Envoy metrics from gateway proxy"
    echo "  status               Show status of all components"
    echo "  urls                 Print access URLs"
    echo "  uninstall            Remove all components"
    echo ""
    echo "What gets installed (minimal — no full Istio mesh):"
    echo "  - OSSM 3.x operator (for CRDs)"
    echo "  - GatewayClass + Gateway in openshift-ingress"
    echo "  - Plain demo app (no sidecar) + HTTPRoute"
    echo "  - PodMonitor for gateway proxy metrics"
    echo "  - Kiali pointed at the gateway's istiod"
    echo ""
    echo "Examples:"
    echo "  $(basename "$0") install                    # install everything"
    echo "  $(basename "$0") traffic 120 5              # 120s at 5 req/s"
    echo "  $(basename "$0") metrics                    # see raw gateway metrics"
    echo "  $(basename "$0") metrics > gateway-metrics.txt"
    echo "  $(basename "$0") uninstall                  # clean up"
    echo ""
}

# ── Entry Point ────────────────────────────────────────────────────────────────

case "${1:-help}" in
    install)   do_install ;;
    traffic)   do_traffic "${2:-}" "${3:-}" ;;
    metrics)   do_metrics ;;
    status)    do_status ;;
    urls)      do_urls ;;
    uninstall) do_uninstall ;;
    help|*)    usage ;;
esac
