# Build the manager binary
FROM --platform=${BUILDPLATFORM} golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETPLATFORM
ARG BUILDPLATFORM

ARG BUILD_VERSION
ARG BUILD_TIME

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Cache dependencies
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api api/
COPY pkg pkg/

# Build
# TARGETOS/TARGETARCH are set by buildx when you pass --platform
RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOARCH=${TARGETARCH} \
    go build -ldflags "-w -s -X 'main.build=${BUILD_VERSION}' -X 'main.buildtime=${BUILD_TIME}'" \
    -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
