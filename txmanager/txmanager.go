package txmanager

import (
	"context"
	"errors"
	"fmt"
	"tcc/log"
	"time"

	"sync"
	"tcc/component"
)

type TXManager struct {
	ctx            context.Context
	stop           context.CancelFunc
	opts           *Options
	txStore        TXStore
	registryCenter *registryCenter
}

func NewTXManager(store TXStore, opts ...Option) *TXManager {
	ctx, cancel := context.WithCancel(context.Background())
	txManager := &TXManager{
		opts:           &Options{},
		txStore:        store,
		registryCenter: newRegistryCenter(),
		ctx:            ctx,
		stop:           cancel,
	}

	for _, opt := range opts {
		opt(txManager.opts)
	}
	// 兜底txManager的默认选项
	repair(txManager.opts)

	// 启动异步轮询任务
	go txManager.run()
	return txManager
}

func (t *TXManager) Stop() {
	t.stop()
}

func (t *TXManager) Register(component component.TCCComponent) error {
	return t.registryCenter.register(component)
}

// Transaction 事务
func (t *TXManager) Transaction(ctx context.Context, reqs ...*RequestEntity) (bool, error) {
	// 1. 限时分布式事务的执行时长
	tctx, cancel := context.WithTimeout(ctx, t.opts.Timeout)
	defer cancel()

	// 2. 获得所有涉及到分布式 tcc 组件
	componentEntities, err := t.getComponents(tctx, reqs...)
	if err != nil {
		return false, err
	}

	// 3.创建事务明细记录，并获取全局唯一的事务ID
	txID, err := t.txStore.CreateTX(ctx, componentEntities.ToComponents()...)
	if err != nil {
		return false, err
	}

	// 4. 执行两阶段提交
	return t.twoPhaseCommit(ctx, txID, componentEntities)
}

func (t *TXManager) getComponents(ctx context.Context, reqs ...*RequestEntity) (ComponentEntities, error) {
	if len(reqs) > 0 {
		return nil, errors.New("empty task")
	}

	// 判断合法性

	// 判断是否有重复 tcc组件 id
	idToReq := make(map[string]*RequestEntity, len(reqs))
	componentIDS := make([]string, 0, len(reqs))
	for _, req := range reqs {
		if _, ok := idToReq[req.ComponentID]; ok {
			return nil, fmt.Errorf("repeat component: %s", req.ComponentID)
		}
		idToReq[req.ComponentID] = req
		componentIDS = append(componentIDS, req.ComponentID)
	}

	// 判断注册中心是否包含请求中组件
	components, err := t.registryCenter.getComponents(componentIDS...)
	if err != nil {
		return nil, err
	}
	if len(components) != len(componentIDS) {
		return nil, errors.New("invalid componentIDs")
	}

	entities := make(ComponentEntities, 0, len(components))

	for _, component := range components {
		entities = append(entities, &ComponentEntity{
			Request:   idToReq[component.ID()].Request,
			Component: component,
		})
	}
	return entities, nil

}

func (t *TXManager) twoPhaseCommit(ctx context.Context, txID string, componentEntities ComponentEntities) (bool, error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error)
	go func() {
		wg := &sync.WaitGroup{}
		for _, componentEntity := range componentEntities {
			wg.Add(1)
			componentEntity := componentEntity
			go func() {
				defer wg.Done()
				resp, err := componentEntity.Component.Try(cctx, &component.TCCReq{
					ComponentID: componentEntity.Component.ID(),
					TXID:        txID,
					Data:        componentEntity.Request,
				})

				if err != nil || !resp.ACK {
					log.ErrorContextf(cctx, "tx try failed, tx id is:%s, component id is %s, err is %v", txID, componentEntity.Component.ID(), err)
					if _err := t.txStore.TXUpdate(cctx, txID, componentEntity.Component.ID(), resp.ACK); _err != nil {
						log.ErrorContextf(cctx, "tx updated failed, tx id: %s, component id: %s, err: %v", txID, componentEntity.Component.ID(), _err)
					}
					errCh <- fmt.Errorf("component: %s try failed", componentEntity.Component.ID())
					return
				}

				if err := t.txStore.TXUpdate(cctx, txID, componentEntity.Component.ID(), resp.ACK); err != nil {
					log.ErrorContextf(cctx, "tx updated failed, tx id: %s, component id: %s, err: %v", txID, componentEntity.Component.ID(), err)
					errCh <- fmt.Errorf("component: %s try failed", componentEntity.Component.ID())
				}
			}()
		}
		wg.Wait()
		close(errCh)
	}()

	succ := true
	// 只要一个try失败就关闭其他协程
	if err := <-errCh; err != nil {
		cancel()
		succ = false
	}

	// 执行第二阶段
	go t.advanceProgressByTXID(txID)

	return succ, nil
}

