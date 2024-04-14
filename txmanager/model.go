package txmanager

import (
	"tcc/component"
	"time"
)

type RequestEntity struct {
	// 组件名称
	ComponentID string `json:"componentName"`
	// Try 请求时传递德参数
	Request map[string]interface{} `json:"request"`
}

type ComponentEntities []*ComponentEntity

func (ce *ComponentEntities) ToComponents() []component.TCCComponent {
	tccComponents := make([]component.TCCComponent, 0, len(*ce))
	for _, entity := range *ce {
		tccComponents = append(tccComponents, entity.Component)
	}
	return tccComponents
}

type ComponentEntity struct {
	Request   map[string]interface{}
	Component component.TCCComponent
}

// 事务状态
type TXStatus string

const (
	// 事务执行中
	TXHanging TXStatus = "hanging"
	// 事务失败
	TxFailure TXStatus = "failure"
	// 事务成功
	TXSuccess TXStatus = "success"
)

func (t TXStatus) String() string {
	return string(t)
}

type ComponentTryStatus string

const (
	TryHanging ComponentTryStatus = "hanging"
	TryFailure ComponentTryStatus = "failure"
	TrySuccess ComponentTryStatus = "success"
)

func (cts ComponentTryStatus) String() string {
	return string(cts)
}

type ComponentTryEntity struct {
	ComponentID string
	TryStatus   ComponentTryStatus
}

// 事务
type Transaction struct {
	TXID       string `json:"txID"`
	components []*ComponentTryEntity
	Status     TXStatus  `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
}

func NewTransaction(txID string, componentEntities ComponentEntities) *Transaction {
	entities := make([]*ComponentTryEntity, 0, len(componentEntities))
	for _, entity := range componentEntities {
		entities = append(entities, &ComponentTryEntity{
			ComponentID: entity.Component.ID(),
		})
	}
	return &Transaction{
		TXID:       txID,
		components: entities,
	}
}

func (t *Transaction) getStatus(createdBefore time.Time) TXStatus {

	// 事务超时
	if t.CreatedAt.Before(createdBefore) {
		return TxFailure
	}

	var hangingExist bool
	for _, component := range t.components {
		if component.TryStatus == TryFailure {
			return TxFailure
		}

		hangingExist = hangingExist || (component.TryStatus != TrySuccess)
	}
	if hangingExist {
		return TXHanging
	}
	return TXSuccess
}
