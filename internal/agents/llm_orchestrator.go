package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"a2a-go/internal/types"
	"a2a-go/internal/utils"
)

const maxRoutingTargets = 3

type LLMOrchestrator struct {
	mu          sync.RWMutex
	caller      RPCCaller
	agentIDs    []string
	routerAgent string
	card        types.AgentCard
}

type routingTarget struct {
	AgentID string `json:"agentId"`
	Agent   string `json:"agent"`
	Message string `json:"message"`
	Task    string `json:"task"`
}

type routingPlan struct {
	Targets []routingTarget `json:"targets"`
	Routes  []routingTarget `json:"routes"`
	Tasks   []routingTarget `json:"tasks"`
	AgentID string          `json:"agentId"`
	Agent   string          `json:"agent"`
	Message string          `json:"message"`
	Task    string          `json:"task"`
	Notes   string          `json:"notes"`
}

type agentDescriptor struct {
	ID          string
	Name        string
	Description string
}

func NewLLMOrchestrator(caller RPCCaller, baseURL string, agentIDs []string, routerAgent string) *LLMOrchestrator {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name:            "A2A Orchestrator (LLM)",
		Description:     "LLM-driven orchestrator that routes tasks to local agents",
		URL:             baseURL + "/agents/orchestrator",
		Version:         "1.0.0",
		Provider:        types.Provider{Name: "Local"},
		Skills:          []types.Skill{},
		Capabilities:    types.AgentCapabilities{Streaming: false, PushNotifications: false, StateTransitionHistory: false},
	}
	return &LLMOrchestrator{
		caller:      caller,
		agentIDs:    agentIDs,
		routerAgent: strings.TrimSpace(routerAgent),
		card:        card,
	}
}

func (o *LLMOrchestrator) ID() string                        { return "orchestrator" }
func (o *LLMOrchestrator) Name() string                      { return "A2A Orchestrator (LLM)" }
func (o *LLMOrchestrator) Initialize() error                 { return nil }
func (o *LLMOrchestrator) Shutdown() error                   { return nil }
func (o *LLMOrchestrator) GetCard() (types.AgentCard, error) { return o.card, nil }

func (o *LLMOrchestrator) GetCapabilities() types.RuntimeCapabilities {
	return types.RuntimeCapabilities{
		SupportsStreaming:    false,
		SupportsCancellation: false,
		MaxConcurrentTasks:   1,
		SupportedInputModes:  []string{"text/plain"},
		SupportedOutputModes: []string{"text/plain"},
	}
}

func (o *LLMOrchestrator) CheckHealth() (types.AgentHealth, error) {
	return types.AgentHealth{Status: "healthy", LastCheck: time.Now().UTC()}, nil
}

func (o *LLMOrchestrator) Execute(ctx types.ExecutionContext) (types.ExecutionResult, error) {
	prompt := extractMessageText(ctx.UserMessage)
	if prompt == "" {
		return types.ExecutionResult{}, errors.New("empty prompt")
	}
	delegates := o.Delegates()
	if len(delegates) == 0 {
		return types.ExecutionResult{}, errors.New("no delegate agents configured")
	}
	router := strings.TrimSpace(o.routerAgent)
	if router == "" {
		return types.ExecutionResult{}, errors.New("no router agent configured")
	}
	if router == o.ID() {
		return types.ExecutionResult{}, errors.New("router agent cannot be orchestrator")
	}

	descriptors := o.describeAgents(ctx, delegates)
	targets, notes, routeErr := o.routeTargets(ctx, prompt, router, descriptors)
	routingNote := ""
	if routeErr != nil {
		routingNote = fmt.Sprintf("note: routing fallback used (%v)", routeErr)
	}

	targets = normalizeTargets(targets, delegates, prompt)
	if len(targets) == 0 {
		targets = []routingTarget{{AgentID: delegates[0], Message: prompt}}
	}
	if len(targets) > maxRoutingTargets {
		targets = targets[:maxRoutingTargets]
	}

	results := make([]string, 0, len(targets)+1)
	if routingNote != "" {
		results = append(results, routingNote)
	}
	if notes != "" {
		results = append(results, "note: "+strings.TrimSpace(notes))
	}

	for _, target := range targets {
		task, err := o.sendToAgent(ctx, target.AgentID, target.Message)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: error: %v", target.AgentID, err))
			continue
		}
		results = append(results, fmt.Sprintf("%s: %s", target.AgentID, extractTaskText(task)))
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

func (o *LLMOrchestrator) Cancel(taskID string) (bool, error) {
	return false, nil
}

func (o *LLMOrchestrator) SetDelegates(ids []string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agentIDs = append([]string{}, ids...)
}

func (o *LLMOrchestrator) Delegates() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return append([]string{}, o.agentIDs...)
}

