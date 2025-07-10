FROM golang:1.21-alpine AS builder

WORKDIR /src

COPY ctrlbench/go.mod ctrlbench/go.sum ./
RUN go mod download

COPY ctrlbench/ ./

RUN CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -o ctrlbench .

FROM scratch


COPY --from=builder /src/ctrlbench /usr/local/bin/ctrlbench

COPY openapi/ /etc/ctrlbench/openapi/

ENV OPENAPI_PATH=/etc/ctrlbench/openapi

EXPOSE 8000

ENTRYPOINT ["/usr/local/bin/ctrlbench"]
