package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"a2a-go/internal/agents"
	"a2a-go/internal/jsonrpc"
	"a2a-go/internal/types"
	"a2a-go/internal/utils"
)

type Server struct {
	cfg       Config
	logger    *utils.Logger
	registry  *AgentRegistry
	tasks     *TaskManager
	contexts  *ContextManager
	handler   *jsonrpc.Handler
	startTime time.Time
	settings  Settings
}

func NewServer(cfg Config, logger *utils.Logger) *Server {
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join(os.Getenv("HOME"), ".a2a-hub")
	}
	server := &Server{
		cfg:       cfg,
		logger:    logger,
		registry:  NewAgentRegistry(logger),
		tasks:     NewTaskManager(),
		contexts:  NewContextManager(),
		handler:   jsonrpc.NewHandler(),
		startTime: time.Now().UTC(),
		settings:  Settings{OrchestratorAgents: append([]string{}, cfg.Orchestrator.Agents...)},
	}
	server.tasks.SetPersistence(filepath.Join(cfg.DataDir, "tasks.json"))
	server.contexts.SetPersistence(filepath.Join(cfg.DataDir, "contexts.json"))
	return server
}

func (s *Server) InitAgents(baseURL string) error {
	caller := NewLocalCaller(s.handler)
	agentsList := []agents.Agent{
		agents.NewClaudeAgent(baseURL),
		agents.NewGeminiAgent(baseURL),
		agents.NewCodexAgent(baseURL),
		agents.NewVibeAgent(baseURL),
	}
	if len(s.cfg.Orchestrator.Agents) > 0 {
		orchestratorAgent := agents.Agent(agents.NewOrchestrator(caller, baseURL, s.cfg.Orchestrator.Agents))
		if strings.TrimSpace(s.cfg.Orchestrator.RouterAgent) != "" {
			orchestratorAgent = agents.NewLLMOrchestrator(caller, baseURL, s.cfg.Orchestrator.Agents, s.cfg.Orchestrator.RouterAgent)
		}
		agentsList = append([]agents.Agent{orchestratorAgent}, agentsList...)
	}
	for _, agent := range agentsList {
		if err := s.registry.Register(agent); err != nil {
			s.logger.Warnf("failed to register %s: %v", agent.ID(), err)
		}
	}
	s.applySettingsToAgents()
	return nil
}

func (s *Server) RegisterHandlers() {
	s.handler.Register("hub/status", s.handleHubStatus)
	s.handler.Register("hub/agents/list", s.handleAgentsList)
	s.handler.Register("hub/agents/get", s.handleAgentsGet)
	s.handler.Register("hub/agents/health", s.handleAgentsHealth)
	s.handler.Register("hub/tasks/list", s.handleTasksList)
	s.handler.Register("hub/contexts/list", s.handleContextsList)
	s.handler.Register("message/send", s.handleMessageSend)
	s.handler.Register("tasks/get", s.handleTaskGet)
	s.handler.Register("tasks/cancel", s.handleTaskCancel)
}

func (s *Server) Handler() *jsonrpc.Handler {
	return s.handler
}

func (s *Server) AgentsList() []AgentInfo {
	return s.registry.List()
}

func (s *Server) AgentByID(id string) (*AgentInfo, bool) {
	return s.registry.Get(id)
}

func (s *Server) Registry() *AgentRegistry {
	return s.registry
}

func (s *Server) LoadState() error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}
	if err := s.LoadSettings(); err != nil {
		return err
	}
	s.applySettingsToAgents()
	if err := s.contexts.Load(); err != nil {
		return err
	}
	if err := s.tasks.Load(); err != nil {
		return err
	}
	return nil
}

func (s *Server) applySettingsToAgents() {
	if info, ok := s.registry.Get("claude-code"); ok {
		if setter, ok := info.Agent.(interface{ SetDefaultConfig(types.ClaudeConfig) }); ok {
			setter.SetDefaultConfig(s.GetClaudeConfig())
		}
	}
	if info, ok := s.registry.Get("codex"); ok {
		if setter, ok := info.Agent.(interface{ SetDefaultConfig(types.CodexConfig) }); ok {
			setter.SetDefaultConfig(s.GetCodexConfig())
		}
	}
}

func extractWorkingDir(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if dir, ok := metadata["workingDirectory"].(string); ok && strings.TrimSpace(dir) != "" {
		return dir
	}
	if dir, ok := metadata["workingDir"].(string); ok && strings.TrimSpace(dir) != "" {
		return dir
	}
	if dir, ok := metadata["cwd"].(string); ok && strings.TrimSpace(dir) != "" {
		return dir
	}
	return ""
}

func (s *Server) Config() Config {
	return s.cfg
}

