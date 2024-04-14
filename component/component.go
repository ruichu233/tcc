package component

import "context"

type TCCComponent interface {
	// 返回唯一组件ID
	ID() string
	// 执行第一段 try 操作
	Try(ctx context.Context, req *TCCReq) (*TCCResp, error)
	// 执行第二阶段的 confirm 操作
	Confirm(ctx context.Context, txID string) (*TCCResp, error)
	// 执行第二阶段的 cancel 操作
	Cancel(ctx context.Context, txID string) (*TCCResp, error)
}

type TCCReq struct {
	ComponentID string                 `json:"componentID"`
	TXID        string                 `json:"txID"`
	Data        map[string]interface{} `json:"data"`
}

type TCCResp struct {
	ComponentID string `json:"componentID"`
	TXID        string `json:"txID"`
	ACK         bool   `json:"ack"`
}
