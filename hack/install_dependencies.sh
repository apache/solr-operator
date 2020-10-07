#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

kubebuilder_version=2.1.0
os=$(go env GOOS)
arch=$(go env GOARCH)

# Install go modules 
GO111MODULE=on go mod tidy

#Install Kustomize
(which kustomize || (cd /usr/local/bin && curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash))

# Install Kubebuilder
if !(which kubebuilder && (kubebuilder version | grep ${kubebuilder_version})); then
  curl -sL "https://go.kubebuilder.io/dl/${kubebuilder_version}/${os}/${arch}" | tar -xz -C /tmp/
  sudo rm -rf /usr/local/kubebuilder
  sudo mv "/tmp/kubebuilder_${kubebuilder_version}_${os}_${arch}" /usr/local/kubebuilder
  export PATH=$PATH:/usr/local/kubebuilder/bin
  kubebuilder version
fi
