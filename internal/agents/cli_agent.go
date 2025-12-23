package agents

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"a2a-go/internal/types"
)

type CLIConfig struct {
	AgentID    string
	Name       string
	Exec       string
	Args       []string
	HealthArgs []string
	Card       types.AgentCard
}

type CLIAgent struct {
	config CLIConfig
}

func NewCLIAgent(cfg CLIConfig) *CLIAgent {
	return &CLIAgent{config: cfg}
}

func (a *CLIAgent) ID() string   { return a.config.AgentID }
func (a *CLIAgent) Name() string { return a.config.Name }

func (a *CLIAgent) Initialize() error { return nil }
func (a *CLIAgent) Shutdown() error   { return nil }

func (a *CLIAgent) GetCard() (types.AgentCard, error) { return a.config.Card, nil }

func (a *CLIAgent) GetCapabilities() types.RuntimeCapabilities {
	return types.RuntimeCapabilities{
		SupportsStreaming:    false,
		SupportsCancellation: false,
		MaxConcurrentTasks:   1,
		SupportedInputModes:  []string{"text/plain"},
		SupportedOutputModes: []string{"text/plain"},
	}
}

func (a *CLIAgent) CheckHealth() (types.AgentHealth, error) {
	start := time.Now()
	cmd := exec.Command(a.config.Exec, a.config.HealthArgs...)
	if err := cmd.Run(); err != nil {
		return types.AgentHealth{Status: "unhealthy", LastCheck: time.Now().UTC()}, err
	}
	return types.AgentHealth{Status: "healthy", LastCheck: time.Now().UTC(), LatencyMs: time.Since(start).Milliseconds()}, nil
}

func (a *CLIAgent) Execute(ctx types.ExecutionContext) (types.ExecutionResult, error) {
	prompt := extractPrompt(ctx.UserMessage)
	if prompt == "" {
		return types.ExecutionResult{}, errors.New("empty prompt")
	}

	args := make([]string, 0, len(a.config.Args)+1)
	for _, arg := range a.config.Args {
		if arg == "{prompt}" {
			args = append(args, prompt)
			continue
		}
		args = append(args, arg)
	}
	execCtx := context.Background()
	if ctx.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(execCtx, ctx.Timeout)
		defer cancel()
	}
	command := exec.CommandContext(execCtx, a.config.Exec, args...)
	stdin, _ := command.StdinPipe()
	stdin.Close()

	var out bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &out
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if stderr.Len() > 0 {
			return types.ExecutionResult{}, errors.New(strings.TrimSpace(stderr.String()))
		}
		return types.ExecutionResult{}, err
	}
	text := strings.TrimSpace(out.String())

	response := types.Message{
		Kind:      "message",
		MessageID: "resp-" + ctx.TaskID,
		Role:      "agent",
		Parts:     []types.Part{{Kind: "text", Text: text}},
		TaskID:    ctx.TaskID,
		ContextID: ctx.ContextID,
	}

	task := types.Task{
		Kind:      "task",
		ID:        ctx.TaskID,
		ContextID: ctx.ContextID,
		Status: types.TaskStatus{
			State:     types.TaskStateCompleted,
			Message:   &response,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		},
		History: append([]types.Message{}, ctx.PreviousHistory...),
	}
	return types.ExecutionResult{Task: task, Artifacts: nil, FinalState: types.TaskStateCompleted}, nil
}

func (a *CLIAgent) Cancel(taskID string) (bool, error) {
	return false, nil
}

func (a *CLIAgent) ExecPath() string {
	return a.config.Exec
}

func extractPrompt(msg types.Message) string {
	parts := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		if part.Kind == "text" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}
