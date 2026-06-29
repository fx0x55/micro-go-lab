//go:build grpc

package grpc_test

// grpc_client_test.go — gRPC 学习测试集
//
// 以 user-rpc (UserService) 为目标，演示 gRPC 在 go-zero 体系下的各种特性。
// 所有测试需要 user-rpc 服务运行在 localhost:9090，并注册到 etcd（key: user-svc.rpc）。
//
// 启动服务:
//   make infra          # 启动 etcd + postgres
//   go run ./service/user/rpc -f service/user/rpc/etc/user-rpc.yaml   # 启动 user-rpc
//
// 运行测试:
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestDirectConnect
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestServiceDiscovery
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestInterceptor
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestLoadBalancing
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestMetadata
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestDeadline
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestErrorHandling
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestKeepAlive
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestHealthCheck
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestReflection
//   go test -tags grpc ./service/user/rpc/tests/grpc -v -run TestConcurrent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	userv1 "github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
)

// ============================================================================
// 基础设施
// ============================================================================

const (
	directAddr = "localhost:9090"
	etcdHost   = "localhost:2379"
	etcdKey    = "user-svc.rpc"
)

// dialDirect 直连模式：不经过服务发现，直接拨号到指定地址。
// 这是最简单的连接方式，适合开发调试。
func dialDirect(t *testing.T) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(directAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// dialViaGoZeroEtcd 通过 go-zero 的 zrpc + etcd 服务发现连接。
//
// go-zero 封装了整个流程：
//  1. RpcClientConf.BuildTarget() 解析 etcd hosts + key，生成内部 target
//  2. zrpc.NewClient 内部注册 etcd resolver（通过 resolver.BuildDiscovTarget）
//  3. gRPC resolver watch etcd key，实时感知实例上下线
//  4. Balancer（默认 p2c_ewma）根据权重和延迟选择后端
//
// 这是生产环境的标准用法。
func dialViaGoZeroEtcd(t *testing.T, opts ...zrpc.ClientOption) userv1.UserServiceClient {
	t.Helper()

	conf := zrpc.NewEtcdClientConf([]string{etcdHost}, etcdKey, "", "")
	cli := zrpc.MustNewClient(conf, opts...)

	client := userv1.NewUserServiceClient(cli.Conn())
	t.Cleanup(func() { cli.Conn().Close() })
	return client
}

func newUserClient(conn *grpc.ClientConn) userv1.UserServiceClient {
	return userv1.NewUserServiceClient(conn)
}

// ============================================================================
// 1. 直连模式 (Direct Connection)
// ============================================================================

// TestDirectConnect 演示最基础的 gRPC 调用：直连服务端地址。
//
// 要点：
//   - grpc.WithTransportCredentials(insecure.NewCredentials()) — 无 TLS
//   - 客户端直接连接指定地址，不经过服务发现
//   - 适合本地开发和调试
func TestDirectConnect(t *testing.T) {
	conn := dialDirect(t)
	client := newUserClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
	if err != nil {
		t.Logf("ValidateUser error (user may not exist, OK for learning): %v", err)
	} else {
		t.Logf("ValidateUser: exists=%v, username=%s", resp.Exists, resp.Username)
	}

	// 调用 GetUser — 获取不存在的用户，观察 gRPC 错误码
	resp2, err := client.GetUser(ctx, &userv1.GetUserRequest{UserId: 99999})
	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			t.Logf("GetUser(99999) error: code=%v, message=%s", st.Code(), st.Message())
		}
	} else {
		t.Logf("GetUser: id=%d, username=%s, email=%s", resp2.Id, resp2.Username, resp2.Email)
	}
}

// ============================================================================
// 2. 服务发现 (Service Discovery via etcd)
// ============================================================================