func (t *TXManager) advanceProgressByTXID(txID string) error {
	// 获取事务日志明细
	tx, err := t.txStore.GetTX(t.ctx, txID)
	if err != nil {
		return err
	}

	// 推进进度
	return t.advanceProgress(tx)

}

func (t *TXManager) advanceProgress(tx *Transaction) error {
	txStatus := tx.getStatus(time.Now().Add(-t.opts.Timeout))
	// hanging 事务暂时不处理
	if txStatus == TXHanging {
		return nil
	}

	succ := txStatus == TXSuccess
	var confirmOrCancel func(ctx context.Context, component component.TCCComponent) (*component.TCCResp, error)
	var txAdvanceProgress func(ctx context.Context) error
	if succ {
		confirmOrCancel = func(ctx context.Context, component component.TCCComponent) (*component.TCCResp, error) {
			return component.Confirm(ctx, component.ID())
		}
		txAdvanceProgress = func(ctx context.Context) error {
			return t.txStore.TXSubmit(ctx, tx.TXID, true)
		}
	} else {
		confirmOrCancel = func(ctx context.Context, component component.TCCComponent) (*component.TCCResp, error) {
			return component.Cancel(ctx, component.ID())
		}
		txAdvanceProgress = func(ctx context.Context) error {
			return t.txStore.TXSubmit(ctx, tx.TXID, false)
		}
	}

	for _, componentTryEntry := range tx.components {
		// 获取对应 tcc 组件
		tccComponents, err := t.registryCenter.getComponents(componentTryEntry.ComponentID)
		if err != nil || len(tccComponents) == 0 {
			return errors.New("get tcc component failed")
		}
		resp, err := confirmOrCancel(t.ctx, tccComponents[0])
		if err != nil {
			return err
		}
		if !resp.ACK {
			return fmt.Errorf("component: %s ack failed", componentTryEntry.ComponentID)
		}
	}

	return txAdvanceProgress(t.ctx)

}

func (t *TXManager) run() {
	var tick time.Duration
	var err error
	for {
		if err == nil {
			tick = t.opts.MonitorTick
		} else {
			// 出现失败，tick需要退避
			tick = t.backOffTick(tick)
		}
		select {
		case <-t.ctx.Done():
			return
		case <-time.After(tick):
			// 加锁
			if err = t.txStore.Lock(t.ctx, t.opts.Timeout); err != nil {
				// 加锁失败，大概率锁被其他节点占有，不进行退避升级
				err = nil
				continue
			}

			// 获取处于hanging状态的事务
			var txs []*Transaction
			if txs, err = t.txStore.GetHangingTXs(t.ctx); err != nil {
				_ = t.txStore.UnLock(t.ctx)
				continue
			}
			err = t.batchAdvanceProgress(txs)
			_ = t.txStore.UnLock(t.ctx)
		}
	}

}

func (t *TXManager) backOffTick(tick time.Duration) time.Duration {
	if tick > t.opts.MonitorTick<<3 {
		return tick
	}
	tick = tick << 1
	return tick
}

func (t *TXManager) batchAdvanceProgress(txs []*Transaction) error {
	// 对每笔事务进行状态推进
	errCh := make(chan error)

	go func() {
		// 并发执行，推进各事务的进度
		var wg sync.WaitGroup
		for _, tx := range txs {
			tx := tx
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := t.advanceProgress(tx); err != nil {
					errCh <- err
				}
			}()
		}
		wg.Wait()
		close(errCh)
	}()
	var firstErr error
	// 通过 chan 阻塞在这里，直到所有携程完成,chan 被 close 才能继续
	for err := range errCh {
		if firstErr != nil {
			continue
		}
		firstErr = err
	}
	return firstErr

}
