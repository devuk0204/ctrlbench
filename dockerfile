FROM golang:1.24.4-alpine AS builder
WORKDIR /src

COPY ctrlbench/go.mod ctrlbench/go.sum ./
RUN go mod download

COPY ctrlbench/. ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o ctrlbench main.go

FROM busybox:1.35.0

COPY --from=builder /src/ctrlbench /usr/local/bin/ctrlbench

COPY openapi/ /etc/ctrlbench/openapi/

ENV OPENAPI_PATH=/etc/ctrlbench/openapi

ENTRYPOINT ["tail", "-f", "/dev/null"]