// TestServiceDiscovery 演示通过 etcd 服务发现连接 gRPC 服务。
//
// 原理：
//  1. 服务端启动时，go-zero 的 zrpc.Server 将自身注册到 etcd
//     key=etcd.Key, value=json{key, data:{addr, weight, ...}}
//  2. 客户端 zrpc.Client 内部 watch 这个 key（通过 resolver.BuildDiscovTarget）
//  3. 当实例上下线时，客户端自动感知并更新连接池
//
// 验证方法：
//   - 启动/停止多个 user-rpc 实例（不同端口），观察日志
//   - etcdctl get user-svc.rpc --prefix  # 查看注册信息
func TestServiceDiscovery(t *testing.T) {
	client := dialViaGoZeroEtcd(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
	if err != nil {
		t.Logf("ValidateUser via etcd error: %v", err)
	} else {
		t.Logf("ValidateUser via etcd: exists=%v, username=%s", resp.Exists, resp.Username)
	}
}

// TestServiceDiscoveryWatch 演示客户端如何感知服务实例变化。
//
// 测试方法：
//  1. 启动一个 user-rpc 实例，运行此测试
//  2. 再启动一个实例（不同端口），观察日志中后端地址增加
//  3. 停止第一个实例，观察请求自动转移到存活实例
func TestServiceDiscoveryWatch(t *testing.T) {
	client := dialViaGoZeroEtcd(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for i := range 5 {
		resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
		if err != nil {
			t.Logf("round %d: error=%v", i, err)
		} else {
			t.Logf("round %d: exists=%v", i, resp.Exists)
		}
		time.Sleep(2 * time.Second)
	}
}

// ============================================================================
// 3. 拦截器 / 中间件 (Interceptors)
// ============================================================================

// gRPC 拦截器等同于 HTTP 中间件，分为：
//   - UnaryInterceptor: 一元调用（普通 RPC）的拦截器
//   - StreamInterceptor: 流式调用的拦截器
//
// go-zero 提供了两层拦截器：
//   1. 原生 gRPC 拦截器 — 通过 grpc.WithChainUnaryInterceptor 注册
//   2. go-zero 内置拦截器 — 通过 RpcClientConf.Middlewares 启用
//      (Trace, Duration, Prometheus, Breaker, Timeout)

// --- 3a. 自定义拦截器 ---

// loggingInterceptor 记录每次 RPC 调用的方法名和耗时。
// 签名必须是 grpc.UnaryClientInterceptor：
//
//	func(ctx, method, req, reply, cc, invoker, opts...) error
//
// 调用 invoker 执行实际 RPC，可以在调用前后插入逻辑。
func loggingInterceptor(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	start := time.Now()
	logx.Infof("[grpc-client] -> %s start", method)

	err := invoker(ctx, method, req, reply, cc, opts...)

	duration := time.Since(start)
	if err != nil {
		logx.Infof("[grpc-client] <- %s failed (%v): %v", method, duration, err)
	} else {
		logx.Infof("[grpc-client] <- %s ok (%v)", method, duration)
	}
	return err
}

// authInterceptor 在每个请求中注入认证 metadata。
// go-zero 服务端 JWT 中间件从 incoming metadata 提取 token，
// 客户端通过拦截器统一注入 outgoing metadata。
func authInterceptor(
	ctx context.Context,
	method string,
	req, reply any,
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	// 从 context 中取出已有的 outgoing metadata，或创建新的
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	// 注入 JWT token（实际场景应从 token 管理器获取）
	md.Set("authorization", "Bearer example-token-12345")
	ctx = metadata.NewOutgoingContext(ctx, md)

	return invoker(ctx, method, req, reply, cc, opts...)
}

// TestCustomInterceptor 演示自定义拦截器的注册。
//
// go-zero 提供两种方式注册自定义拦截器：
//  1. grpc.WithChainUnaryInterceptor — 仅适用于原生 grpc.NewClient
//  2. zrpc.WithUnaryClientInterceptor — 适用于 zrpc.MustNewClient
//
// 拦截器按注册顺序执行（洋葱模型）：logging 先于 auth。
func TestCustomInterceptor(t *testing.T) {
	// 方式 1：原生 gRPC 拦截器（适用于 grpc.NewClient）
	conn, err := grpc.NewClient(directAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			loggingInterceptor,
			authInterceptor,
		),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	client := newUserClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
	if err != nil {
		t.Logf("error: %v", err)
	} else {
		t.Logf("response: exists=%v", resp.Exists)
	}
}

// TestZrpcInterceptor 演示通过 zrpc 注册拦截器（go-zero 推荐方式）。
//
// zrpc.WithUnaryClientInterceptor 内部会和 go-zero 内置拦截器
// (Breaker, Timeout, Trace...) 一起组成拦截器链。
func TestZrpcInterceptor(t *testing.T) {
	client := dialViaGoZeroEtcd(t,
		zrpc.WithUnaryClientInterceptor(loggingInterceptor),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
	if err != nil {
		t.Logf("error: %v", err)
	} else {
		t.Logf("response: exists=%v", resp.Exists)
	}
}

// --- 3b. go-zero 内置拦截器配置参考 ---
//
// 在 YAML 配置中可以启用的内置拦截器：
//
// RpcClientConf:
//   Timeout: 5000              # 连接超时（ms）
//   Middlewares:
//     Timeout: true            # 调用超时拦截器
//     Duration: true           # 调用耗时日志
//     Prometheus: true         # Prometheus 指标
//     Breaker: true            # 熔断器（连续失败超阈值后快速失败）
//     Trace: true              # 分布式追踪
//
// go-zero 服务端拦截器 (etc/user-rpc.yaml):
// RpcServerConf:
//   Middlewares:
//     Timeout: true            # 服务端超时保护
//     Breaker: true            # 并发限制
//     Recover: true            # panic 恢复
//     Stat: true               # 请求统计

// ============================================================================
// 4. 负载均衡 (Load Balancing)
// ============================================================================

// gRPC 负载均衡的核心概念：
//
//  Name Resolver                      Balancer
//  ┌──────────────────┐     ┌──────────────────────┐
//  │ 解析 target 为    │────>│ 根据策略选择一个后端   │
//  │ 多个后端地址列表   │     │ 发送 RPC 请求          │
//  └──────────────────┘     └──────────────────────┘
//
// go-zero 的默认策略: p2c_ewma (Power of Two Choices + EWMA)
//   - 随机选 2 个后端，选延迟较低的那个
//   - EWMA 加权平均近期延迟，对慢后端自适应降权
//   - 比简单 round_robin 更智能，能自动回避慢实例
//
// 其他策略：
//   - round_robin: 轮询所有后端（适合后端性能均匀的场景）
//   - pick_first: 只用第一个后端（适合只有一个实例）
//
// 配置方式（RpcClientConf）：
//   BalancerName: "round_robin"  # 或 "p2c_ewma"（默认）

// TestLoadBalancingP2cEwma 演示 go-zero 默认的 p2c_ewma 负载均衡。
//
// p2c_ewma 的优势：
//   - 随机选 2 个后端，对比延迟后选快的那个
//   - 不需要集中式调度，每个客户端独立决策
//   - 对慢实例自适应（EWMA 衰减），自动降权
//
// 测试方法：
//  1. 启动多个 user-rpc 实例（不同端口、不同权重）
//  2. 运行此测试，观察请求分布
//  3. 给某个实例制造延迟（如 sleep），观察流量自动偏移
func TestLoadBalancingP2cEwma(t *testing.T) {
	// 不设置 BalancerName，使用默认的 p2c_ewma
	client := dialViaGoZeroEtcd(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := range 5 {
		resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
		if err != nil {
			t.Logf("round %d: error=%v", i, err)
		} else {
			t.Logf("round %d: exists=%v", i, resp.Exists)
		}
	}
}

// TestLoadBalancingRoundRobin 演示 round_robin 负载均衡。
//
// 测试方法：
//  1. 启动多个 user-rpc 实例（不同端口）
//  2. etcdctl get user-svc.rpc --prefix  # 确认有多个实例
//  3. 运行此测试，观察日志中不同后端被交替使用
func TestLoadBalancingRoundRobin(t *testing.T) {
	// BalancerName: "round_robin"
	// go-zero 通过 RpcClientConf.BalancerName 设置 gRPC balancer
	cli := zrpc.MustNewClient(zrpc.RpcClientConf{
		Etcd:         zrpc.NewEtcdClientConf([]string{etcdHost}, etcdKey, "", "").Etcd,
		Timeout:      3000,
		BalancerName: "round_robin",
		NonBlock:     true,
		Middlewares: zrpc.ClientMiddlewaresConf{
			Trace:    true,
			Duration: true,
		},
	})
	t.Cleanup(func() { cli.Conn().Close() })

	client := userv1.NewUserServiceClient(cli.Conn())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := range 5 {
		resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
		if err != nil {
			t.Logf("round %d: error=%v", i, err)
		} else {
			t.Logf("round %d: exists=%v", i, resp.Exists)
		}
	}
}

// TestLoadBalancingPickFirst 演示 pick_first 策略。
//
// pick_first 只连接第一个可用的后端，所有请求打到同一个实例。
// 适合只有一个服务实例，或者你希望保证请求亲和性。
func TestLoadBalancingPickFirst(t *testing.T) {
	cli := zrpc.MustNewClient(zrpc.RpcClientConf{
		Etcd:         zrpc.NewEtcdClientConf([]string{etcdHost}, etcdKey, "", "").Etcd,
		Timeout:      3000,
		BalancerName: "pick_first",
		NonBlock:     true,
		Middlewares: zrpc.ClientMiddlewaresConf{
			Trace:    true,
			Duration: true,
		},
	})
	t.Cleanup(func() { cli.Conn().Close() })

	client := userv1.NewUserServiceClient(cli.Conn())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := range 3 {
		resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
		if err != nil {
			t.Logf("round %d: error=%v", i, err)
		} else {
			t.Logf("round %d: exists=%v", i, resp.Exists)
		}
	}
}

// --- 负载均衡实战场景 ---
//
// 场景1: 蓝绿部署
//   新版本注册到新 key (user-svc.rpc.canary)，客户端切换 key 逐步切流。
//
// 场景2: 灰度发布
//   新旧版本注册到同一 key，通过 Weight 控制流量比例：
//     旧版本 Weight=100，新版本 Weight=10
//     p2c_ewma 会按权重分配流量，同时根据实际延迟自适应调整。
//
// 场景3: K8s 滚动更新
//   MaxConnectionAge 让客户端定期重建连接，发现新 Pod。
//   配合 Weight 渐进调整，实现零停机发布。

// ============================================================================
// 5. Metadata（请求头）
// ============================================================================

// TestMetadata 演示 gRPC metadata 的使用。
//
// metadata 是 gRPC 的请求头机制，等同于 HTTP headers。
// 常见用途：
//   - authorization: JWT token
//   - x-request-id / x-trace-id: 链路追踪
//   - x-tenant-id: 多租户隔离
//
// 服务端读取方式：
//
//	md, ok := metadata.FromIncomingContext(ctx)
//	token := md.Get("authorization")  // []string
func TestMetadata(t *testing.T) {
	conn := dialDirect(t)
	client := newUserClient(conn)

	ctx := context.Background()

	// 构造 outgoing metadata（发送给服务端）
	md := metadata.New(map[string]string{
		"x-request-id":  "test-req-001",
		"x-trace-id":    "trace-abc-123",
		"authorization": "Bearer some-jwt-token",
		"x-tenant-id":   "tenant-001",
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
	if err != nil {
		t.Logf("ValidateUser with metadata error: %v", err)
	} else {
		t.Logf("ValidateUser with metadata: exists=%v", resp.Exists)
	}
}

// TestMetadataAppend 演示追加 metadata（而非覆盖）。
//
// New 会替换已有的 outgoing metadata。
// AppendToOutgoingContext 追加键值对，保留已有的 metadata。
// gRPC 允许同名 metadata 有多个值（类似 HTTP headers 的多值）。
func TestMetadataAppend(t *testing.T) {
	conn := dialDirect(t)
	client := newUserClient(conn)

	ctx := context.Background()

	// 先设置基础 metadata
	md := metadata.New(map[string]string{
		"x-request-id": "req-001",
	})
	ctx = metadata.NewOutgoingContext(ctx, md)

	// 追加更多 metadata（不覆盖已有的 x-request-id）
	ctx = metadata.AppendToOutgoingContext(ctx,
		"x-trace-id", "trace-001",
		"x-tenant-id", "tenant-001",
	)

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
	if err != nil {
		t.Logf("error: %v", err)
	} else {
		t.Logf("response: exists=%v", resp.Exists)
	}
}

// ============================================================================
// 6. Deadline / Timeout（超时控制）
// ============================================================================

// TestDeadline 演示 gRPC 的 deadline 传播机制。
//
// gRPC 的超时与 HTTP 有本质区别：
//   - HTTP: 客户端超时和服务端超时各自独立
//   - gRPC: 客户端设置 deadline，通过 grpc-timeout header 传递给服务端，
//     服务端共享同一个 deadline。服务端处理超时，两端同时取消。
//
// 这种机制确保：
//   - 不会出现"客户端已放弃但服务端还在处理"的浪费
//   - 级联调用（order-api -> user-rpc）deadline 一路传递
//
// go-zero 服务端读取 deadline:
//
//	deadline, ok := ctx.Deadline()
//	remaining := time.Until(deadline)
func TestDeadline(t *testing.T) {
	conn := dialDirect(t)
	client := newUserClient(conn)

	t.Run("normal_timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		start := time.Now()
		resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
		if err != nil {
			t.Logf("error after %v: %v", time.Since(start), err)
		} else {
			t.Logf("ok after %v: exists=%v", time.Since(start), resp.Exists)
		}
	})

	t.Run("very_short_timeout", func(t *testing.T) {
		// 极短超时，触发 DeadlineExceeded
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		_, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
		if err != nil {
			st, _ := status.FromError(err)
			t.Logf("code=%v, message=%s", st.Code(), st.Message())
			if st.Code() == codes.DeadlineExceeded {
				t.Log("Got expected DeadlineExceeded error")
			}
		} else {
			t.Log("Response received (server very fast)")
		}
	})

	t.Run("with_deadline_at", func(t *testing.T) {
		// WithDeadline: 绝对时间点，超过后 context 自动取消
		deadline := time.Now().Add(2 * time.Second)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()

		resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
		if err != nil {
			t.Logf("error: %v", err)
		} else {
			t.Logf("ok: exists=%v", resp.Exists)
		}
	})
}

// ============================================================================
// 7. 错误处理 (Error Handling)
// ============================================================================

// TestErrorHandling 演示 gRPC 的结构化错误处理。
//
// gRPC 使用 status.Code 表示错误类型，比 HTTP status code 更精确：
//
//	OK                (0)  成功
//	InvalidArgument   (3)  参数校验失败
//	NotFound          (5)  资源不存在
//	AlreadyExists     (6)  资源已存在
//	PermissionDenied  (7)  无权限
//	Unauthenticated  (16)  未认证
//	Internal         (13)  服务端内部错误
//	Unavailable      (14)  服务不可用（熔断、连接失败）
//	DeadlineExceeded  (4)  超时
//
// 服务端设置：
//
//	return status.Error(codes.NotFound, "user not found")
//
// 客户端解析：
//
//	st, ok := status.FromError(err)
//	switch st.Code() { ... }
func TestErrorHandling(t *testing.T) {
	conn := dialDirect(t)
	client := newUserClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	t.Run("not_found", func(t *testing.T) {
		_, err := client.GetUser(ctx, &userv1.GetUserRequest{UserId: 999999})
		if err != nil {
			st, ok := status.FromError(err)
			if ok {
				t.Logf("code=%v, message=%s", st.Code(), st.Message())
			}
		}
	})

	t.Run("status_details", func(t *testing.T) {
		// status.Error 可以携带附加详情（通过 proto Any）
		_, err := client.GetUser(ctx, &userv1.GetUserRequest{UserId: 0})
		if err != nil {
			st, ok := status.FromError(err)
			if ok {
				t.Logf("code=%v, message=%s", st.Code(), st.Message())
				// st.Details() 返回附加的 proto 消息
				for _, d := range st.Details() {
					t.Logf("  detail: %T %v", d, d)
				}
			}
		}
	})
}

// ============================================================================
// 8. Keep-Alive（连接保活）
// ============================================================================

// TestKeepAlive 演示 gRPC 连接保活配置。
//
// gRPC 底层使用 HTTP/2 长连接，keep-alive 控制连接的存活检测。
//
// 客户端参数:
//
//	Time:                空闲多久后发送 ping（gRPC 强制最小 10s）
//	Timeout:             ping 后等待响应的超时时间
//	PermitWithoutStream: 无活跃 stream 时是否发送 ping
//
// 服务端参数 (go-zero RpcServerConf):
//
//	KeepaliveTime:       期望的 ping 间隔
//	KeepaliveTimeout:    ping 超时
//	MaxConnectionAge:    连接最大存活时间（定期强制关闭，让客户端重连到新实例）
//	MaxConnectionAgeGrace: 强制关闭后的宽限期
//
// 为什么需要 MaxConnectionAge？
//
//	K8s 滚动更新时，旧 Pod 下线后客户端连接不会自动断开。
//	设置 MaxConnectionAge 后，客户端定期重建连接，发现新 Pod。
func TestKeepAlive(t *testing.T) {
	conn, err := grpc.NewClient(directAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second, // 空闲 10s 后发 ping
			Timeout:             3 * time.Second,  // ping 超时
			PermitWithoutStream: true,             // 无 stream 时也保活
		}),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	client := newUserClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: 1})
	if err != nil {
		t.Logf("error: %v", err)
	} else {
		t.Logf("ok: exists=%v", resp.Exists)
	}
}

// ============================================================================
// 9. Health Check（健康检查）
// ============================================================================

// TestHealthCheck 演示 gRPC 标准健康检查协议 (grpc.health.v1.Health)。
//
// go-zero 在 zrpc.Server 中内置了 health service 注册（Health: true）。
//
// 健康检查的用途：
//   - K8s liveness/readiness probe
//   - 负载均衡器判断后端是否可用
//   - 服务网格（Istio/Linkerd）探测后端健康状态
func TestHealthCheck(t *testing.T) {
	conn := dialDirect(t)
	healthClient := grpc_health_v1.NewHealthClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	t.Run("overall_health", func(t *testing.T) {
		// service_name 为空：查询整体健康状态
		resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		if err != nil {
			t.Logf("health check error: %v", err)
		} else {
			t.Logf("overall health: %v", resp.Status)
		}
	})

	t.Run("specific_service", func(t *testing.T) {
		// 查询特定服务的健康状态
		resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
			Service: "user.v1.UserService",
		})
		if err != nil {
			t.Logf("service health check error: %v", err)
		} else {
			t.Logf("user.v1.UserService health: %v", resp.Status)
		}
	})
}

