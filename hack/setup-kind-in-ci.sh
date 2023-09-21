#!/bin/bash

#
# Refer to the --help output for a description of this script and its available options.
#

infomsg() {
  echo "[INFO] ${1}"
}

helpmsg() {
  cat <<HELP
This script will run setup a KinD cluster for testing Kiali against a real environment in CI.
Options:
-a|--auth-strategy <anonymous|token>
    Auth stategy to use for Kiali.
    Default: anonymous
-dorp|--docker-or-podman <docker|podman>
    What to use when building images.
    Default: docker
-iv|--istio-version <#.#.#>
    The version of Istio you want to install.
    If you want to run with a dev build of Istio, the value must be something like "#.#-dev".
    This option is ignored if -ii is false.
    If not specified, the latest version of Istio is installed.
    Default: <the latest release>
-mc|--multicluster <true|false>
    Whether to set up a multicluster environment.
    Default: false
HELP
}

# process command line arguments
while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    -a|--auth-strategy)           AUTH_STRATEGY="$2";         shift;shift; ;;
    -dorp|--docker-or-podman)     DORP="$2";                  shift;shift; ;;
    -h|--help)                    helpmsg;                    exit 1       ;;
    -iv|--istio-version)          ISTIO_VERSION="$2";         shift;shift; ;;
    -mc|--multicluster)           MULTICLUSTER="$2";          shift;shift; ;;
    *) echo "Unknown argument: [$key]. Aborting."; helpmsg; exit 1 ;;
  esac
done

# abort on any error
set -e

# set up some of our defaults
AUTH_STRATEGY="${AUTH_STRATEGY:-anonymous}"
DORP="${DORP:-docker}"
MULTICLUSTER="${MULTICLUSTER:-false}"

# Defaults the branch to master unless it is already set
TARGET_BRANCH="${TARGET_BRANCH:-master}"

# If a specific version of Istio hasn't been provided, try and guess the right one
# based on the Kiali branch being tested (TARGET_BRANCH) and the compatibility matrices:
# https://kiali.io/docs/installation/installation-guide/prerequisites/
# https://istio.io/latest/docs/releases/supported-releases/
if [ -z "${ISTIO_VERSION}" ]; then
  if [ "${TARGET_BRANCH}" == "v1.48" ]; then
    ISTIO_VERSION="1.13.0"
  fi
fi

KIND_NODE_IMAGE=""
if [ "${ISTIO_VERSION}" == "1.13.0" ]; then
  KIND_NODE_IMAGE="kindest/node:v1.23.4@sha256:0e34f0d0fd448aa2f2819cfd74e99fe5793a6e4938b328f657c8e3f81ee0dfb9"
else
  KIND_NODE_IMAGE="kindest/node:v1.27.3@sha256:3966ac761ae0136263ffdb6cfd4db23ef8a83cba8a463690e98317add2c9ba72"
fi

# print out our settings for debug purposes
cat <<EOM
=== SETTINGS ===
AUTH_STRATEGY="$AUTH_STRATEGY
DORP=$DORP
ISTIO_VERSION=$ISTIO_VERSION
KIND_NODE_IMAGE=$KIND_NODE_IMAGE
MULTICLUSTER=$MULTICLUSTER
TARGET_BRANCH=$TARGET_BRANCH
=== SETTINGS ===
EOM

infomsg "Make sure everything exists"
which kubectl > /dev/null || (infomsg "kubectl executable is missing"; exit 1)
which kind > /dev/null || (infomsg "kind executable is missing"; exit 1)
which "${DORP}" > /dev/null || (infomsg "[$DORP] is not in the PATH"; exit 1)

HELM_CHARTS_DIR="$(mktemp -d)"
SCRIPT_DIR="$(cd $(dirname "${BASH_SOURCE[0]}") && pwd)"

if [ -n "${ISTIO_VERSION}" ]; then
  if [[ "${ISTIO_VERSION}" == *-dev ]]; then
    DOWNLOAD_ISTIO_VERSION_ARG="--dev-istio-version ${ISTIO_VERSION}"
  else
    DOWNLOAD_ISTIO_VERSION_ARG="--istio-version ${ISTIO_VERSION}"
  fi
fi

infomsg "Downloading istio"
hack/istio/download-istio.sh ${DOWNLOAD_ISTIO_VERSION_ARG}

