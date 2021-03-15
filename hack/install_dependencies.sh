#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

kubebuilder_version=2.3.1
kustomize_version=4.0.5
os=$(go env GOOS)
arch=$(go env GOARCH)

# Install go modules 
GO111MODULE=on go mod tidy

#Install Kustomize
if ! (which kustomize); then
  (cd /tmp && curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash -s -- ${kustomize_version} /usr/local/bin)
  echo "Installed kustomize at /usr/local/bin/kustomize, version: $(kustomize version --short)"
else
  echo "Kustomize already installed at $(which kustomize), version: $(kustomize version --short)"
fi

# Install Kubebuilder
if ! (which kubebuilder && (kubebuilder version | grep ${kubebuilder_version})); then
  curl -sL "https://go.kubebuilder.io/dl/${kubebuilder_version}/${os}/${arch}" | tar -xz -C /tmp/
  sudo rm -rf /usr/local/kubebuilder
  sudo mv "/tmp/kubebuilder_${kubebuilder_version}_${os}_${arch}" /usr/local/kubebuilder
  export PATH=$PATH:/usr/local/kubebuilder/bin
  echo "Installed kubebuilder at /usr/local/kubebuilder/bin/kubebuilder"
  kubebuilder version
else
  echo "Kubebuilder already installed at $(which kubebuilder)"
fi

# Install go-licenses
go get github.com/google/go-licenses
