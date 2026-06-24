.PHONY: run-user-api run-user-rpc run-order-api build test clean docker-up docker-down proto lint install-lint \
       infra infra-full infra-down dev-user-api dev-user-rpc dev-order-api \
       debug debug-user-api debug-user-rpc debug-order-api

run-user-api:
	go run ./service/user/api

run-user-rpc:
	go run ./service/user/rpc

run-order-api:
	go run ./service/order/api

build:
	go build -o bin/user-api ./service/user/api
	go build -o bin/user-rpc ./service/user/rpc
	go build -o bin/order-api ./service/order/api

test:
	go test ./... -v

clean:
	rm -rf bin/

proto:
	protoc --go_out=. --go-grpc_out=. \
	  --go_opt=module=github.com/fx0x55/micro-go-lab \
	  --go-grpc_opt=module=github.com/fx0x55/micro-go-lab \
	  api/user/v1/user.proto
	mv api/user/v1/*.go service/user/rpc/pb/
	@sed -i '' 's/^package userv1$$/package pb/' service/user/rpc/pb/user.pb.go service/user/rpc/pb/user_grpc.pb.go
	@sed -i '' 's|"github.com/fx0x55/micro-go-lab/gen/user/v1"|"github.com/fx0x55/micro-go-lab/service/user/rpc/pb"|g' service/user/rpc/pb/user_grpc.pb.go

docker-up:
	docker compose up -d --build

docker-full:
	docker compose --profile monitoring up -d --build

docker-down:
	docker compose --profile monitoring down -v

install-lint:
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "golangci-lint not found, installing v2.12.2..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2; \
	else \
		CURRENT_VERSION=$$(golangci-lint version --short 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown"); \
		if [ "$$CURRENT_VERSION" != "2.12.2" ]; then \
			echo "golangci-lint version $$CURRENT_VERSION found, but v2.12.2 is required. Upgrading..."; \
			go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2; \
		else \
			echo "golangci-lint v2.12.2 is already installed"; \
		fi \
	fi

lint: install-lint
	golangci-lint run ./...

format: install-lint
	golangci-lint run --fix ./...

# === Infrastructure (docker-dev.yml) ===
infra:
	docker compose -f docker-dev.yml up -d

infra-full:
	docker compose -f docker-dev.yml --profile monitoring up -d

infra-down:
	docker compose -f docker-dev.yml down -v

# === Local development (infra Docker + native Go) ===
dev-user-api: infra
	go run ./service/user/api

dev-user-rpc: infra
	go run ./service/user/rpc

dev-order-api: infra
	go run ./service/order/api

# === Container debug (Delve) ===
debug:
	docker compose --profile debug up -d --build

debug-user-api:
	docker compose --profile debug up -d --build user-api-debug

debug-user-rpc:
	docker compose --profile debug up -d --build user-rpc-debug

debug-order-api:
	docker compose --profile debug up -d --build order-api-debug
