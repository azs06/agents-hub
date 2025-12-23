package jsonrpc

import (
	"context"
	"encoding/json"
)

type HandlerFunc func(ctx context.Context, params json.RawMessage) (any, *RPCError)

type Handler struct {
	methods map[string]HandlerFunc
}

func NewHandler() *Handler {
	return &Handler{methods: make(map[string]HandlerFunc)}
}

func (h *Handler) Register(method string, fn HandlerFunc) {
	h.methods[method] = fn
}

func (h *Handler) Handle(ctx context.Context, req Request) Response {
	if req.JSONRPC != "2.0" || req.Method == "" {
		return Response{JSONRPC: "2.0", Error: &RPCError{Code: ErrInvalidRequest, Message: "Invalid Request"}, ID: req.ID}
	}
	fn, ok := h.methods[req.Method]
	if !ok {
		return Response{JSONRPC: "2.0", Error: &RPCError{Code: ErrMethodNotFound, Message: "Method not found"}, ID: req.ID}
	}
	params := json.RawMessage(req.Params)
	result, err := fn(ctx, params)
	if err != nil {
		return Response{JSONRPC: "2.0", Error: err, ID: req.ID}
	}
	return Response{JSONRPC: "2.0", Result: result, ID: req.ID}
}

const (
	ErrParseError      = -32700
	ErrInvalidRequest  = -32600
	ErrMethodNotFound  = -32601
	ErrInvalidParams   = -32602
	ErrInternalError   = -32603
	ErrTaskNotFound    = -32001
	ErrTaskNotCancelable = -32002
	ErrAgentNotFound   = -32003
	ErrAgentUnavailable = -32004
	ErrUnsupported     = -32005
	ErrAuthError       = -32006
	ErrTimeout         = -32007
	ErrContextNotFound = -32008
)
