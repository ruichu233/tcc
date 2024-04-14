package txmanager

import "time"

type Options struct {
	Timeout     time.Duration // 事务处理超时阈值
	MonitorTick time.Duration //轮询任务执行时间间隔
}

type Option func(*Options)

func WithTimeout(timeout time.Duration) Option {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return func(o *Options) {
		o.Timeout = timeout
	}
}

func WithMonitorTick(tick time.Duration) Option {
	if tick <= 0 {
		tick = 10 * time.Second
	}
	return func(o *Options) {
		o.MonitorTick = tick
	}
}

func repair(o *Options) {
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Second
	}
	if o.MonitorTick <= 0 {
		o.MonitorTick = 10 * time.Second
	}
}