// ============================================================================
// 10. Reflection（服务反射）
// ============================================================================

// TestReflection 演示 gRPC 服务反射。
//
// reflection 允许客户端在运行时查询服务的 proto 定义：
//   - 列出所有注册的服务
//   - 列出每个服务的所有方法
//   - 获取每个方法的请求/响应类型
//
// go-zero 在 dev/test 模式下自动启用 reflection（通过 reflection.Register）。
// 生产环境建议关闭，避免暴露内部 API 结构。
//
// 不写代码也能用（grpcurl 工具）：
//
//	grpcurl -plaintext localhost:9090 list
//	grpcurl -plaintext localhost:9090 describe user.v1.UserService
//	grpcurl -plaintext -d '{"user_id":1}' localhost:9090 user.v1.UserService/ValidateUser
func TestReflection(t *testing.T) {
	conn := dialDirect(t)
	refClient := rpb.NewServerReflectionClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := refClient.ServerReflectionInfo(ctx)
	if err != nil {
		t.Fatalf("reflection stream failed: %v", err)
	}

	// 请求服务列表
	err = stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_ListServices{},
	})
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv failed: %v", err)
	}

	if listResp := resp.GetListServicesResponse(); listResp != nil {
		t.Log("=== Registered gRPC Services ===")
		for _, svc := range listResp.GetService() {
			t.Logf("  Service: %s", svc.Name)
		}
	}
}

