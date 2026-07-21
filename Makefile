.PHONY: run-user-api run-user-rpc run-order-api build test clean docker-up docker-down proto lint install-lint \
       infra infra-full infra-down dev-user-api dev-user-rpc dev-order-api dev-inventory-rpc \
       debug debug-user-api debug-user-rpc debug-order-api debug-inventory-rpc \
       repro-deadlock

run-user-api:
	go run ./service/user/api

run-user-rpc:
	go run ./service/user/rpc

run-order-api:
	go run ./service/order/api

run-inventory-rpc:
	go run ./service/inventory/rpc

build:
	go build -o bin/user-api ./service/user/api
	go build -o bin/user-rpc ./service/user/rpc
	go build -o bin/order-api ./service/order/api
	go build -o bin/inventory-rpc ./service/inventory/rpc

test:
	go test ./... -v

clean:
	rm -rf bin/

proto:
	goctl rpc protoc api/user/v1/user.proto \
	  --proto_path=api/user/v1 \
	  --go_out=. --go-grpc_out=. \
	  --go_opt=module=github.com/fx0x55/micro-go-lab \
	  --go-grpc_opt=module=github.com/fx0x55/micro-go-lab \
	  --zrpc_out=service/user/rpc --style=goZero
	# 统一走 goctl rpc protoc：生成 pb/ + userservice/ + internal/server/。
	# logic/svc/config 因 goctl 的"存在即跳过"规则保留业务代码。
	# goctl 1.10.1 推断 pb 别名为 pb_pb（proto 路径含 user/v1），sed 修正为 pb。
	# goctl 按 package 名创建 user.v1.go / user.v1.yaml，删除（真实入口是 userrpc.go）。
	sed -i '' 's/pb_pb\./pb./g' \
	  service/user/rpc/userservice/userservice.go \
	  service/user/rpc/internal/server/userserviceserver.go \
	  service/user/rpc/internal/logic/*.go
	rm -f service/user/rpc/user.v1.go service/user/rpc/etc/user.v1.yaml

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
	KAFKA_BOOTSTRAP_SERVERS=localhost:9094,localhost:9095,localhost:9096 go run ./service/user/rpc

dev-order-api: infra
	KAFKA_BOOTSTRAP_SERVERS=localhost:9094,localhost:9095,localhost:9096 go run ./service/order/api

dev-inventory-rpc: infra
	KAFKA_BOOTSTRAP_SERVERS=localhost:9094,localhost:9095,localhost:9096 go run ./service/inventory/rpc

# === Container debug (Delve) ===
debug:
	docker compose --profile debug up -d --build

debug-user-api:
	docker compose --profile debug up -d --build user-api-debug

debug-user-rpc:
	docker compose --profile debug up -d --build user-rpc-debug

debug-order-api:
	docker compose --profile debug up -d --build order-api-debug

debug-inventory-rpc:
	docker compose --profile debug up -d --build inventory-rpc-debug

# === API code generation (goctl api go) ===
# 重新生成 types.go + routes.go（覆盖），handler/logic/svc/config 因"存在即跳过"保留。
gen-user-api:
	goctl api go -api service/user/api/user.api -dir service/user/api --style=goZero

gen-order-api:
	goctl api go -api service/order/api/order.api -dir service/order/api --style=goZero

gen-inventory-rpc:
	goctl rpc protoc api/inventory/v1/inventory.proto \
	  --proto_path=api/inventory/v1 \
	  --go_out=. --go-grpc_out=. \
	  --go_opt=module=github.com/fx0x55/micro-go-lab \
	  --go-grpc_opt=module=github.com/fx0x55/micro-go-lab \
	  --zrpc_out=service/inventory/rpc --style=goZero
	sed -i '' 's/pb_pb\./pb./g' \
	  service/inventory/rpc/inventoryservice/*.go \
	  service/inventory/rpc/internal/server/*.go \
	  service/inventory/rpc/internal/logic/*.go
	rm -f service/inventory/rpc/inventory.v1.go service/inventory/rpc/etc/inventory.v1.yaml

gen-api: gen-user-api gen-order-api
gen-rpc: gen-inventory-rpc
gen: gen-api gen-rpc

# === Troubleshooting labs ===
# 死锁 lab：起 infra（含开了 innodb_print_all_deadlocks 的 MySQL）后，跑双会话 AB-BA 复现。
# 只封装到"造事"为止；抓死锁图（SHOW ENGINE INNODB STATUS）留给读者练手，见
# docs/troubleshooting/database-deadlock.md。
repro-deadlock:
	docker compose -f docker-dev.yml up -d mysql
	@echo "等待 MySQL 就绪…"
	@for i in $$(seq 1 30); do \
	  docker compose -f docker-dev.yml exec -T mysql mysqladmin ping -h localhost -uroot -proot >/dev/null 2>&1 && break; \
	  sleep 1; \
	done
	bash deploy/mysql/deadlock-repro.sh
	@echo
	@echo "（可选）以 BUG_DB_DEADLOCK=1 跑 inventory-rpc，同时并发下两个相同 SKU 的单，"
	@echo "可观察 db_deadlocks_total 指标上涨 + Loki 里 deadlock=\"true\" 日志。"
