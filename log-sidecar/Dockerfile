ARG COREUTILS_VERSION=9.6

FROM alpine:latest as builder
ARG COREUTILS_VERSION
# Build a statically compiled version of tail to copy into the final image
RUN apk add --no-cache build-base
WORKDIR /src
ENV FORCE_UNSAFE_CONFIGURE=1
RUN wget https://ftp.gnu.org/gnu/coreutils/coreutils-9.6.tar.xz && \
    tar xf coreutils-${COREUTILS_VERSION}.tar.xz && \
    cd coreutils-${COREUTILS_VERSION} && \
    ./configure LDFLAGS="-static" && \
    make
RUN mv /src/coreutils-${COREUTILS_VERSION}/src/tail /tail

FROM gcr.io/distroless/static-debian12
COPY --from=builder /tail /usr/bin/tail
ENTRYPOINT ["/usr/bin/tail", "-n+1", "-F"]
CMD ["/dev/null"]