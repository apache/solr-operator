#!/usr/bin/env bash
# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail
# error on unset variables
set -u

echo "Setting up Release ${VERSION}, making last commit."

mkdir -p release-artifacts/crds

# Create Release CRD files
{
  cat hack/headers/header.yaml.txt
  printf "\n"
} > release-artifacts/crds/all.yaml
for filename in config/crd/bases/*.yaml; do
    output_file=${filename#"config/crd/bases/solr.apache.org_"}
    # Create individual file with Apache Header
    {
      cat hack/headers/header.yaml.txt;
      printf "\n"
      cat "${filename}";
    } > "release-artifacts/crds/${output_file}"

    # Add to aggregate file
    cat "${filename}" >> release-artifacts/crds/all.yaml
done

# Fetch the correct dependency Zookeeper CRD, package with other CRDS
{
  cat hack/headers/zookeeper-operator-header.yaml.txt;
  printf "\n\n---\n"
  curl -sL "https://raw.githubusercontent.com/pravega/zookeeper-operator/v0.2.9/deploy/crds/zookeeper.pravega.io_zookeeperclusters_crd.yaml"
} > release-artifacts/crds/zookeeperclusters.yaml

# Package all Solr and Dependency CRDs
{
  cat release-artifacts/crds/all.yaml
  printf "\n"
  cat release-artifacts/crds/zookeeperclusters.yaml
} > release-artifacts/crds/all-with-dependencies.yaml

#git add helm config docs

#git commit -asm "Cutting release version ${VERSION} of the Solr Operator"
