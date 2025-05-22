ARG GO_VERSION=1.24
FROM docker.io/library/golang:${GO_VERSION}-alpine

RUN apk add --no-cache bash curl git make which

ENV LOCALBIN="/bin"

COPY . /operator

RUN cd /operator \
    && go mod download \
    && make kustomize controller-gen envtest golangci-lint mockgen

ENTRYPOINT ["sleep", "infinity"]