func (s *Server) OrchestratorAgents() []string {
	info, ok := s.registry.Get("orchestrator")
	if ok {
		if getter, ok := info.Agent.(interface{ Delegates() []string }); ok {
			return getter.Delegates()
		}
	}
	return append([]string{}, s.cfg.Orchestrator.Agents...)
}

func (s *Server) UpdateOrchestratorAgents(ids []string) bool {
	s.cfg.Orchestrator.Agents = append([]string{}, ids...)
	s.updateSettingsAgents(ids)
	if err := s.SaveSettings(); err != nil {
		s.logger.Warnf("failed to save settings: %v", err)
	}
	info, ok := s.registry.Get("orchestrator")
	if !ok {
		return false
	}
	if setter, ok := info.Agent.(interface{ SetDelegates([]string) }); ok {
		setter.SetDelegates(ids)
		return true
	}
	return false
}

func (s *Server) handleHubStatus(ctx context.Context, params json.RawMessage) (any, *jsonrpc.RPCError) {
	agentsInfo := s.registry.List()
	resultAgents := make([]map[string]any, 0, len(agentsInfo))
	healthy := 0
	degraded := 0
	unhealthy := 0
	unknown := 0
	for _, info := range agentsInfo {
		status := info.Health.Status
		switch status {
		case "healthy":
			healthy++
		case "degraded":
			degraded++
		case "unhealthy":
			unhealthy++
		default:
			unknown++
		}
		resultAgents = append(resultAgents, map[string]any{
			"id":     info.Agent.ID(),
			"name":   info.Agent.Name(),
			"status": status,
		})
	}
	return map[string]any{
		"version":     "1.0.0",
		"uptime":      int(time.Since(s.startTime).Seconds()),
		"agents":      resultAgents,
		"activeTasks": 0,
		"totalTasks":  len(s.tasks.List("", "", 0, 0)),
		"total":       len(agentsInfo),
		"healthy":     healthy,
		"degraded":    degraded,
		"unhealthy":   unhealthy,
		"unknown":     unknown,
	}, nil
}

func (s *Server) handleAgentsList(ctx context.Context, params json.RawMessage) (any, *jsonrpc.RPCError) {
	var req struct {
		IncludeHealth bool `json:"includeHealth"`
	}
	_ = json.Unmarshal(params, &req)
	infos := s.registry.List()
	result := make([]map[string]any, 0, len(infos))
	for _, info := range infos {
		entry := map[string]any{
			"id":           info.Agent.ID(),
			"name":         info.Agent.Name(),
			"card":         info.Card,
			"registeredAt": info.RegisteredAt.Format(time.RFC3339Nano),
		}
		if req.IncludeHealth {
			entry["health"] = info.Health
		}
		result = append(result, entry)
	}
	return result, nil
}

func (s *Server) handleAgentsGet(ctx context.Context, params json.RawMessage) (any, *jsonrpc.RPCError) {
	var req struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &req); err != nil || req.AgentID == "" {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "agentId required"}
	}
	info, ok := s.registry.Get(req.AgentID)
	if !ok {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrAgentNotFound, Message: "agent not found"}
	}
	return map[string]any{
		"id":           info.Agent.ID(),
		"name":         info.Agent.Name(),
		"card":         info.Card,
		"health":       info.Health,
		"registeredAt": info.RegisteredAt.Format(time.RFC3339Nano),
	}, nil
}

func (s *Server) handleAgentsHealth(ctx context.Context, params json.RawMessage) (any, *jsonrpc.RPCError) {
	var req struct {
		AgentID string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &req); err != nil || req.AgentID == "" {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "agentId required"}
	}
	info, ok := s.registry.Get(req.AgentID)
	if !ok {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrAgentNotFound, Message: "agent not found"}
	}
	return info.Health, nil
}

func (s *Server) handleTasksList(ctx context.Context, params json.RawMessage) (any, *jsonrpc.RPCError) {
	var req struct {
		ContextID string          `json:"contextId"`
		State     types.TaskState `json:"state"`
		Limit     int             `json:"limit"`
		Offset    int             `json:"offset"`
	}
	_ = json.Unmarshal(params, &req)
	return s.tasks.List(req.ContextID, req.State, req.Limit, req.Offset), nil
}

