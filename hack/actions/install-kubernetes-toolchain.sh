#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly KUSTOMIZE_VERS="v3.8.6"
readonly KUBECTL_VERS="v1.21.1"
readonly KIND_VERS="v0.11.1"
readonly SONOBUOY_VERS="0.19.0"
readonly KUBEBUILDER_VERS="3.1.0"

# The CI env variable is set by Github Actions when running jobs.
# This is used to know how to configure specific GithubAction jobs.
readonly CI="false"

# Envtest Binaries Manager is required for newer versions. See the following for details:
# https://github.com/projectcontour/contour/issues/3832
readonly KUBEBUILDER_TOOLS_VERS="1.19.2"

# Note: The KUBEBUILDER_ASSETS env is required if this path is changed. See the following for details:
# https://book.kubebuilder.io/reference/envtest.html
readonly KUBEBUILDER_TOOLS_DIR="/usr/local/kubebuilder"

readonly PROGNAME=$(basename $0)
readonly CURL=${CURL:-curl}

# Google storage is case sensitive, so we we need to lowercase the OS.
readonly OS=$(uname | tr '[:upper:]' '[:lower:]')

usage() {
  echo "Usage: $PROGNAME INSTALLDIR"
}

download() {
    local -r url="$1"
    local -r target="$2"

    echo Downloading "$target" from "$url"
    ${CURL} --progress-bar --location  --output "$target" "$url"
}

case "$#" in
  "1")
    mkdir -p "$1"
    readonly DESTDIR=$(cd "$1" && pwd)
    ;;
  *)
    usage
    exit 64
    ;;
esac

echo "Installing Kubernetes toolchain..."

# Install ginkgo CLI
if [[ ${GITHUB_ACTIONS} == "true" && ${OS} == "linux" ]]; then
  go get github.com/onsi/ginkgo/...
  mv /home/runner/go/bin/ginkgo ${DESTDIR}/ginkgo
fi

download \
   "https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERS}/kind-${OS}-amd64" \
   "${DESTDIR}/kind"

chmod +x  "${DESTDIR}/kind"

download \
    "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERS}/bin/${OS}/amd64/kubectl" \
    "${DESTDIR}/kubectl"

chmod +x "${DESTDIR}/kubectl"

# Required for integration testing of controller-runtime controllers.
download \
    "https://go.kubebuilder.io/dl/${KUBEBUILDER_VERS}/${OS}/amd64" \
    "${DESTDIR}/kubebuilder"

chmod +x "${DESTDIR}/kubebuilder"

download \
    "https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-${KUBEBUILDER_TOOLS_VERS}-${OS}-amd64.tar.gz" \
    "${DESTDIR}/envtest-bins.tar.gz"

sudo mkdir -p ${KUBEBUILDER_TOOLS_DIR}
sudo tar -C ${KUBEBUILDER_TOOLS_DIR} --strip-components=1 -zvxf "${DESTDIR}/envtest-bins.tar.gz"
rm "${DESTDIR}/envtest-bins.tar.gz"

download \
    "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERS}/kustomize_${KUSTOMIZE_VERS}_${OS}_amd64.tar.gz" \
    "${DESTDIR}/kustomize.tgz"

tar -C "${DESTDIR}" -xf "${DESTDIR}/kustomize.tgz" kustomize
rm "${DESTDIR}/kustomize.tgz"

download \
    "https://github.com/vmware-tanzu/sonobuoy/releases/download/v${SONOBUOY_VERS}/sonobuoy_${SONOBUOY_VERS}_linux_amd64.tar.gz" \
    "${DESTDIR}/sonobuoy.tgz"

tar -C "${DESTDIR}" -xf "${DESTDIR}/sonobuoy.tgz" sonobuoy
rm "${DESTDIR}/sonobuoy.tgz"
