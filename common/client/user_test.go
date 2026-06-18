package client

import (
	"testing"

	"github.com/fx0x55/micro-go-lab/common/config"
	"github.com/zeromicro/go-zero/core/discov"
)

// 回归测试：防止 buildRpcClientConf 漏掉 conf.FillDefault，
// 否则 Middlewares.Trace 为零值 false，order-api → user-rpc 的 gRPC 客户端
// 不会挂载 tracing/breaker/timeout 拦截器，链路追踪会在此断裂。
func TestBuildRpcClientConfEnablesTracing(t *testing.T) {
	c := buildRpcClientConf(&config.UserSvcConfig{
		Etcd:    discov.EtcdConf{Hosts: []string{"127.0.0.1:2379"}, Key: "user-svc.rpc"},
		Timeout: 2000,
	})

	if !c.Middlewares.Trace {
		t.Fatalf("Middlewares.Trace = false; FillDefault 未生效，gRPC 客户端 tracing 将被禁用")
	}
	if !c.Middlewares.Breaker {
		t.Errorf("Middlewares.Breaker = false; 客户端熔断拦截器将被禁用")
	}
	if !c.Middlewares.Timeout {
		t.Errorf("Middlewares.Timeout = false; 客户端超时拦截器将被禁用")
	}
	if !c.NonBlock {
		t.Errorf("NonBlock = false; 期望保持非阻塞拨号")
	}
}
