# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build the manager binary
FROM golang:1.27 AS builder

WORKDIR /workspace
ARG GO111MODULE=on

# The Go module and build caches are BuildKit cache mounts (see the RUN instructions below), so they are reused
# across builds without being stored in image layers, which would otherwise pile up for every source change.

# Copy the Go Modules manifests
COPY go.mod go.sum ./
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Download necessary build tools. This only depends on the Makefile (which pins the tool versions), so
# the tools are not rebuilt when the source changes.
COPY Makefile ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    make controller-gen go-licenses

# Copy development resources
COPY build/build.sh build/build.sh
COPY LICENSE NOTICE build/LICENSE-ADDITION build/NOTICE-ADDITION ./

# Add additional binary-release-only information to the LICENSE and NOTICES files
RUN echo "\n\n" >> LICENSE; \
    cat LICENSE-ADDITION >> LICENSE; \
    echo "\n\n" >> NOTICE; \
    cat NOTICE-ADDITION >> NOTICE

# Copy the go source and utilities
COPY hack/ hack/
COPY main.go ./
COPY version/ version/
COPY api/ api/
COPY controllers/ controllers/

ARG GIT_SHA

# Build
# The git SHA is passed to make explicitly, since the .git directory is not part of the build context.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 make GIT_SHA="${GIT_SHA}" fetch-licenses-full build

# =============================================================================
# Copy the controller-manager into a thin image
# =============================================================================

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# Debug is needed, so that the license files are viewable.
# If there is another way to view these files, we can remove "debug-".
FROM gcr.io/distroless/static:debug-nonroot

# Build args are scoped to a stage, so the ones used by the labels below must be declared again here
ARG GIT_SHA
ARG VERSION
ARG DOCS_VERSION=latest

LABEL org.opencontainers.image.title="Apache Solr Operator"
LABEL org.opencontainers.image.description="The Apache Solr Operator is a Kubernetes Operator that makes it easy to deploy, manage, and scale Apache Solr in a cloud-native environment."
LABEL org.opencontainers.image.authors="The Apache Solr Project"
LABEL org.opencontainers.image.url="https://solr.apache.org/operator"
LABEL org.opencontainers.image.source="https://github.com/apache/solr-operator"
LABEL org.opencontainers.image.documentation="https://solr.apache.org/guide/operator/${DOCS_VERSION}/"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${GIT_SHA}"
LABEL org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /
COPY --from=builder workspace/bin/solr-operator .
COPY --from=builder workspace/LICENSE workspace/NOTICE etc/licenses/
COPY --from=builder workspace/licenses etc/licenses/dependencies
USER 65532:65532

ENTRYPOINT ["/solr-operator"]
