package event

// EventBus 定义事件总线接口
type EventBus interface {
	Publish(event Event)
	Subscribe(handler func(Event))
	Close()
}

// ChannelEventBus 使用channel实现的事件总线
type ChannelEventBus struct {
	ch         chan Event
	subscriber func(Event)
	done       chan struct{}
}

// NewChannelEventBus 创建新的ChannelEventBus
func NewChannelEventBus(bufferSize int) *ChannelEventBus {
	return &ChannelEventBus{
		ch:   make(chan Event, bufferSize),
		done: make(chan struct{}),
	}
}

// Publish 发布事件到总线
func (eb *ChannelEventBus) Publish(event Event) {
	eb.ch <- event
}

// Subscribe 订阅事件
func (eb *ChannelEventBus) Subscribe(handler func(Event)) {
	eb.subscriber = handler
	go eb.startConsumer()
}

// startConsumer 启动消费者协程
func (eb *ChannelEventBus) startConsumer() {
	for {
		select {
		case event := <-eb.ch:
			if eb.subscriber != nil {
				eb.subscriber(event)
			}
		case <-eb.done:
			return
		}
	}
}

// Close 关闭事件总线
func (eb *ChannelEventBus) Close() {
	close(eb.done)
}