// ============================================================================
// 11. 并发调用 (Concurrent Calls)
// ============================================================================

// TestConcurrentCalls 演示 gRPC 客户端的并发特性。
//
// gRPC 客户端线程安全，多个 goroutine 可以共用同一个连接。
// 底层 HTTP/2 多路复用：
//   - 多个 stream 共享同一个 TCP 连接
//   - 每个 stream 有唯一的 stream ID
//   - 请求和响应可以交错传输
//
// 与 HTTP/1.1 连接池模型不同（HTTP/1.1 一个连接同时只处理一个请求）。
func TestConcurrentCalls(t *testing.T) {
	conn := dialDirect(t)
	client := newUserClient(conn)

	var wg sync.WaitGroup
	results := make(chan string, 10)

	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			resp, err := client.ValidateUser(ctx, &userv1.ValidateUserRequest{UserId: uint64(id)})
			if err != nil {
				results <- fmt.Sprintf("goroutine %d: error=%v", id, err)
			} else {
				results <- fmt.Sprintf("goroutine %d: exists=%v", id, resp.Exists)
			}
		}(i)
	}

	wg.Wait()
	close(results)

	for r := range results {
		t.Log(r)
	}
}

// ============================================================================
// 12. go-zero gRPC 完整配置参考
// ============================================================================

