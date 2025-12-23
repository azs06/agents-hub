package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"a2a-go/internal/hub"
	"a2a-go/internal/jsonrpc"
	"a2a-go/internal/transport"
	"a2a-go/internal/types"
	"a2a-go/internal/utils"
)

func Run() int {
	if len(os.Args) < 2 {
		return runTUI(os.Args[1:])
	}

	cmd := os.Args[1]
	if strings.HasPrefix(cmd, "-") {
		return runTUI(os.Args[1:])
	}
	switch cmd {
	case "start":
		return runStart(os.Args[2:])
	case "stop":
		return runStop(os.Args[2:])
	case "status":
		return runStatus(os.Args[2:])
	case "agents":
		return runAgents(os.Args[2:])
	case "send":
		return runSend(os.Args[2:])
	case "tasks":
		return runTasks(os.Args[2:])
	case "tui":
		return runTUI(os.Args[2:])
	default:
		usage()
		return 1
	}
}

func usage() {
	fmt.Println("a2a-hub <command> [options]")
	fmt.Println("Commands: start, stop, status, agents, send, tasks, tui")
}

func runStart(args []string) int {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	foreground := fs.Bool("foreground", false, "run in foreground")
	httpPort := fs.Int("http-port", 8080, "http port")
	noHTTP := fs.Bool("no-http", false, "disable http")
	socketPath := fs.String("socket", "/tmp/a2a-hub.sock", "unix socket path")
	verbose := fs.Bool("verbose", false, "debug logging")
	orchestratorAgents := fs.String("orchestrator-agents", "", "comma-separated agent IDs for orchestrator")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	_ = foreground

	cfg := hub.DefaultConfig()
	cfg.Socket.Path = *socketPath
	cfg.HTTP.Port = *httpPort
	cfg.HTTP.Enabled = !*noHTTP
	cfg.Orchestrator.Agents = resolveOrchestratorAgents(*orchestratorAgents)
	if *verbose {
		cfg.Logging.Level = "debug"
	}

	logger := utils.NewLogger(cfg.Logging.Level)
	server := hub.NewServer(cfg, logger)
	server.RegisterHandlers()
	baseURL := fmt.Sprintf("http://%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)
	_ = server.InitAgents(baseURL)
	if err := server.LoadState(); err != nil {
		logger.Warnf("failed to load state: %v", err)
	}
	if err := server.WritePid(); err != nil {
		logger.Warnf("failed to write pid: %v", err)
	}

	ctx, cancel := contextWithSignals()
	defer cancel()
	server.Registry().StartHealthChecks(30 * time.Second)

	if cfg.Socket.Enabled {
		unixTransport := transport.NewUnixTransport(cfg, server, logger)
		go func() {
			if err := unixTransport.Start(ctx); err != nil {
				logger.Errorf("unix transport error: %v", err)
			}
		}()
	}
	if cfg.HTTP.Enabled {
		httpTransport := transport.NewHTTPTransport(cfg, server, logger)
		go func() {
			if err := httpTransport.Start(ctx); err != nil {
				logger.Errorf("http transport error: %v", err)
			}
		}()
	}

	<-ctx.Done()
	server.Registry().Stop()
	server.RemovePid()
	return 0
}

func runStop(args []string) int {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	socketPath := fs.String("socket", "/tmp/a2a-hub.sock", "unix socket path")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	_ = socketPath
	pidFile := filepath.Join(os.Getenv("HOME"), ".a2a-hub", "hub.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Println("hub not running")
		return 1
	}
	pid := strings.TrimSpace(string(data))
	p, err := os.FindProcess(parsePID(pid))
	if err != nil {
		fmt.Println("failed to find process")
		return 1
	}
	_ = p.Signal(syscall.SIGTERM)
	fmt.Println("stop signal sent")
	return 0
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	format := fs.String("format", "pretty", "output format: json|pretty")
	socketPath := fs.String("socket", "/tmp/a2a-hub.sock", "unix socket path")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	resp, err := sendRPCUnix(*socketPath, jsonrpc.Request{JSONRPC: "2.0", Method: "hub/status", Params: nil, ID: "1"})
	if err != nil {
		fmt.Println("hub not responding")
		return 1
	}
	printResponse(resp, *format)
	return 0
}

func runAgents(args []string) int {
	fs := flag.NewFlagSet("agents", flag.ContinueOnError)
	format := fs.String("format", "pretty", "output format: json|pretty")
	socketPath := fs.String("socket", "/tmp/a2a-hub.sock", "unix socket path")
	withHealth := fs.Bool("health", false, "include health")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	params, _ := json.Marshal(map[string]any{"includeHealth": *withHealth})
	resp, err := sendRPCUnix(*socketPath, jsonrpc.Request{JSONRPC: "2.0", Method: "hub/agents/list", Params: params, ID: "1"})
	if err != nil {
		fmt.Println("hub not responding")
		return 1
	}
	printResponse(resp, *format)
	return 0
}

func runSend(args []string) int {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	format := fs.String("format", "pretty", "output format: json|pretty")
	socketPath := fs.String("socket", "/tmp/a2a-hub.sock", "unix socket path")
	contextID := fs.String("context", "", "context id")
	timeoutMs := fs.Int("timeout", 0, "timeout ms")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 2 {
		fmt.Println("usage: a2a-hub send <agent-id> \"message\"")
		return 1
	}
	agentID := fs.Arg(0)
	messageText := fs.Arg(1)

	msg := types.Message{
		Kind: "message",
		MessageID: "msg-" + fmt.Sprint(time.Now().UnixNano()),
		Role: "user",
		Parts: []types.Part{{Kind: "text", Text: messageText}},
		ContextID: *contextID,
		Metadata: map[string]any{"targetAgent": agentID},
	}
	params, _ := json.Marshal(map[string]any{
		"message": msg,
		"configuration": map[string]any{"historyLength": 10, "timeout": *timeoutMs},
	})
	resp, err := sendRPCUnix(*socketPath, jsonrpc.Request{JSONRPC: "2.0", Method: "message/send", Params: params, ID: "1"})
	if err != nil {
		fmt.Println("hub not responding")
		return 1
	}
	printResponse(resp, *format)
	return 0
}

func runTasks(args []string) int {
	fs := flag.NewFlagSet("tasks", flag.ContinueOnError)
	format := fs.String("format", "pretty", "output format: json|pretty")
	socketPath := fs.String("socket", "/tmp/a2a-hub.sock", "unix socket path")
	contextID := fs.String("context", "", "context id")
	state := fs.String("state", "", "task state")
	limit := fs.Int("limit", 20, "limit")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	params, _ := json.Marshal(map[string]any{"contextId": *contextID, "state": *state, "limit": *limit, "offset": 0})
	resp, err := sendRPCUnix(*socketPath, jsonrpc.Request{JSONRPC: "2.0", Method: "hub/tasks/list", Params: params, ID: "1"})
	if err != nil {
		fmt.Println("hub not responding")
		return 1
	}
	printResponse(resp, *format)
	return 0
}

func contextWithSignals() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()
	return ctx, cancel
}

