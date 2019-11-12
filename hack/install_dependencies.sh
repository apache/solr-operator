#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

kubebuilder_version=2.0.1
os=$(go env GOOS)
arch=$(go env GOARCH)

# Install go modules 
GO111MODULE=on go mod tidy 

# Install Kubebuilder
curl -sL https://go.kubebuilder.io/dl/${kubebuilder_version}/${os}/${arch} | tar -xz -C /tmp/
sudo mv /tmp/kubebuilder_${kubebuilder_version}_${os}_${arch} /usr/local/kubebuilder
export PATH=$PATH:/usr/local/kubebuilder/bin
