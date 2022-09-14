# Build the manager binary
FROM golang:1.19 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/

RUN go mod tidy
RUN go mod download

# Build
RUN CGO_ENABLED=1 GOOS=linux GO111MODULE=on go build -a -o msm-proxy cmd/msm-dp/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#FROM gcr.io/distroless/static:nonroot
FROM ubuntu
WORKDIR /
COPY --from=builder /workspace/msm-proxy .
#USER nonroot:nonroot

ENTRYPOINT ["/msm-proxy"]
