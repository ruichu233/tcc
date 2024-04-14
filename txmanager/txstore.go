package txmanager

import (
	"context"
	"tcc/component"
	"time"
)

type TXStore interface {
	// CreateTX 创建一条事务明细记录
	CreateTX(ctx context.Context, components ...component.TCCComponent) (txID string, err error)
	// TXUpdate 更新事务进度：
	//规则为：倘若有一个component try 操作执行失败，则整个事务失败；倘若所有 component try 操作执行成功，则整个事务成功
	TXUpdate(ctx context.Context, txID string, componentID string, accept bool) error
	// TXSubmit 提交事务的最终状态
	TXSubmit(ctx context.Context, txID string, success bool) error
	// GetHangingTXs 获取到所有处于中间态的事务
	GetHangingTXs(ctx context.Context) ([]*Transaction, error)
	// GetTX 获取指定一笔事务
	GetTX(ctx context.Context, txID string) (*Transaction, error)
	// Lock 锁住事务日志表
	Lock(ctx context.Context, expireDuration time.Duration) error
	// UnLock 解锁事务日志表
	UnLock(ctx context.Context) error
}