setup_kind_singlecluster() {
  "${SCRIPT_DIR}"/start-kind.sh --name ci --image "${KIND_NODE_IMAGE}"

  infomsg "Installing istio"
  if [[ "${ISTIO_VERSION}" == *-dev ]]; then
    local hub_arg="--image-hub default"
  fi
  # Apparently you can't set the requests to zero for the proxy so just setting them to some really low number.
  hack/istio/install-istio-via-istioctl.sh --reduce-resources true --client-exe-path "$(which kubectl)" -cn "cluster-default" -mid "mesh-default" -net "network-default" -gae "true" ${hub_arg:-}

  infomsg "Pushing the images into the cluster..."
  make -e DORP="${DORP}" -e CLUSTER_TYPE="kind" -e KIND_NAME="ci" cluster-push-kiali

  local istio_ingress_gateway_ip
  istio_ingress_gateway_ip="$(kubectl get svc istio-ingressgateway -n istio-system -o=jsonpath='{.status.loadBalancer.ingress[0].ip}')"

  # Re-use bookinfo gateway but have a separate VirtualService for Kiali
  kubectl apply -f - <<EOF
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: kiali
  namespace: istio-system
spec:
  gateways:
  - bookinfo/bookinfo-gateway
  hosts:
  - "${istio_ingress_gateway_ip}"
  http:
  - match:
    - uri:
        prefix: /kiali
    route:
    - destination:
        host: kiali
        port:
          number: 20001
EOF

  infomsg "Cloning kiali helm-charts..."
  git clone --single-branch --branch "${TARGET_BRANCH}" https://github.com/kiali/helm-charts.git "${HELM_CHARTS_DIR}"
  make -C "${HELM_CHARTS_DIR}" build-helm-charts

  HELM="${HELM_CHARTS_DIR}/_output/helm-install/helm"

  infomsg "Using helm: $(ls -l ${HELM})"
  infomsg "$(${HELM} version)"

  infomsg "Installing kiali server via Helm"
  infomsg "Chart to be installed: $(ls -1 ${HELM_CHARTS_DIR}/_output/charts/kiali-server-*.tgz)"
  # The grafana and tracing urls need to be set for backend e2e tests
  # but they don't need to be accessible outside the cluster.
  # Need a single dashboard set for grafana.
  ${HELM} install \
    --namespace istio-system \
    --wait \
    --set auth.strategy="${AUTH_STRATEGY}" \
    --set deployment.logger.log_level="trace" \
    --set deployment.image_name=localhost/kiali/kiali \
    --set deployment.image_version=dev \
    --set deployment.image_pull_policy="Never" \
    --set external_services.grafana.url="http://grafana.istio-system:3000" \
    --set external_services.grafana.dashboards[0].name="Istio Mesh Dashboard" \
    --set external_services.tracing.url="http://tracing.istio-system:16685/jaeger" \
    --set health_config.rate[0].kind="service" \
    --set health_config.rate[0].name="y-server" \
    --set health_config.rate[0].namespace="alpha" \
    --set health_config.rate[0].tolerance[0].code="5xx" \
    --set health_config.rate[0].tolerance[0].degraded=2 \
    --set health_config.rate[0].tolerance[0].failure=100 \
    kiali-server \
    "${HELM_CHARTS_DIR}"/_output/charts/kiali-server-*.tgz
}

setup_kind_multicluster() {
  if [ -n "${ISTIO_VERSION}" ]; then
    if [[ "${ISTIO_VERSION}" == *-dev ]]; then
      DOWNLOAD_ISTIO_VERSION_ARG="--dev-istio-version ${ISTIO_VERSION}"
    else
      DOWNLOAD_ISTIO_VERSION_ARG="--istio-version ${ISTIO_VERSION}"
    fi
  fi

  infomsg "Downloading istio"
  hack/istio/download-istio.sh ${DOWNLOAD_ISTIO_VERSION_ARG}

  infomsg "Cloning kiali helm-charts..."
  git clone --single-branch --branch "${TARGET_BRANCH}" https://github.com/kiali/helm-charts.git "${HELM_CHARTS_DIR}"
  make -C "${HELM_CHARTS_DIR}" build-helm-charts

  infomsg "Kind cluster to be created with name [ci]"
  local script_dir
  script_dir="$(cd $(dirname "${BASH_SOURCE[0]}") && pwd)"
  local output_dir
  output_dir="${script_dir}/../_output"
  # use the Istio release that was last downloaded (that's the -t option to ls)
  local istio_dir
  istio_dir=$(ls -dt1 ${output_dir}/istio-* | head -n1)
  if [[ "${ISTIO_VERSION}" == *-dev ]]; then
    local hub_arg="--istio-hub default"
  fi
  hack/istio/multicluster/install-primary-remote.sh --manage-kind true -dorp docker --istio-dir "${istio_dir}" ${hub_arg:-}
  hack/istio/multicluster/deploy-kiali.sh --manage-kind true -dorp docker -kas anonymous -kudi true -kshc "${HELM_CHARTS_DIR}"/_output/charts/kiali-server-*.tgz
}

if [ "${MULTICLUSTER}" == "true" ]; then
  setup_kind_multicluster
else
  setup_kind_singlecluster
  # Create the citest service account whose token will be used to log into Kiali
  infomsg "Installing the test ServiceAccount with read-write permissions"
  for o in role rolebinding serviceaccount; do ${HELM} template --show-only "templates/${o}.yaml" --namespace=istio-system --set deployment.instance_name=citest --set auth.strategy=anonymous kiali-server "${HELM_CHARTS_DIR}"/_output/charts/kiali-server-*.tgz; done | kubectl apply -f -
fi


# Unfortunately kubectl rollout status fails if the resource does not exist yet.
for (( i=1; i<=60; i++ ))
do
  PODS=$(kubectl get pods -l app=kiali -n istio-system -o name)
  if [ "${PODS}" != "" ]; then
    infomsg "Kiali pods exist"
    break
  fi

  infomsg "Waiting for kiali pod to exist"
  sleep 5
done

# Checking pod status in a loop gives us more debug info on the state of the pod.
TIMEOUT="True"
for (( i=1; i<=30; i++ ))
do
  READY=$(kubectl get pods -l app=kiali -n istio-system -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}')
  if [ "${READY}" == "True" ]; then
    infomsg "Kiali finished rolling out successfully"
    TIMEOUT="False"
    break
  fi

  infomsg "Waiting for kiali pod to be ready"
  infomsg "Kiali pod status:"
  # Show status info of kiali pod. yq is used to parse out just the status info.
  if command -v yq &> /dev/null; then
    kubectl get pods -l app=kiali -n istio-system -o yaml | yq '.items[0].status'
  else
    kubectl get pods -l app=kiali -n istio-system -o yaml
  fi
  sleep 10
done

if [ "${TIMEOUT}" == "True" ]; then
  infomsg "Timed out waiting for kiali pods to be ready"
  exit 1
fi

infomsg "Kiali is ready."
