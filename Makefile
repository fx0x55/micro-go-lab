.PHONY: run-user run-order build test clean docker-up docker-down

run-user:
	go run ./cmd/user-svc

run-order:
	go run ./cmd/order-svc

build:
	go build -o bin/user-svc ./cmd/user-svc
	go build -o bin/order-svc ./cmd/order-svc

test:
	go test ./... -v

clean:
	rm -rf bin/

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down -v
