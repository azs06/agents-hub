package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"agents-hub/internal/a2a"
	"agents-hub/internal/hub"
	"agents-hub/internal/jsonrpc"
	"agents-hub/internal/utils"
)

type HTTPTransport struct {
	cfg    hub.Config
	server *hub.Server
	logger *utils.Logger
	http   *http.Server
}

func NewHTTPTransport(cfg hub.Config, server *hub.Server, logger *utils.Logger) *HTTPTransport {
	return &HTTPTransport{cfg: cfg, server: server, logger: logger}
}

func (t *HTTPTransport) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Original JSON-RPC endpoint (for backwards compatibility)
	mux.HandleFunc("/", t.handleRPC)
	mux.HandleFunc("/health", t.handleHealth)
	mux.HandleFunc("/.well-known/agent.json", t.handleHubCard)
	mux.HandleFunc("/.well-known/agents", t.handleAgents)
	mux.HandleFunc("/.well-known/agents/", t.handleAgent)
	mux.HandleFunc("/stream", t.handleStream)

	// Register A2A protocol routes
	baseURL := fmt.Sprintf("http://%s:%d", t.cfg.HTTP.Host, t.cfg.HTTP.Port)
	a2aServer, err := a2a.NewA2AServer(t.server, baseURL)
	if err != nil {
		t.logger.Warnf("failed to create A2A server: %v", err)
	} else {
		a2aServer.RegisterRoutes(mux)
		t.logger.Debugf("A2A protocol enabled at /a2a")
	}

	addr := fmt.Sprintf("%s:%d", t.cfg.HTTP.Host, t.cfg.HTTP.Port)
	t.http = &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		ctxShutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = t.http.Shutdown(ctxShutdown)
	}()

	return t.http.ListenAndServe()
}

func (t *HTTPTransport) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req jsonrpc.Request
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, jsonrpc.Response{JSONRPC: "2.0", Error: &jsonrpc.RPCError{Code: jsonrpc.ErrParseError, Message: "Parse error"}})
		return
	}
	resp := t.server.Handler().Handle(r.Context(), req)
	writeJSON(w, resp)
}

func (t *HTTPTransport) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req jsonrpc.Request
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	resp := t.server.Handler().Handle(r.Context(), req)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	data, _ := json.Marshal(resp)
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n\n"))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (t *HTTPTransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (t *HTTPTransport) handleHubCard(w http.ResponseWriter, r *http.Request) {
	baseURL := fmt.Sprintf("http://%s:%d", t.cfg.HTTP.Host, t.cfg.HTTP.Port)
	writeJSON(w, t.server.HubCard(baseURL))
}

func (t *HTTPTransport) handleAgents(w http.ResponseWriter, r *http.Request) {
	baseURL := fmt.Sprintf("http://%s:%d", t.cfg.HTTP.Host, t.cfg.HTTP.Port)
	agents := t.server.AgentsList()
	cards := make([]any, 0, len(agents))
	for _, info := range agents {
		card := info.Card
		card.URL = baseURL + "/.well-known/agents/" + info.Agent.ID() + ".json"
		cards = append(cards, card)
	}
	writeJSON(w, cards)
}

func (t *HTTPTransport) handleAgent(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/.well-known/agents/")
	id := strings.TrimSuffix(path, ".json")
	info, ok := t.server.AgentByID(id)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	card := info.Card
	baseURL := fmt.Sprintf("http://%s:%d", t.cfg.HTTP.Host, t.cfg.HTTP.Port)
	card.URL = baseURL + "/.well-known/agents/" + info.Agent.ID() + ".json"
	writeJSON(w, card)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}
