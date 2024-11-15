FROM golang:1.23 as builder
ARG TARGETOS
ARG TARGETARCH

ARG BUILD_VERSION
ARG BUILD_TIME

WORKDIR /workspace
COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags "-w -s -X 'main.build=${BUILD_VERSION}' -X 'main.buildtime=${BUILD_TIME}'" -a -o aisinit ./cmd/aisinit

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/aisinit /usr/bin

ENTRYPOINT ["aisinit"]
