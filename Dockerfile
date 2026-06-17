FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM builder AS build-user-api
RUN CGO_ENABLED=0 go build -o /user-api ./service/user/api

FROM builder AS build-user-rpc
RUN CGO_ENABLED=0 go build -o /user-rpc ./service/user/rpc

FROM builder AS build-order-api
RUN CGO_ENABLED=0 go build -o /order-api ./service/order/api

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
