package agents

import (
	"fmt"
	"strings"

	"a2a-go/internal/types"
)

// CodexAgent wraps Codex CLI with flexible configuration.
type CodexAgent struct {
	*CLIAgent
	defaultConfig types.CodexConfig
}

func NewCodexAgent(baseURL string) *CodexAgent {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name:            "Codex CLI",
		Description:     "OpenAI Codex CLI wrapper with configurable sandbox and profiles",
		URL:             baseURL + "/agents/codex",
		Version:         "1.0.0",
		Provider:        types.Provider{Name: "OpenAI"},
		Skills:          codexSkills(),
		Capabilities:    types.AgentCapabilities{Streaming: true, PushNotifications: false, StateTransitionHistory: true},
	}

	cliAgent := NewCLIAgent(CLIConfig{
		AgentID:        "codex",
		Name:           "Codex CLI",
		Exec:           resolveExec("codex", "CODEX_CMD", "CODEX_EXEC"),
		HealthArgs:     []string{"--version"},
		Args:           []string{"exec", "{prompt}"},
		Card:           card,
		PromptPatterns: codexPromptPatterns(),
	})

	return &CodexAgent{
		CLIAgent:      cliAgent,
		defaultConfig: types.CodexConfig{},
	}
}

func (a *CodexAgent) SetDefaultConfig(config types.CodexConfig) {
	a.defaultConfig = config
}

func (a *CodexAgent) Execute(ctx types.ExecutionContext) (types.ExecutionResult, error) {
	config := a.extractCodexConfig(ctx)
	args := a.buildArgs(ctx, config)
	ctx = a.withCodexPrompt(ctx, config)
	return a.CLIAgent.ExecuteWithArgs(ctx, args)
}

func (a *CodexAgent) ExecuteStreaming(ctx types.ExecutionContext, output chan<- types.StreamEvent, input <-chan string) error {
	config := a.extractCodexConfig(ctx)
	args := a.buildArgs(ctx, config)
	ctx = a.withCodexPrompt(ctx, config)
	return a.CLIAgent.ExecuteStreamingWithArgs(ctx, args, output, input)
}

func (a *CodexAgent) extractCodexConfig(ctx types.ExecutionContext) types.CodexConfig {
	config := a.defaultConfig
	config.AddDirs = append([]string{}, config.AddDirs...)
	config.ConfigOverrides = append([]string{}, config.ConfigOverrides...)
	config.EnableFeatures = append([]string{}, config.EnableFeatures...)
	config.DisableFeatures = append([]string{}, config.DisableFeatures...)

	if ctx.UserMessage.Metadata == nil {
		return config
	}
	raw, ok := ctx.UserMessage.Metadata["codexConfig"]
	if !ok {
		return config
	}
	cfgMap, ok := raw.(map[string]any)
	if !ok {
		return config
	}
	if model, ok := cfgMap["model"].(string); ok {
		config.Model = model
	}
	if profile, ok := cfgMap["profile"].(string); ok {
		config.Profile = profile
	}
	if sandbox, ok := cfgMap["sandboxMode"].(string); ok {
		config.SandboxMode = types.CodexSandboxMode(sandbox)
	}
	if approval, ok := cfgMap["approvalPolicy"].(string); ok {
		config.ApprovalPolicy = types.CodexApprovalPolicy(approval)
	}
	if fullAuto, ok := cfgMap["fullAuto"].(bool); ok {
		config.FullAuto = fullAuto
	}
	if bypass, ok := cfgMap["bypassApprovals"].(bool); ok {
		config.BypassApprovals = bypass
	}
	if search, ok := cfgMap["search"].(bool); ok {
		config.Search = search
	}
	if workingDir, ok := cfgMap["workingDir"].(string); ok {
		config.WorkingDir = workingDir
	}
	if workingDir, ok := cfgMap["workingDirectory"].(string); ok {
		config.WorkingDir = workingDir
	}
	if prompt, ok := cfgMap["systemPrompt"].(string); ok {
		config.SystemPrompt = prompt
	}
	if includeHistory, ok := cfgMap["includeHistory"].(bool); ok {
		config.IncludeHistory = includeHistory
	}
	if addDirs, ok := cfgMap["addDirs"].([]string); ok {
		config.AddDirs = append([]string{}, addDirs...)
	} else if addDirs, ok := cfgMap["addDirs"].([]any); ok {
		config.AddDirs = toStringSlice(addDirs)
	}
	if overrides, ok := cfgMap["configOverrides"].([]string); ok {
		config.ConfigOverrides = append([]string{}, overrides...)
	} else if overrides, ok := cfgMap["configOverrides"].([]any); ok {
		config.ConfigOverrides = toStringSlice(overrides)
	}
	if features, ok := cfgMap["enableFeatures"].([]string); ok {
		config.EnableFeatures = append([]string{}, features...)
	} else if features, ok := cfgMap["enableFeatures"].([]any); ok {
		config.EnableFeatures = toStringSlice(features)
	}
	if features, ok := cfgMap["disableFeatures"].([]string); ok {
		config.DisableFeatures = append([]string{}, features...)
	} else if features, ok := cfgMap["disableFeatures"].([]any); ok {
		config.DisableFeatures = toStringSlice(features)
	}
	return config
}

