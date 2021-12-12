# Build the manager binary
FROM golang:1.16 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/
COPY hack/ hack/
COPY contrib/ contrib/
COPY .git/ .git/

# mkdir binary folder
RUN mkdir -p binaries

# Build caelus
RUN hack/build.sh && \
    cp _output/bin/* /workspace/binaries/

# Build lighthouse
RUN cd /workspace/contrib/lighthouse && \
    ./hack/binary && \
    cp _output/bin/* /workspace/binaries/
RUN cd /workspace/contrib/lighthouse-plugin && \
    ./hack/binary && \
    cp _output/bin/* /workspace/binaries/

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM centos:centos7
WORKDIR /
COPY --from=builder /workspace/_output/bin/caelus ./
COPY --from=builder /workspace/binaries /binaries/
#USER nonroot:nonroot

# Used to quickly reclaim memory
ENV GODEBUG="madvdontneed=1"
CMD ["/caelus"]
