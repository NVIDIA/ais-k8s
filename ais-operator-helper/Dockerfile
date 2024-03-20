FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY src/ ./

RUN go build -o /cleanup-helper cleanup-helper.go

FROM alpine:latest

COPY --from=builder /cleanup-helper /cleanup-helper
