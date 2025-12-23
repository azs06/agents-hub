package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"

	"a2a-go/internal/hub"
	"a2a-go/internal/jsonrpc"
	"a2a-go/internal/utils"
)

type UnixTransport struct {
	cfg    hub.Config
	server *hub.Server
	logger *utils.Logger
	ln     net.Listener
}

func NewUnixTransport(cfg hub.Config, server *hub.Server, logger *utils.Logger) *UnixTransport {
	return &UnixTransport{cfg: cfg, server: server, logger: logger}
}

func (t *UnixTransport) Start(ctx context.Context) error {
	_ = os.Remove(t.cfg.Socket.Path)
	ln, err := net.Listen("unix", t.cfg.Socket.Path)
	if err != nil {
		return err
	}
	t.ln = ln
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go t.handleConn(conn)
	}
}

func (t *UnixTransport) handleConn(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		var req jsonrpc.Request
		if err := json.Unmarshal(line, &req); err != nil {
			resp := jsonrpc.Response{JSONRPC: "2.0", Error: &jsonrpc.RPCError{Code: jsonrpc.ErrParseError, Message: "Parse error"}}
			data, _ := json.Marshal(resp)
			_, _ = conn.Write(append(data, '\n'))
			continue
		}
		resp := t.server.Handler().Handle(context.Background(), req)
		data, _ := json.Marshal(resp)
		_, _ = conn.Write(append(data, '\n'))
	}
}
