#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly KUSTOMIZE_VERS="v3.8.6"
readonly KUBECTL_VERS="v1.21.1"
readonly KUBEBUILDER_VERS="2.3.1"
readonly KIND_VERS="v0.11.1"
readonly SONOBUOY_VERS="0.19.0"

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
if [[ ${OS} == "linux" ]]; then
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
echo Downloading "${DESTDIR}/kubebuilder" from "https://go.kubebuilder.io/dl/${KUBEBUILDER_VERS}/${OS}/amd64"
curl --progress-bar --location "https://go.kubebuilder.io/dl/${KUBEBUILDER_VERS}/${OS}/amd64" | tar -xz -C "${DESTDIR}"
mv "${DESTDIR}/kubebuilder_${KUBEBUILDER_VERS}_${OS}_amd64" "${DESTDIR}/kubebuilder"

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
