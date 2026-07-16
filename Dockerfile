# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .

# === Release builds ===
FROM builder AS build-user-api
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o /user-api ./service/user/api

FROM builder AS build-user-rpc
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o /user-rpc ./service/user/rpc

FROM builder AS build-order-api
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o /order-api ./service/order/api

FROM builder AS build-inventory-rpc
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o /inventory-rpc ./service/inventory/rpc

# === Debug builds (with Delve) ===
FROM builder AS build-user-api-debug
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -gcflags="all=-N -l" -o /user-api ./service/user/api

FROM builder AS build-user-rpc-debug
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -gcflags="all=-N -l" -o /user-rpc ./service/user/rpc

FROM builder AS build-order-api-debug
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -gcflags="all=-N -l" -o /order-api ./service/order/api

FROM builder AS build-inventory-rpc-debug
RUN go install github.com/go-delve/delve/cmd/dlv@latest
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -gcflags="all=-N -l" -o /inventory-rpc ./service/inventory/rpc

# === Release runtime ===
FROM alpine:3.21 AS user-api
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-user-api /user-api .
COPY service/user/api/etc/ ./etc/
EXPOSE 8080
CMD ["./user-api"]

FROM alpine:3.21 AS user-rpc
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-user-rpc /user-rpc .
COPY service/user/rpc/etc/ ./etc/
EXPOSE 9090
CMD ["./user-rpc"]

FROM alpine:3.21 AS order-api
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-order-api /order-api .
COPY service/order/api/etc/ ./etc/
EXPOSE 8081
CMD ["./order-api"]

FROM alpine:3.21 AS inventory-rpc
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-inventory-rpc /inventory-rpc .
COPY service/inventory/rpc/etc/ ./etc/
EXPOSE 9091
CMD ["./inventory-rpc"]

# === Debug runtime (with Delve) ===
FROM alpine:3.21 AS user-api-debug
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-user-api-debug /go/bin/dlv /usr/local/bin/dlv
COPY --from=build-user-api-debug /user-api .
COPY service/user/api/etc/ ./etc/
EXPOSE 8080 40001
CMD ["dlv", "--listen=:40001", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "./user-api"]

FROM alpine:3.21 AS user-rpc-debug
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-user-rpc-debug /go/bin/dlv /usr/local/bin/dlv
COPY --from=build-user-rpc-debug /user-rpc .
COPY service/user/rpc/etc/ ./etc/
EXPOSE 9090 40002
CMD ["dlv", "--listen=:40002", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "./user-rpc"]

FROM alpine:3.21 AS order-api-debug
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-order-api-debug /go/bin/dlv /usr/local/bin/dlv
COPY --from=build-order-api-debug /order-api .
COPY service/order/api/etc/ ./etc/
EXPOSE 8081 40003
CMD ["dlv", "--listen=:40003", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "./order-api"]

FROM alpine:3.21 AS inventory-rpc-debug
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-inventory-rpc-debug /go/bin/dlv /usr/local/bin/dlv
COPY --from=build-inventory-rpc-debug /inventory-rpc .
COPY service/inventory/rpc/etc/ ./etc/
EXPOSE 9091 40004
CMD ["dlv", "--listen=:40004", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "./inventory-rpc"]