func (o *LLMOrchestrator) ExecPath() string {
	return "in-process"
}

func (o *LLMOrchestrator) routeTargets(ctx types.ExecutionContext, prompt, router string, agents []agentDescriptor) ([]routingTarget, string, error) {
	text := buildRoutingPrompt(prompt, agents)
	task, err := o.sendToAgent(ctx, router, text)
	if err != nil {
		return nil, "", err
	}
	raw := extractTaskText(task)
	targets, notes, err := parseRoutingTargets(raw)
	if err != nil {
		return nil, "", err
	}
	return targets, notes, nil
}

func (o *LLMOrchestrator) sendToAgent(ctx types.ExecutionContext, agentID, text string) (types.Task, error) {
	msg := types.Message{
		Kind:      "message",
		MessageID: utils.NewID("msg"),
		Role:      "user",
		Parts:     []types.Part{{Kind: "text", Text: text}},
		ContextID: ctx.ContextID,
		Metadata:  map[string]any{"targetAgent": agentID},
	}
	if strings.TrimSpace(ctx.WorkingDir) != "" {
		msg.Metadata["workingDirectory"] = ctx.WorkingDir
	}
	configuration := map[string]any{"historyLength": 10}
	if ctx.Timeout > 0 {
		configuration["timeout"] = int(ctx.Timeout / time.Millisecond)
	}
	params, _ := json.Marshal(map[string]any{
		"message":       msg,
		"configuration": configuration,
	})
	execCtx := context.Background()
	if ctx.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(execCtx, ctx.Timeout)
		defer cancel()
	}
	resp, err := o.caller.Call(execCtx, "message/send", params)
	if err != nil {
		return types.Task{}, err
	}
	if resp.Error != nil {
		return types.Task{}, errors.New(resp.Error.Message)
	}
	return decodeTask(resp.Result)
}

func (o *LLMOrchestrator) describeAgents(ctx types.ExecutionContext, delegates []string) []agentDescriptor {
	info, err := o.fetchAgentInfo(ctx)
	if err != nil || len(info) == 0 {
		return fallbackDescriptors(delegates)
	}
	byID := make(map[string]agentDescriptor, len(info))
	for _, entry := range info {
		desc := strings.TrimSpace(entry.Card.Description)
		if desc == "" {
			desc = strings.TrimSpace(entry.Name)
		}
		byID[entry.ID] = agentDescriptor{
			ID:          entry.ID,
			Name:        entry.Name,
			Description: desc,
		}
	}
	descriptors := make([]agentDescriptor, 0, len(delegates))
	for _, id := range delegates {
		if entry, ok := byID[id]; ok {
			descriptors = append(descriptors, entry)
			continue
		}
		descriptors = append(descriptors, agentDescriptor{ID: id, Name: id, Description: ""})
	}
	return descriptors
}

