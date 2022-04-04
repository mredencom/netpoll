package netpoll

type Event uint32

type HandleEvent func(fd int, event Event)

const (
	// 最大的事件数量
	maxWaitEventsBegin = 1 << 10
)
