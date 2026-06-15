.PHONY: run-user-api run-user-rpc run-order-api build test clean docker-up docker-down proto

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
	  --go_opt=module=github.com/wokoworks/go-server \
	  --go-grpc_opt=module=github.com/wokoworks/go-server \
	  api/user/v1/user.proto
	mv api/user/v1/*.go service/user/rpc/pb/
	@sed -i '' 's/^package userv1$$/package pb/' service/user/rpc/pb/user.pb.go service/user/rpc/pb/user_grpc.pb.go
	@sed -i '' 's|"github.com/wokoworks/go-server/gen/user/v1"|"github.com/wokoworks/go-server/service/user/rpc/pb"|g' service/user/rpc/pb/user_grpc.pb.go

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down -v