func (a *CodexAgent) buildArgs(ctx types.ExecutionContext, config types.CodexConfig) []string {
	args := []string{}

	if config.Model != "" {
		args = append(args, "--model", config.Model)
	}
	if config.Profile != "" {
		args = append(args, "--profile", config.Profile)
	}
	if config.BypassApprovals {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	} else if config.FullAuto {
		args = append(args, "--full-auto")
	} else {
		if config.SandboxMode != "" && config.SandboxMode != types.CodexSandboxDefault {
			args = append(args, "--sandbox", string(config.SandboxMode))
		}
		if config.ApprovalPolicy != "" && config.ApprovalPolicy != types.CodexApprovalDefault {
			args = append(args, "--ask-for-approval", string(config.ApprovalPolicy))
		}
	}
	if config.Search {
		args = append(args, "--search")
	}

	workingDir := strings.TrimSpace(config.WorkingDir)
	if workingDir == "" {
		workingDir = strings.TrimSpace(ctx.WorkingDir)
	}
	if workingDir != "" {
		args = append(args, "--cd", workingDir)
	}

	for _, dir := range config.AddDirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		args = append(args, "--add-dir", dir)
	}
	for _, override := range config.ConfigOverrides {
		if strings.TrimSpace(override) == "" {
			continue
		}
		args = append(args, "--config", override)
	}
	for _, feature := range config.EnableFeatures {
		if strings.TrimSpace(feature) == "" {
			continue
		}
		args = append(args, "--enable", feature)
	}
	for _, feature := range config.DisableFeatures {
		if strings.TrimSpace(feature) == "" {
			continue
		}
		args = append(args, "--disable", feature)
	}

	args = append(args, "exec", "{prompt}")
	return args
}

func (a *CodexAgent) withCodexPrompt(ctx types.ExecutionContext, config types.CodexConfig) types.ExecutionContext {
	prompt := a.buildPrompt(ctx, config)
	ctx.UserMessage = types.Message{
		Kind:      "message",
		MessageID: ctx.UserMessage.MessageID,
		Role:      ctx.UserMessage.Role,
		Parts:     []types.Part{{Kind: "text", Text: prompt}},
		TaskID:    ctx.TaskID,
		ContextID: ctx.ContextID,
		Metadata:  ctx.UserMessage.Metadata,
	}
	if strings.TrimSpace(config.WorkingDir) != "" {
		ctx.WorkingDir = config.WorkingDir
	}
	return ctx
}

func (a *CodexAgent) buildPrompt(ctx types.ExecutionContext, config types.CodexConfig) string {
	userPrompt := strings.TrimSpace(extractPrompt(ctx.UserMessage))
	if userPrompt == "" {
		return ""
	}
	sections := make([]string, 0, 3)
	if strings.TrimSpace(config.SystemPrompt) != "" {
		sections = append(sections, "SYSTEM:\n"+strings.TrimSpace(config.SystemPrompt))
	}
	if config.IncludeHistory && len(ctx.PreviousHistory) > 0 {
		sections = append(sections, formatHistory(ctx.PreviousHistory))
	}
	sections = append(sections, userPrompt)
	return strings.Join(sections, "\n\n")
}

func formatHistory(history []types.Message) string {
	lines := []string{"Conversation history:"}
	for _, msg := range history {
		text := strings.TrimSpace(extractPrompt(msg))
		if text == "" {
			continue
		}
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", strings.ToUpper(role), text))
	}
	return strings.Join(lines, "\n")
}

func toStringSlice(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func codexPromptPatterns() []string {
	return []string{
		`(?i)\bapprove\b`,
		`(?i)\bapproval\b`,
		`(?i)\bproceed\b`,
		`(?i)\bcontinue\b`,
		`(?i)\bpress (enter|return)\b`,
		`(?i)\b(y/n|y/N|yes/no)\b`,
		`\[y/n\]`,
		`\[y/N\]`,
	}
}

func codexSkills() []types.Skill {
	return []types.Skill{
		{
			ID:          "code-review",
			Name:        "Code Review",
			Description: "Review code for quality, bugs, security issues, and best practices",
			Tags:        []string{"review", "quality", "security"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "refactor",
			Name:        "Refactoring",
			Description: "Restructure existing code without changing behavior",
			Tags:        []string{"refactor", "cleanup", "structure"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "write-tests",
			Name:        "Test Writing",
			Description: "Write unit tests, integration tests, and test fixtures",
			Tags:        []string{"testing", "tests", "unit-test"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "debug",
			Name:        "Debugging",
			Description: "Diagnose and fix bugs in code",
			Tags:        []string{"debug", "fix", "troubleshoot"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "explain",
			Name:        "Code Explanation",
			Description: "Explain how code works, its architecture and design patterns",
			Tags:        []string{"explain", "understand", "documentation"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "document",
			Name:        "Documentation",
			Description: "Write documentation, READMEs, comments, and API docs",
			Tags:        []string{"docs", "readme", "comments"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "architect",
			Name:        "Architecture Design",
			Description: "Design system architecture and high-level structure",
			Tags:        []string{"architecture", "design", "planning"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "implement",
			Name:        "Implementation",
			Description: "Write new code and implement features",
			Tags:        []string{"implement", "code", "feature"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
	}
}
