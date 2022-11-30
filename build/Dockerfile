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
FROM golang:1.19 as builder

WORKDIR /workspace
ARG GO111MODULE=on

# Copy the Go Modules manifests
COPY go.mod go.sum ./
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy development resources
COPY .git/ .git/
COPY build/build.sh build/build.sh
COPY Makefile LICENSE NOTICE build/LICENSE-ADDITION build/NOTICE-ADDITION ./

# Add additional binary-release-only information to the LICENSE and NOTICES files
RUN echo "\n\n" >> LICENSE; \
    cat LICENSE-ADDITION >> LICENSE; \
    echo "\n\n" >> NOTICE; \
    cat NOTICE-ADDITION >> NOTICE

# Download necessary libraries
RUN make controller-gen go-licenses

# Copy the go source and utilities
COPY hack/ hack/
COPY main.go ./
COPY version/ version/
COPY api/ api/
COPY controllers/ controllers/

ARG GIT_SHA

# Build
RUN CGO_ENABLED=0 GIT_SHA="${GIT_SHA}" make fetch-licenses-full build

# =============================================================================
# Copy the controller-manager into a thin image
# =============================================================================

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# Debug is needed, so that the license files are viewable.
# If there is another way to view these files, we can remove "debug-".
FROM gcr.io/distroless/static:debug-nonroot

WORKDIR /
COPY --from=builder workspace/bin/solr-operator .
COPY --from=builder workspace/LICENSE workspace/NOTICE etc/licenses/
COPY --from=builder workspace/licenses etc/licenses/dependencies
USER 65532:65532

ENTRYPOINT ["/solr-operator"]