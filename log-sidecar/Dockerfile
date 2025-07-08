FROM alpine:latest as builder

ARG COREUTILS_VERSION=9.7
ARG TINI_VERSION=v0.19.0

# Build a statically compiled version of tail to copy into the final image
RUN apk add --no-cache build-base
WORKDIR /src
ENV FORCE_UNSAFE_CONFIGURE=1
RUN wget https://ftp.gnu.org/gnu/coreutils/coreutils-${COREUTILS_VERSION}.tar.xz && \
    tar xf coreutils-${COREUTILS_VERSION}.tar.xz && \
    cd coreutils-${COREUTILS_VERSION} && \
    ./configure LDFLAGS="-static" && \
    make
RUN mv /src/coreutils-${COREUTILS_VERSION}/src/tail /tail

RUN wget -O /tini https://github.com/krallin/tini/releases/download/${TINI_VERSION}/tini-static && \
    chmod +x /tini
FROM gcr.io/distroless/static-debian12
COPY --from=builder /tail /usr/bin/tail
COPY --from=builder /tini /usr/bin/tini

# tini handles SIGTERM for static tail
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/bin/tail", "-n+1", "-F"]
CMD ["/dev/null"]