func (s *Server) handleContextsList(ctx context.Context, params json.RawMessage) (any, *jsonrpc.RPCError) {
	var req struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(params, &req)
	contexts := s.contexts.List(req.Limit)
	result := make([]map[string]any, 0, len(contexts))
	for _, ctx := range contexts {
		result = append(result, map[string]any{
			"id":        ctx.ID,
			"createdAt": ctx.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	return result, nil
}

func (s *Server) handleMessageSend(ctx context.Context, params json.RawMessage) (any, *jsonrpc.RPCError) {
	var req struct {
		Message       types.Message `json:"message"`
		Configuration struct {
			HistoryLength int    `json:"historyLength"`
			TimeoutMs     int    `json:"timeout"`
			WorkingDir    string `json:"workingDirectory"`
		} `json:"configuration"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "invalid params"}
	}
	if req.Message.Kind != "message" {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "message required"}
	}
	if req.Message.Metadata == nil {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "metadata.targetAgent required"}
	}
	agentID, ok := req.Message.Metadata["targetAgent"].(string)
	if !ok || agentID == "" {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "metadata.targetAgent required"}
	}
	info, ok := s.registry.Get(agentID)
	if !ok {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrAgentNotFound, Message: "agent not found"}
	}

	contextID := req.Message.ContextID
	if contextID == "" {
		contextID = utils.NewID("ctx")
		s.contexts.Create(contextID)
	}
	if _, exists := s.contexts.Get(contextID); !exists {
		s.contexts.Create(contextID)
	}

	taskID := utils.NewID("task")
	req.Message.TaskID = taskID
	req.Message.ContextID = contextID
	status := types.TaskStatus{State: types.TaskStateSubmitted, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}
	task := &types.Task{Kind: "task", ID: taskID, ContextID: contextID, Status: status}
	s.tasks.Create(task)
	_ = s.tasks.UpdateStatus(taskID, types.TaskStateWorking, nil)

	workingDir := strings.TrimSpace(req.Configuration.WorkingDir)
	if workingDir == "" {
		workingDir = extractWorkingDir(req.Message.Metadata)
	}

	result, err := info.Agent.Execute(types.ExecutionContext{
		TaskID:      taskID,
		ContextID:   contextID,
		UserMessage: req.Message,
		Timeout:     time.Duration(req.Configuration.TimeoutMs) * time.Millisecond,
		WorkingDir:  workingDir,
	})
	if err != nil {
		_ = s.tasks.UpdateStatus(taskID, types.TaskStateFailed, &types.Message{Kind: "message", MessageID: "error-" + taskID, Role: "agent", Parts: []types.Part{{Kind: "text", Text: err.Error()}}, TaskID: taskID, ContextID: contextID})
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrInternalError, Message: err.Error()}
	}
	if result.Task.Status.Message != nil {
		result.Task.Status.Message.ContextID = contextID
		result.Task.Status.Message.TaskID = taskID
	}
	task.Status = result.Task.Status
	task.History = append([]types.Message{req.Message}, result.Task.History...)
	task.Artifacts = result.Task.Artifacts
	task.ContextID = contextID
	_ = s.tasks.UpdateStatus(taskID, task.Status.State, task.Status.Message)

	return task, nil
}

func (s *Server) handleTaskGet(ctx context.Context, params json.RawMessage) (any, *jsonrpc.RPCError) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "id required"}
	}
	task, ok := s.tasks.Get(req.ID)
	if !ok {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrTaskNotFound, Message: "task not found"}
	}
	return task, nil
}

func (s *Server) handleTaskCancel(ctx context.Context, params json.RawMessage) (any, *jsonrpc.RPCError) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &req); err != nil || req.ID == "" {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "id required"}
	}
	task, ok := s.tasks.Get(req.ID)
	if !ok {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrTaskNotFound, Message: "task not found"}
	}
	if task.Status.State == types.TaskStateCompleted || task.Status.State == types.TaskStateFailed || task.Status.State == types.TaskStateCanceled {
		return nil, &jsonrpc.RPCError{Code: jsonrpc.ErrTaskNotCancelable, Message: "task not cancelable"}
	}
	task.Status.State = types.TaskStateCanceled
	task.Status.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	return map[string]any{"canceled": true}, nil
}

func (s *Server) HubCard(baseURL string) types.AgentCard {
	return types.AgentCard{
		ProtocolVersion: "1.0",
		Name:            "A2A Local Hub",
		Description:     "Local multi-agent hub",
		URL:             baseURL,
		Version:         "1.0.0",
		Provider:        types.Provider{Name: "Local"},
		Skills:          []types.Skill{},
		Capabilities:    types.AgentCapabilities{Streaming: true, PushNotifications: false, StateTransitionHistory: false},
	}
}

func (s *Server) EnsureDataDir() error {
	if s.cfg.DataDir == "" {
		return errors.New("data dir required")
	}
	return os.MkdirAll(s.cfg.DataDir, 0o755)
}

func (s *Server) PidFile() string {
	return filepath.Join(s.cfg.DataDir, "hub.pid")
}

func (s *Server) WritePid() error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}
	return os.WriteFile(s.PidFile(), []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
}

func (s *Server) RemovePid() {
	_ = os.Remove(s.PidFile())
}
