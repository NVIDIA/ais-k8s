FROM golang:1.27-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY src/ ./

RUN CGO_ENABLED=0 go build -o /cleanup-helper cleanup-helper.go

FROM gcr.io/distroless/static-debian13@sha256:58133991db06659feaabe0f4e97a35cebf15ef4ea08f8a4c6d2ee5f75e4aa6a0

COPY --from=builder /cleanup-helper /cleanup-helper
