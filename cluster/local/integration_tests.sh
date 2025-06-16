#!/usr/bin/env bash
set -e

# setting up colors
BLU='\033[0;34m'
YLW='\033[0;33m'
GRN='\033[0;32m'
RED='\033[0;31m'
NOC='\033[0m' # No Color
echo_info(){
    printf "\n${BLU}%s${NOC}" "$1"
}
echo_step(){
    printf "\n${BLU}>>>>>>> %s${NOC}\n" "$1"
}
echo_sub_step(){
    printf "\n${BLU}>>> %s${NOC}\n" "$1"
}

echo_step_completed(){
    printf "${GRN} [✔]${NOC}"
}

echo_success(){
    printf "\n${GRN}%s${NOC}\n" "$1"
}
echo_warn(){
    printf "\n${YLW}%s${NOC}" "$1"
}
echo_error(){
    printf "\n${RED}%s${NOC}" "$1"
    exit 1
}

# The name of your provider. Many provider Makefiles override this value.
PACKAGE_NAME="provider-rabbitmq"

# ------------------------------
projectdir="$( cd "$( dirname "${BASH_SOURCE[0]}")"/../.. && pwd )"

# get the build environment variables from the special build.vars target in the main makefile
eval $(make --no-print-directory -C ${projectdir} build.vars)

# ------------------------------

SAFEHOSTARCH="${SAFEHOSTARCH:-amd64}"
BUILD_IMAGE="${BUILD_REGISTRY}/${PROJECT_NAME}-${SAFEHOSTARCH}"
PACKAGE_IMAGE="crossplane.io/inttests/${PROJECT_NAME}:${VERSION}"
CONTROLLER_IMAGE="${BUILD_REGISTRY}/${PROJECT_NAME}-${SAFEHOSTARCH}"

version_tag="$(cat ${projectdir}/_output/version)"
# tag as latest version to load into kind cluster
PACKAGE_CONTROLLER_IMAGE="${DOCKER_REGISTRY}/${PROJECT_NAME}:${VERSION}"
K8S_CLUSTER="${K8S_CLUSTER:-${BUILD_REGISTRY}-inttests}"

CROSSPLANE_NAMESPACE="crossplane-system"

# cleanup on exit
if [ "$skipcleanup" != true ]; then
  function cleanup {
    echo_step "Cleaning up..."
    export KUBECONFIG=
    "${KIND}" delete cluster --name="${K8S_CLUSTER}"
  }

  trap cleanup EXIT
fi

docker tag "${BUILD_IMAGE}" "${PACKAGE_IMAGE}"

# create kind cluster with extra mounts
KIND_NODE_IMAGE="kindest/node:${KIND_NODE_IMAGE_TAG}"
echo_step "creating k8s cluster using kind ${KIND_VERSION} and node image ${KIND_NODE_IMAGE}"
KIND_CONFIG="$( cat <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
EOF
)"
echo "${KIND_CONFIG}" | "${KIND}" create cluster --name="${K8S_CLUSTER}" --wait=5m --image="${KIND_NODE_IMAGE}" --config=-

# tag controller image and load it into kind cluster
echo_step "loading controller image into kind cluster, CONTROLLER_IMAGE=${CONTROLLER_IMAGE} PACKAGE_CONTROLLER_IMAGE=${PACKAGE_CONTROLLER_IMAGE}"
docker tag "${CONTROLLER_IMAGE}" "${PACKAGE_CONTROLLER_IMAGE}"
"${KIND}" load docker-image "${PACKAGE_CONTROLLER_IMAGE}" --name="${K8S_CLUSTER}"

echo_step "create crossplane-system namespace"
"${KUBECTL}" create ns crossplane-system

# install crossplane
CROSSPLANE_HELM_DIR="${projectdir}/helm/crossplane"
helm dep update "${CROSSPLANE_HELM_DIR}"
helm upgrade crossplane "${CROSSPLANE_HELM_DIR}" --install --namespace crossplane-system --create-namespace --wait

# install package
echo_step "installing ${PROJECT_NAME} into \"${CROSSPLANE_NAMESPACE}\" namespace"

INSTALL_YAML="$( cat <<EOF
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: "${PACKAGE_NAME}"
spec:
  package: xpkg.upbound.io/pnowy/provider-rabbitmq:v0.5.0
#  package: "${PACKAGE_CONTROLLER_IMAGE}"
EOF
)"

echo "${INSTALL_YAML}" | "${KUBECTL}" apply -f -

echo_step "waiting for provider to be installed"
kubectl wait "provider.pkg.crossplane.io/${PACKAGE_NAME}" --for=condition=healthy --timeout=180s

echo_step "Installing RabbitMQ"
RABBITMQ_HELM_DIR="${projectdir}/helm/rabbitmq"
helm dep update "${RABBITMQ_HELM_DIR}"
helm upgrade rabbitmq "${RABBITMQ_HELM_DIR}" --install --namespace rabbitmq --create-namespace --wait

echo_step "RabbitMQ provider config"
kubectl apply -f "${projectdir}/examples/provider/config.yaml"
echo_success "RabbitMQ is ready!"

echo_step "Running integration tests"
make uptest

# uninstall provider
if [ "$skipcleanup" != true ]; then
  echo_step "uninstalling ${PROJECT_NAME}"
  echo "${INSTALL_YAML}" | "${KUBECTL}" delete -f -
  # check pods deleted
  timeout=60
  current=0
  step=3
  while [[ $(kubectl get providerrevision.pkg.crossplane.io -o name | wc -l | tr -d '[:space:]') != "0" ]]; do
    echo "waiting for provider to be deleted for another $step seconds"
    current=$current+$step
    if ! [[ $timeout > $current ]]; then
      echo_error "timeout of ${timeout}s has been reached"
    fi
    sleep $step;
  done
fi


echo_success "Integration tests succeeded!"
