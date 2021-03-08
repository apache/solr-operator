# Build the manager binary
FROM golang:1.14 as builder

WORKDIR /workspace
ARG GO111MODULE=on

# Download necessary libraries
RUN go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1; \
    go get github.com/google/go-licenses

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy development resources
COPY .git/ .git/
COPY hack/ hack/
COPY build/build.sh build/build.sh
COPY LICENSE LICENSE
COPY NOTICE NOTICE
COPY Makefile ./

# Copy the go source
COPY main.go ./
COPY api/ api/
COPY controllers/ controllers/

ARG VERSION
ARG GIT_SHA

# Build
RUN CGO_ENABLED=0 make fetch-licenses-full build

# =============================================================================
# Copy the controller-manager into a thin image
# =============================================================================

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
# Debug is needed, so that the license files are viewable.
# If there is another way to view these files, we can remove "debug-".
FROM gcr.io/distroless/base:debug-nonroot

WORKDIR /
COPY --from=builder workspace/bin/solr-operator .
COPY --from=builder workspace/LICENSE workspace/NOTICE etc/licenses/
COPY --from=builder workspace/licenses etc/licenses/dependencies
USER nonroot:nonroot

ENTRYPOINT ["/solr-operator"]