func sendRPCUnix(socketPath string, req jsonrpc.Request) (jsonrpc.Response, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return jsonrpc.Response{}, err
	}
	defer conn.Close()
	data, _ := json.Marshal(req)
	_, err = conn.Write(append(data, '\n'))
	if err != nil {
		return jsonrpc.Response{}, err
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return jsonrpc.Response{}, err
	}
	var resp jsonrpc.Response
	if err := json.Unmarshal(bytes.TrimSpace(line), &resp); err != nil {
		return jsonrpc.Response{}, err
	}
	return resp, nil
}

func printResponse(resp jsonrpc.Response, format string) {
	if format == "json" {
		data, _ := json.Marshal(resp)
		fmt.Println(string(data))
		return
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Println(string(data))
}

func parsePID(val string) int {
	pid := 0
	_, _ = fmt.Sscanf(val, "%d", &pid)
	return pid
}

func resolveOrchestratorAgents(flagValue string) []string {
	if flagValue == "" {
		flagValue = os.Getenv("ORCHESTRATOR_AGENTS")
	}
	if flagValue == "" {
		return hub.DefaultConfig().Orchestrator.Agents
	}
	if strings.EqualFold(flagValue, "none") {
		return nil
	}
	items := strings.Split(flagValue, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		val := strings.TrimSpace(item)
		if val == "" {
			continue
		}
		out = append(out, val)
	}
	return out
}