// 以下不是测试代码，而是 go-zero gRPC 配置速查。
//
// --- RpcClientConf（客户端）---
//
// ```yaml
// UserSvc:
//   Etcd:                          # etcd 服务发现
//     Hosts:
//       - localhost:2379
//     Key: user-svc.rpc
//   Endpoints:                     # 直连模式（与 Etcd 二选一）
//     - localhost:9090
//   NonBlock: true                 # 非阻塞连接（启动时不等待服务端就绪）
//   Timeout: 5000                  # 连接超时（ms）
//   BalancerName: round_robin      # 负载均衡策略 (默认 p2c_ewma)
//   KeepaliveTime: 10s             # keep-alive 间隔
//   Middlewares:
//     Timeout: true                # 调用超时拦截器
//     Duration: true               # 调用耗时日志
//     Prometheus: true             # Prometheus 指标
//     Breaker: true                # 熔断器
//     Trace: true                  # 分布式追踪
// ```
//
// --- RpcServerConf（服务端）---
//
// ```yaml
// RpcServerConf:
//   Name: user-rpc
//   ListenOn: 0.0.0.0:9090
//   Etcd:                          # 注册到 etcd
//     Hosts:
//       - localhost:2379
//     Key: user-svc.rpc
//   Weight: 100                    # 负载均衡权重（可动态调整）
//   MaxConns: 500                  # 最大并发连接数
//   Timeout: 3000                  # 调用超时（ms）
//   CpuThreshold: 900              # CPU 超阈值自动降低权重
//   Health: true                   # 启用 health check
//   Middlewares:
//     Timeout: true                # 服务端超时保护
//     Breaker: true                # 并发限制
//     Recover: true                # panic 恢复
//     Trace: true                  # 分布式追踪
//     Duration: true               # 调用耗时
//     Prometheus: true             # Prometheus 指标
// ```

// ============================================================================
// 13. grpcurl 命令行工具参考
// ============================================================================

// 安装:
//   go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
//
// 列出所有服务:
//   grpcurl -plaintext localhost:9090 list
//
// 查看服务详情:
//   grpcurl -plaintext localhost:9090 describe user.v1.UserService
//
// 查看方法签名:
//   grpcurl -plaintext localhost:9090 describe user.v1.ValidateUserRequest
//
// 调用方法:
//   grpcurl -plaintext -d '{"user_id":1}' \
//     localhost:9090 user.v1.UserService/ValidateUser
//
// 带 metadata 调用:
//   grpcurl -plaintext \
//     -H "authorization: Bearer <token>" \
//     -d '{"user_id":1}' \
//     localhost:9090 user.v1.UserService/ValidateUser
//
// 通过 etcd 获取实际地址（grpcurl 不支持 go-zero discov）:
//   etcdctl get user-svc.rpc --prefix  # 解析出 addr 字段