func (o *LLMOrchestrator) fetchAgentInfo(ctx types.ExecutionContext) ([]struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Card types.AgentCard `json:"card"`
}, error) {
	params, _ := json.Marshal(map[string]any{"includeHealth": false})
	execCtx := context.Background()
	if ctx.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(execCtx, ctx.Timeout)
		defer cancel()
	}
	resp, err := o.caller.Call(execCtx, "hub/agents/list", params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, errors.New(resp.Error.Message)
	}
	data, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	var entries []struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		Card types.AgentCard `json:"card"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildRoutingPrompt(prompt string, agents []agentDescriptor) string {
	var builder strings.Builder
	builder.WriteString("You are a routing agent for a local A2A hub.\n")
	builder.WriteString("Choose the best agent(s) to handle the user request.\n")
	builder.WriteString("Return JSON only with this schema:\n")
	builder.WriteString("{\"targets\":[{\"agentId\":\"<id>\",\"message\":\"<message>\"}],\"notes\":\"optional\"}\n")
	builder.WriteString("Rules:\n")
	builder.WriteString("- Use only agentId values from the list below.\n")
	builder.WriteString("- Use at most 3 targets.\n")
	builder.WriteString("- If a single agent can handle the request, return one target.\n")
	builder.WriteString("- Keep messages concise and grounded in the user request.\n\n")
	builder.WriteString("Available agents:\n")
	for _, agent := range agents {
		line := fmt.Sprintf("- %s: %s", agent.ID, agent.Name)
		if agent.Description != "" {
			line = line + " - " + agent.Description
		}
		builder.WriteString(line + "\n")
	}
	builder.WriteString("\nUser request:\n")
	builder.WriteString(prompt)
	return builder.String()
}

func parseRoutingTargets(text string) ([]routingTarget, string, error) {
	payload := extractJSON(text)
	if payload == "" {
		return nil, "", errors.New("router returned no JSON")
	}
	var plan routingPlan
	if err := json.Unmarshal([]byte(payload), &plan); err == nil {
		targets := plan.Targets
		if len(targets) == 0 {
			if len(plan.Routes) > 0 {
				targets = plan.Routes
			} else if len(plan.Tasks) > 0 {
				targets = plan.Tasks
			} else if plan.AgentID != "" || plan.Agent != "" {
				targets = []routingTarget{{
					AgentID: plan.AgentID,
					Agent:   plan.Agent,
					Message: firstNonEmpty(plan.Message, plan.Task),
				}}
			}
		}
		return targets, plan.Notes, nil
	}
	var targets []routingTarget
	if err := json.Unmarshal([]byte(payload), &targets); err == nil && len(targets) > 0 {
		return targets, "", nil
	}
	var target routingTarget
	if err := json.Unmarshal([]byte(payload), &target); err == nil && (target.AgentID != "" || target.Agent != "") {
		return []routingTarget{target}, "", nil
	}
	return nil, "", errors.New("unable to parse routing plan")
}

func extractJSON(text string) string {
	start := strings.IndexAny(text, "{[")
	if start == -1 {
		return ""
	}
	decoder := json.NewDecoder(strings.NewReader(text[start:]))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return ""
	}
	return string(raw)
}

func normalizeTargets(targets []routingTarget, delegates []string, fallbackMessage string) []routingTarget {
	if len(targets) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(delegates))
	for _, id := range delegates {
		allowed[id] = struct{}{}
	}
	normalized := make([]routingTarget, 0, len(targets))
	for _, target := range targets {
		agentID := strings.TrimSpace(firstNonEmpty(target.AgentID, target.Agent))
		if agentID == "" {
			continue
		}
		if _, ok := allowed[agentID]; !ok {
			continue
		}
		message := strings.TrimSpace(firstNonEmpty(target.Message, target.Task))
		if message == "" {
			message = fallbackMessage
		}
		normalized = append(normalized, routingTarget{AgentID: agentID, Message: message})
	}
	return normalized
}

func fallbackDescriptors(delegates []string) []agentDescriptor {
	descriptors := make([]agentDescriptor, 0, len(delegates))
	for _, id := range delegates {
		descriptors = append(descriptors, agentDescriptor{ID: id, Name: id})
	}
	return descriptors
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
