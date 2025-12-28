package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"agents-hub/internal/jsonrpc"
	"agents-hub/internal/types"
	"agents-hub/internal/utils"
)

type RPCCaller interface {
	Call(ctx context.Context, method string, params []byte) (jsonrpc.Response, error)
}

type Orchestrator struct {
	mu       sync.RWMutex
	caller   RPCCaller
	agentIDs []string
	card     types.AgentCard
}

func NewOrchestrator(caller RPCCaller, baseURL string, agentIDs []string) *Orchestrator {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name:            "A2A Orchestrator",
		Description:     "Delegates tasks to other local agents",
		URL:             baseURL + "/agents/orchestrator",
		Version:         "1.0.0",
		Provider:        types.Provider{Name: "Local"},
		Skills:          []types.Skill{},
		Capabilities:    types.AgentCapabilities{Streaming: false, PushNotifications: false, StateTransitionHistory: false},
	}
	return &Orchestrator{caller: caller, agentIDs: agentIDs, card: card}
}

func (o *Orchestrator) ID() string                        { return "orchestrator" }
func (o *Orchestrator) Name() string                      { return "A2A Orchestrator" }
func (o *Orchestrator) Initialize() error                 { return nil }
func (o *Orchestrator) Shutdown() error                   { return nil }
func (o *Orchestrator) GetCard() (types.AgentCard, error) { return o.card, nil }

func (o *Orchestrator) GetCapabilities() types.RuntimeCapabilities {
	return types.RuntimeCapabilities{
		SupportsStreaming:    false,
		SupportsCancellation: false,
		MaxConcurrentTasks:   1,
		SupportedInputModes:  []string{"text/plain"},
		SupportedOutputModes: []string{"text/plain"},
	}
}

func (o *Orchestrator) CheckHealth() (types.AgentHealth, error) {
	return types.AgentHealth{Status: "healthy", LastCheck: time.Now().UTC()}, nil
}

// DefaultOrchestratorTimeout is used when no timeout is specified (10 minutes)
const DefaultOrchestratorTimeout = 10 * time.Minute

func (o *Orchestrator) Execute(ctx types.ExecutionContext) (types.ExecutionResult, error) {
	prompt := extractMessageText(ctx.UserMessage)
	if prompt == "" {
		return types.ExecutionResult{}, errors.New("empty prompt")
	}
	if len(o.Delegates()) == 0 {
		return types.ExecutionResult{}, errors.New("no delegate agents configured")
	}
	parts := splitPrompt(prompt)
	if len(parts) == 0 {
		parts = []string{prompt}
	}

	// Use default timeout if none specified
	timeout := ctx.Timeout
	if timeout <= 0 {
		timeout = DefaultOrchestratorTimeout
	}
	// Create a context with timeout for all delegate calls
	callCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	results := make([]string, 0, len(parts))
	for i, part := range parts {
		delegates := o.Delegates()
		agentID := delegates[i%len(delegates)]
		metadata := map[string]any{"targetAgent": agentID}
		if strings.TrimSpace(ctx.WorkingDir) != "" {
			metadata["workingDirectory"] = ctx.WorkingDir
		}
		msg := types.Message{
			Kind:      "message",
			MessageID: utils.NewID("msg"),
			Role:      "user",
			Parts:     []types.Part{{Kind: "text", Text: strings.TrimSpace(part)}},
			ContextID: ctx.ContextID,
			Metadata:  metadata,
		}
		params, _ := json.Marshal(map[string]any{
			"message": msg,
			"configuration": map[string]any{
				"historyLength": 10,
				"timeout":       int(timeout / time.Millisecond),
			},
		})
		resp, err := o.caller.Call(callCtx, "message/send", params)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: error: %v", agentID, err))
			continue
		}
		if resp.Error != nil {
			results = append(results, fmt.Sprintf("%s: error: %s", agentID, resp.Error.Message))
			continue
		}
		task, err := decodeTask(resp.Result)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: error: %v", agentID, err))
			continue
		}
		results = append(results, fmt.Sprintf("%s: %s", agentID, extractTaskText(task)))
	}

	response := types.Message{
		Kind:      "message",
		MessageID: "resp-" + ctx.TaskID,
		Role:      "agent",
		Parts:     []types.Part{{Kind: "text", Text: strings.Join(results, "\n\n")}},
		TaskID:    ctx.TaskID,
		ContextID: ctx.ContextID,
	}
	return types.ExecutionResult{
		Task: types.Task{
			Kind:      "task",
			ID:        ctx.TaskID,
			ContextID: ctx.ContextID,
			Status:    types.TaskStatus{State: types.TaskStateCompleted, Message: &response, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)},
			History:   append([]types.Message{}, ctx.PreviousHistory...),
		},
		FinalState: types.TaskStateCompleted,
	}, nil
}

func (o *Orchestrator) Cancel(taskID string) (bool, error) {
	return false, nil
}

func (o *Orchestrator) SetDelegates(ids []string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agentIDs = append([]string{}, ids...)
}

func (o *Orchestrator) Delegates() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return append([]string{}, o.agentIDs...)
}

func (o *Orchestrator) ExecPath() string {
	return "in-process"
}

func extractMessageText(msg types.Message) string {
	parts := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		if part.Kind == "text" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func splitPrompt(prompt string) []string {
	if strings.Contains(prompt, "\n") {
		lines := strings.Split(prompt, "\n")
		return compactStrings(lines)
	}
	if strings.Contains(prompt, ";") {
		parts := strings.Split(prompt, ";")
		return compactStrings(parts)
	}
	if strings.Contains(prompt, " and ") {
		parts := strings.Split(prompt, " and ")
		return compactStrings(parts)
	}
	return []string{prompt}
}

func compactStrings(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func decodeTask(result any) (types.Task, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return types.Task{}, err
	}
	var task types.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return types.Task{}, err
	}
	return task, nil
}

func extractTaskText(task types.Task) string {
	if task.Status.Message == nil {
		return string(task.Status.State)
	}
	return extractMessageText(*task.Status.Message)
}
