FROM golang:1.24.4-alpine AS builder

WORKDIR /root

COPY ctrlbench/go.mod ctrlbench/go.sum ./

RUN go mod download

COPY ctrlbench/. ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o ctrlbench main.go

FROM alpine:3.18

RUN apk add --no-cache bash

COPY --from=builder /root/ctrlbench /usr/local/bin/ctrlbench

COPY openapi/ /root/openapi/

ENV OPENAPI_PATH=/root/openapi/

ENTRYPOINT ["tail", "-f", "/dev/null"]
