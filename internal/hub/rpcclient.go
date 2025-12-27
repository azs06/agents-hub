package hub

import (
	"context"

	"agents-hub/internal/jsonrpc"
)

type LocalCaller struct {
	handler *jsonrpc.Handler
}

func NewLocalCaller(handler *jsonrpc.Handler) *LocalCaller {
	return &LocalCaller{handler: handler}
}

func (c *LocalCaller) Call(ctx context.Context, method string, params []byte) (jsonrpc.Response, error) {
	req := jsonrpc.Request{JSONRPC: "2.0", Method: method, Params: params, ID: "internal"}
	resp := c.handler.Handle(ctx, req)
	return resp, nil
}
