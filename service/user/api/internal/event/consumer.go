package event

import (
	"encoding/json"

	"github.com/zeromicro/go-zero/core/logx"
)

// Consumer 定义事件消费者
type Consumer struct {
	eventBus EventBus
}

// NewConsumer 创建新的Consumer
func NewConsumer(eventBus EventBus) *Consumer {
	return &Consumer{
		eventBus: eventBus,
	}
}

// Start 启动消费者
func (c *Consumer) Start() {
	c.eventBus.Subscribe(c.handleEvent)
}

// handleEvent 处理事件
func (c *Consumer) handleEvent(event *Event) {
	switch event.Type {
	case UserRegistered:
		c.handleUserRegistered(event)
	default:
		logx.Infof("未知的事件类型: %s", event.Type)
	}
}

// handleUserRegistered 处理用户注册事件
func (c *Consumer) handleUserRegistered(event *Event) {
	// 将payload转换为JSON
	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		logx.Errorf("序列化事件payload失败: %v", err)
		return
	}

	// 这里只是打印日志，实际应该创建欢迎待办
	logx.Infof("收到用户注册事件，准备创建欢迎待办: %s", string(payloadBytes))

	// TODO: 在实际实现中，这里应该：
	// 1. 解析payload获取user_id
	// 2. 调用TodoRepo.Create创建欢迎待办
	// 3. 处理可能的错误
}

// Stop 停止消费者
func (c *Consumer) Stop() {
	c.eventBus.Close()
}
