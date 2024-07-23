# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.23rc2 as builder

# Add GRPC healthchecker
# TODO when Kube 1.27 or higher is a prerequisite this binary can be removed.
RUN go install github.com/grpc-ecosystem/grpc-health-probe@latest

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/

#RUN go mod tidy
RUN go mod download

ARG TARGETOS TARGETARCH

# Build
RUN --mount=type=cache,target=/root/.cache/go-build \
        --mount=type=cache,target=/go/pkg \
        GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -o msm-proxy cmd/msm-dp/*

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#FROM gcr.io/distroless/static:nonroot
FROM ubuntu
WORKDIR /
COPY --from=builder /workspace/msm-proxy .
COPY --from=builder /go/bin/grpc-health-probe /usr/local/bin
#USER nonroot:nonroot

ENTRYPOINT ["/msm-proxy"]
