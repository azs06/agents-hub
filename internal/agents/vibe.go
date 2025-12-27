package agents

import (
	"strings"

	"agents-hub/internal/types"
)

// VibeAgent wraps Mistral Vibe CLI with enhanced configuration
// Vibe is a command-line coding assistant powered by Mistral's models
// See: https://github.com/mistralai/mistral-vibe
type VibeAgent struct {
	*CLIAgent
	defaultConfig types.VibeConfig
}

// NewVibeAgent creates a new Vibe agent with skills and capabilities
func NewVibeAgent(baseURL string) *VibeAgent {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name:            "Mistral Vibe",
		Description:     "Mistral Vibe CLI - command-line coding assistant powered by Mistral's models",
		URL:             baseURL + "/agents/vibe",
		Version:         "1.0.0",
		Provider:        types.Provider{Name: "Mistral"},
		Skills:          vibeSkills(),
		Capabilities:    types.AgentCapabilities{Streaming: true, PushNotifications: false, StateTransitionHistory: true},
	}

	// Default args use --prompt for non-interactive mode with auto-approve
	cliAgent := NewCLIAgent(CLIConfig{
		AgentID:        "vibe",
		Name:           "Mistral Vibe",
		Exec:           resolveExec("vibe", "VIBE_CMD", "VIBE_EXEC"),
		HealthArgs:     []string{"--help"},
		Args:           []string{"--prompt", "{prompt}"},
		Card:           card,
		PromptPatterns: vibePromptPatterns(),
	})

	return &VibeAgent{
		CLIAgent:      cliAgent,
		defaultConfig: types.VibeConfig{NonInteractive: true},
	}
}

// SetDefaultConfig sets the default configuration for this agent
func (a *VibeAgent) SetDefaultConfig(config types.VibeConfig) {
	a.defaultConfig = config
}

// Execute runs Vibe with dynamic arguments based on config
func (a *VibeAgent) Execute(ctx types.ExecutionContext) (types.ExecutionResult, error) {
	config := a.extractVibeConfig(ctx)
	ctx = a.withVibePrompt(ctx, config)
	// Clear PreviousHistory since withVibePrompt already incorporated it if IncludeHistory was set
	// This prevents the base ExecuteWithArgs from adding history again
	ctx.PreviousHistory = nil
	args := a.buildArgs(config)
	return a.CLIAgent.ExecuteWithArgs(ctx, args)
}

// Note: ExecuteStreaming is intentionally NOT implemented for VibeAgent.
// Vibe's TUI conflicts with agents-hub TUI when run via PTY.
// The TUI will fall back to regular Execute() which uses stdin pipe.

// extractVibeConfig gets VibeConfig from execution context metadata or defaults
func (a *VibeAgent) extractVibeConfig(ctx types.ExecutionContext) types.VibeConfig {
	// Start with default config
	config := a.defaultConfig

	// Check if config is passed in message metadata
	if ctx.UserMessage.Metadata != nil {
		if cfgRaw, ok := ctx.UserMessage.Metadata["vibeConfig"].(map[string]any); ok {
			// Parse agent name
			if agent, ok := cfgRaw["agent"].(string); ok {
				config.Agent = agent
			}
			// Parse nonInteractive
			if nonInteractive, ok := cfgRaw["nonInteractive"].(bool); ok {
				config.NonInteractive = nonInteractive
			}
			// Parse autoApprove
			if autoApprove, ok := cfgRaw["autoApprove"].(bool); ok {
				config.AutoApprove = autoApprove
			}
			// Parse includeHistory
			if includeHistory, ok := cfgRaw["includeHistory"].(bool); ok {
				config.IncludeHistory = includeHistory
			}
			// Parse systemPrompt
			if systemPrompt, ok := cfgRaw["systemPrompt"].(string); ok {
				config.SystemPrompt = systemPrompt
			}
		}
	}

	return config
}

