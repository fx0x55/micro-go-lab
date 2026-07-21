package order

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/fx0x55/micro-go-lab/service/order/api/internal/types"
)

// ──────────────────────────────────────────────────────────────────────────
// 故障注入（troubleshooting lab）
//
// 本文件用于"线上 CPU / 内存 / goroutine 排查"教学。仅在对应 BUG_* 环境变量
// 为 1 时生效，默认关闭，不影响正常开发与生产流程。
//
// 注入点都放在真实业务路径（创建订单）里，形态与线上常见 bug 一致：
//   - BUG_CPU=1         CPU 热点：热路径里的 O(n^2) 低效实现
//   - BUG_MEMLEAK=1     内存泄漏：全局缓存无淘汰策略（后续切片实现）
//   - BUG_GOROUTINE=1   goroutine 泄漏：异步处理丢 context、卡在无人收的 channel（后续切片实现）
//
// 用法示例：
//
//	BUG_CPU=1 make dev-order-api
//	BUG_CPU=1 BUG_CPU_ITERS=4000 go run ./service/order/api
// ──────────────────────────────────────────────────────────────────────────

// cpuBugSink 充当"黑洞"，防止编译器把只写不读的热循环优化掉（死代码消除）。
var cpuBugSink int

// computeRiskScore 模拟"反欺诈风控评分"：对商品名做重复特征扫描。
//
// 真实线上这类 bug 的典型形态：
//   - 在请求热路径里做了 O(n^2) 的模式匹配（双层循环扫重复片段）
//   - 没意识到输入长度会让单次请求耗 CPU 达数十~数百毫秒
//   - 常规压测流量小看不出问题；真实流量一上来 CPU 立刻打满
//
// 这里把单次 O(n^2) 扫描用外层循环放大，product_name 较长时单次请求约 50~150ms，
// 足以在 30s CPU profile 中成为一眼可见的热点。
//
// 为什么不用正则 ReDoS：Go 的 regexp 是 RE2 引擎，对回溯类 ReDoS 免疫，
// 想用正则烧 CPU 在 Go 里行不通——这一点本身也是排查时的一个 D2 要点。
func computeRiskScore(req *types.CreateOrderRequest) {
	const defaultIters = 50000
	iterations := defaultIters
	if v := os.Getenv("BUG_CPU_ITERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			iterations = n
		}
	}

	name := req.ProductName
	score := 0
	for range iterations {
		n := len(name)
		for i := range n {
			for j := i + 1; j < n; j++ {
				if name[i] == name[j] {
					score++
				}
			}
		}
	}
	cpuBugSink = score // 写入包级 sink，避免死代码消除
}

// ──────────────────────────────────────────────────────────────────────────
// 内存泄漏故障（troubleshooting lab）
//
// 模拟线上最常见的内存泄漏形态：全局缓存无淘汰策略（只进不出）。
// 每个 idempotency key 缓存一条"风控画像"，永不清除，堆持续线性增长。
//
// 这类 bug 线上最典型的特征：
//   - 对象本身不大，但 value 里带了 blob（整个请求 body / DB 行 / 图片缩略图），
//     单条就吃掉几十 KB，缓存条目一多堆就爆
//   - 本地缓存忘了设 TTL / 上限，或 TTL 逻辑有 bug 实际没生效
//   - 用 sync.Map / map+Mutex 做缓存，却没有任何淘汰路径
// ──────────────────────────────────────────────────────────────────────────

// riskProfileCache 全局风控画像缓存，只在 BUG_MEMLEAK=1 时写入。
// 故意不做任何淘汰，演示"只进不出"的堆增长。
var (
	riskProfileCache = make(map[string]*riskProfile)
	riskProfileMu    sync.Mutex
)

// riskProfile 模拟一条被缓存的风控画像。
// Snapshot 字段模拟"不小心把原始 payload / 会话快照一起缓存了"——
// 这正是线上内存泄漏的高频根因：缓存对象看似小，里面的 blob 才是大头。
type riskProfile struct {
	UserID    uint
	Reason    string
	Score     int
	CreatedAt time.Time
	Snapshot  []byte // 模拟缓存的原始 payload，默认 32KB，可用 BUG_MEMLEAK_BYTES 调整
}

// cacheRiskProfile 把一条画像塞进全局缓存，永不淘汰。
// 用 idempotency key 做 cache key（每个请求唯一），保证每条都新增、堆只增不减。
func cacheRiskProfile(userID uint, req *types.CreateOrderRequest, key string) {
	if key == "" {
		key = req.Sku + "|" + req.ProductName // 兜底：尽量让 key 唯一
	}
	sz := 32 * 1024
	if v := os.Getenv("BUG_MEMLEAK_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sz = n
		}
	}
	snap := make([]byte, sz)
	for i := range snap {
		snap[i] = byte(i) // 填非零值，避免被识别为可回收的零页
	}
	p := &riskProfile{
		UserID:    userID,
		Reason:    "auto-cached",
		Score:     0,
		CreatedAt: time.Now(),
		Snapshot:  snap,
	}
	riskProfileMu.Lock()
	riskProfileCache[key] = p
	riskProfileMu.Unlock()
}
