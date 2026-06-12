FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM builder AS build-user-svc
RUN CGO_ENABLED=0 go build -o /user-svc ./cmd/user-svc

FROM builder AS build-order-svc
RUN CGO_ENABLED=0 go build -o /order-svc ./cmd/order-svc

FROM alpine:3.19 AS user-svc
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-user-svc /user-svc .
COPY config/user-svc.yaml ./config/
EXPOSE 8080 9090
CMD ["./user-svc"]

FROM alpine:3.19 AS order-svc
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build-order-svc /order-svc .
COPY config/order-svc.yaml ./config/
EXPOSE 8081
CMD ["./order-svc"]