// withVibePrompt builds the prompt with system prompt and history if configured
func (a *VibeAgent) withVibePrompt(ctx types.ExecutionContext, config types.VibeConfig) types.ExecutionContext {
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
	return ctx
}

// buildPrompt constructs the full prompt including system prompt and history
func (a *VibeAgent) buildPrompt(ctx types.ExecutionContext, config types.VibeConfig) string {
	userPrompt := strings.TrimSpace(extractPrompt(ctx.UserMessage))
	if userPrompt == "" {
		return ""
	}

	sections := make([]string, 0, 3)

	// Add system prompt if configured
	if strings.TrimSpace(config.SystemPrompt) != "" {
		sections = append(sections, "SYSTEM:\n"+strings.TrimSpace(config.SystemPrompt))
	}

	// Always add conversation history for multi-agent awareness
	// This ensures all agents see the full cross-agent conversation
	if len(ctx.PreviousHistory) > 0 {
		sections = append(sections, a.formatHistory(ctx.PreviousHistory))
	}

	sections = append(sections, userPrompt)
	return strings.Join(sections, "\n\n")
}

// formatHistory formats conversation history for the prompt with agent attribution
func (a *VibeAgent) formatHistory(history []types.Message) string {
	// Use the cross-agent history format for consistency
	return formatCrossAgentHistory(history)
}

// buildArgs constructs CLI arguments from VibeConfig
// Vibe CLI supports:
//   - vibe "prompt" : Interactive mode with initial prompt
//   - vibe --prompt "text" : Non-interactive mode with auto-approve
//   - vibe --agent name : Use custom agent configuration
//   - vibe --output text : Force text output (no TUI)
func (a *VibeAgent) buildArgs(config types.VibeConfig) []string {
	args := []string{}

	// Agent configuration (from ~/.vibe/agents/)
	if strings.TrimSpace(config.Agent) != "" {
		args = append(args, "--agent", config.Agent)
	}

	// Use --prompt for non-interactive mode (auto-approves tools)
	// Otherwise use positional argument for interactive mode
	if config.NonInteractive {
		// Force text output to prevent vibe's TUI from rendering
		args = append(args, "--prompt", "{prompt}", "--output", "text")
	} else {
		args = append(args, "{prompt}")
	}

	return args
}

// vibePromptPatterns returns patterns that indicate Vibe is waiting for input
func vibePromptPatterns() []string {
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

// vibeSkills returns the skills Vibe can perform
func vibeSkills() []types.Skill {
	return []types.Skill{
		{
			ID:          "code-generation",
			Name:        "Code Generation",
			Description: "Generate code in various programming languages",
			Tags:        []string{"code", "generate", "implement"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "code-review",
			Name:        "Code Review",
			Description: "Review code for quality, bugs, and best practices",
			Tags:        []string{"review", "quality", "analysis"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "code-explanation",
			Name:        "Code Explanation",
			Description: "Explain how code works and its architecture",
			Tags:        []string{"explain", "document", "understand"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "refactoring",
			Name:        "Code Refactoring",
			Description: "Improve code structure without changing behavior",
			Tags:        []string{"refactor", "cleanup", "optimize"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "test-generation",
			Name:        "Test Generation",
			Description: "Generate unit tests and test cases",
			Tags:        []string{"testing", "tests", "quality"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "documentation",
			Name:        "Documentation Writing",
			Description: "Write documentation, comments, and API docs",
			Tags:        []string{"docs", "comments", "readme"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "debugging",
			Name:        "Debugging Assistance",
			Description: "Help diagnose and fix bugs in code",
			Tags:        []string{"debug", "fix", "troubleshoot"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
		{
			ID:          "architecture-design",
			Name:        "Architecture Design",
			Description: "Design system architecture and structure",
			Tags:        []string{"architecture", "design", "planning"},
			InputModes:  []string{"text/plain"},
			OutputModes: []string{"text/plain"},
		},
	}
}
