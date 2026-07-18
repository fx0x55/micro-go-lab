package order

import (
	"testing"

	"github.com/fx0x55/micro-go-lab/service/order/api/internal/types"
)

// BenchmarkComputeRiskScore 用于离线验证 CPU 热点故障：
//
//	go test -bench BenchmarkComputeRiskScore -benchmem -cpuprofile cpu.prof ./service/order/api/internal/logic/order/
//	go tool pprof -http :8081 cpu.prof
//
// 这里直接调用故障函数，无需 DB/Redis/etcd 等中间件即可拿到一张可读的 CPU profile，
// 适合先离线建立"火焰图该怎么读"的直觉，再迁移到线上活体抓取。
func BenchmarkComputeRiskScore(b *testing.B) {
	b.SkipNow()
	req := &types.CreateOrderRequest{
		ProductName: "Premium Widgets Pro Max Ultra Series X-2026 Special Edition Pack",
	}
	b.ReportAllocs()
	for range b.N {
		computeRiskScore(req)
	}
	_ = cpuBugSink
}
