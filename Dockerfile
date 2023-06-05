# Build the manager binary
FROM golang:1.18 as builder

WORKDIR /workspace

ARG GO111MODULE=on
ARG GOPROXY='https://goproxy.cn,direct'

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o greatdb-operator ./cmd/controller-manager/server.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM kubeimages/distroless-static
WORKDIR /
COPY --from=builder /workspace/greatdb-operator .
USER 65532:65532

ENTRYPOINT ["/greatdb-operator"]
