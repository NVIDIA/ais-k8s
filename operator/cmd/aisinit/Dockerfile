FROM golang:1.22 as builder

ARG BUILD_VERSION
ARG BUILD_TIME

ENV BUILD_VERSION ${BUILD_VERSION}
ENV BUILD_TIME ${BUILD_TIME}

WORKDIR /workspace
COPY . .

RUN make aisinit

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/bin/aisinit /usr/bin

ENTRYPOINT ["aisinit"]
