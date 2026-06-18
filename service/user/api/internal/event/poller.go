package event

import (
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// Poller 定义事件轮询器
type Poller struct {
	outbox   *Outbox
	interval time.Duration
	done     chan struct{}
}

// NewPoller 创建新的Poller
func NewPoller(outbox *Outbox, interval time.Duration) *Poller {
	return &Poller{
		outbox:   outbox,
		interval: interval,
		done:     make(chan struct{}),
	}
}

// Start 启动轮询器
func (p *Poller) Start() {
	go p.poll()
}

// poll 轮询Outbox并发布事件
func (p *Poller) poll() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			published := p.outbox.PublishPending()
			if published > 0 {
				logx.Infof("发布了 %d 个待处理事件", published)
			}
		case <-p.done:
			return
		}
	}
}

// Stop 停止轮询器
func (p *Poller) Stop() {
	close(p.done)
}
