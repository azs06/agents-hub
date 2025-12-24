package agents

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"a2a-go/internal/types"

	"github.com/creack/pty"
)

type CLIConfig struct {
	AgentID        string
	Name           string
	Exec           string
	Args           []string
	HealthArgs     []string
	Card           types.AgentCard
	PromptPatterns []string
}

type CLIAgent struct {
	config         CLIConfig
	promptPatterns []*regexp.Regexp
}

func NewCLIAgent(cfg CLIConfig) *CLIAgent {
	compiled := make([]*regexp.Regexp, 0, len(cfg.PromptPatterns))
	for _, pattern := range cfg.PromptPatterns {
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		compiled = append(compiled, re)
	}
	return &CLIAgent{config: cfg, promptPatterns: compiled}
}

func (a *CLIAgent) ID() string   { return a.config.AgentID }
func (a *CLIAgent) Name() string { return a.config.Name }

func (a *CLIAgent) Initialize() error { return nil }
func (a *CLIAgent) Shutdown() error   { return nil }

func (a *CLIAgent) GetCard() (types.AgentCard, error) { return a.config.Card, nil }

func (a *CLIAgent) GetCapabilities() types.RuntimeCapabilities {
	return types.RuntimeCapabilities{
		SupportsStreaming:    true,
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
	applyExecutionContext(command, ctx)
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

// ExecuteStreaming runs the agent with real-time output streaming and interactive input
func (a *CLIAgent) ExecuteStreaming(ctx types.ExecutionContext, output chan<- types.StreamEvent, input <-chan string) error {
	prompt := extractPrompt(ctx.UserMessage)
	if prompt == "" {
		output <- types.StreamEvent{Kind: "error", Text: "empty prompt", AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
		return errors.New("empty prompt")
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
	applyExecutionContext(command, ctx)

	// Start with PTY for interactive mode
	ptmx, err := pty.Start(command)
	if err != nil {
		output <- types.StreamEvent{Kind: "error", Text: err.Error(), AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
		return err
	}
	defer ptmx.Close()

	// Channel to signal completion
	done := make(chan struct{})

	// Goroutine: Read output and send to channel
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(ptmx)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			kind := "output"
			if a.isPrompt(line) {
				kind = "prompt"
			}
			output <- types.StreamEvent{
				Kind:      kind,
				Text:      line,
				AgentID:   a.ID(),
				TaskID:    ctx.TaskID,
				Timestamp: time.Now().UTC(),
			}
		}
		if err := scanner.Err(); err != nil {
			output <- types.StreamEvent{Kind: "error", Text: err.Error(), AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
		}
	}()

	// Goroutine: Forward user input to PTY
	go func() {
		for {
			select {
			case text, ok := <-input:
				if !ok {
					return
				}
				_, _ = ptmx.Write([]byte(text + "\n"))
			case <-done:
				return
			}
		}
	}()

	// Wait for completion
	if err := command.Wait(); err != nil {
		output <- types.StreamEvent{Kind: "error", Text: err.Error(), AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
		return err
	}

	// Wait for output reading to finish
	<-done

	output <- types.StreamEvent{Kind: "complete", AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
	return nil
}

func (a *CLIAgent) ExecPath() string {
	return a.config.Exec
}

// ExecuteWithArgs runs the agent with custom arguments (for agent extensions)
func (a *CLIAgent) ExecuteWithArgs(ctx types.ExecutionContext, customArgs []string) (types.ExecutionResult, error) {
	prompt := extractPrompt(ctx.UserMessage)
	if prompt == "" {
		return types.ExecutionResult{}, errors.New("empty prompt")
	}

	args := make([]string, 0, len(customArgs)+1)
	for _, arg := range customArgs {
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
	applyExecutionContext(command, ctx)
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

// ExecuteStreamingWithArgs runs the agent with custom arguments and real-time streaming
func (a *CLIAgent) ExecuteStreamingWithArgs(ctx types.ExecutionContext, customArgs []string, output chan<- types.StreamEvent, input <-chan string) error {
	prompt := extractPrompt(ctx.UserMessage)
	if prompt == "" {
		output <- types.StreamEvent{Kind: "error", Text: "empty prompt", AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
		return errors.New("empty prompt")
	}

	args := make([]string, 0, len(customArgs)+1)
	for _, arg := range customArgs {
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
	applyExecutionContext(command, ctx)

	// Start with PTY for interactive mode
	ptmx, err := pty.Start(command)
	if err != nil {
		output <- types.StreamEvent{Kind: "error", Text: err.Error(), AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
		return err
	}
	defer ptmx.Close()

	// Channel to signal completion
	done := make(chan struct{})

	// Goroutine: Read output and send to channel
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(ptmx)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			kind := "output"
			if a.isPrompt(line) {
				kind = "prompt"
			}
			output <- types.StreamEvent{
				Kind:      kind,
				Text:      line,
				AgentID:   a.ID(),
				TaskID:    ctx.TaskID,
				Timestamp: time.Now().UTC(),
			}
		}
		if err := scanner.Err(); err != nil {
			output <- types.StreamEvent{Kind: "error", Text: err.Error(), AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
		}
	}()

	// Goroutine: Forward user input to PTY
	go func() {
		for {
			select {
			case text, ok := <-input:
				if !ok {
					return
				}
				_, _ = ptmx.Write([]byte(text + "\n"))
			case <-done:
				return
			}
		}
	}()

	// Wait for completion
	if err := command.Wait(); err != nil {
		output <- types.StreamEvent{Kind: "error", Text: err.Error(), AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
		return err
	}

	// Wait for output reading to finish
	<-done

	output <- types.StreamEvent{Kind: "complete", AgentID: a.ID(), TaskID: ctx.TaskID, Timestamp: time.Now().UTC()}
	return nil
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

func applyExecutionContext(command *exec.Cmd, ctx types.ExecutionContext) {
	if strings.TrimSpace(ctx.WorkingDir) != "" {
		command.Dir = ctx.WorkingDir
	}
}

func (a *CLIAgent) isPrompt(line string) bool {
	if len(a.promptPatterns) == 0 {
		return false
	}
	for _, pattern := range a.promptPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}
