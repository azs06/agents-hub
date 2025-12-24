package agents

import (
	"strings"

	"a2a-go/internal/types"
)

// ClaudeAgent wraps Claude CLI with enhanced configuration
type ClaudeAgent struct {
	*CLIAgent
	defaultConfig types.ClaudeConfig
}

// NewClaudeAgent creates a new Claude agent with skills and capabilities
func NewClaudeAgent(baseURL string) *ClaudeAgent {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name:            "Claude Code CLI",
		Description:     "Claude Code CLI wrapper with flexible configuration",
		URL:             baseURL + "/agents/claude-code",
		Version:         "1.0.0",
		Provider:        types.Provider{Name: "Anthropic"},
		Skills:          claudeSkills(),
		Capabilities:    types.AgentCapabilities{Streaming: true, PushNotifications: false, StateTransitionHistory: true},
	}

	cliAgent := NewCLIAgent(CLIConfig{
		AgentID:    "claude-code",
		Name:       "Claude Code CLI",
		Exec:       resolveExecWithFallback("claude", []string{"/Users/soikat/.claude/local/claude"}, "CLAUDE_CMD", "CLAUDE_EXEC"),
		HealthArgs: []string{"--version"},
		Args:       []string{"-p", "{prompt}", "--output-format", "text"}, // Base args (used when no config)
		Card:       card,
	})

	return &ClaudeAgent{
		CLIAgent:      cliAgent,
		defaultConfig: types.ClaudeConfig{},
	}
}

// SetDefaultConfig sets the default configuration for this agent
func (a *ClaudeAgent) SetDefaultConfig(config types.ClaudeConfig) {
	a.defaultConfig = config
}

// Execute runs Claude with dynamic arguments based on config
func (a *ClaudeAgent) Execute(ctx types.ExecutionContext) (types.ExecutionResult, error) {
	config := a.extractClaudeConfig(ctx)
	args := a.buildArgs(config)
	return a.CLIAgent.ExecuteWithArgs(ctx, args)
}

// ExecuteStreaming runs Claude with streaming and dynamic arguments
func (a *ClaudeAgent) ExecuteStreaming(ctx types.ExecutionContext, output chan<- types.StreamEvent, input <-chan string) error {
	config := a.extractClaudeConfig(ctx)
	args := a.buildArgs(config)
	return a.CLIAgent.ExecuteStreamingWithArgs(ctx, args, output, input)
}

// extractClaudeConfig gets ClaudeConfig from execution context metadata or defaults
func (a *ClaudeAgent) extractClaudeConfig(ctx types.ExecutionContext) types.ClaudeConfig {
	// Start with default config
	config := a.defaultConfig

	// Check if config is passed in message metadata
	if ctx.UserMessage.Metadata != nil {
		if cfgRaw, ok := ctx.UserMessage.Metadata["claudeConfig"]; ok {
			if cfgMap, ok := cfgRaw.(map[string]any); ok {
				// Parse continue
				if cont, ok := cfgMap["continue"].(bool); ok {
					config.Continue = cont
				}
				// Parse sessionId
				if sid, ok := cfgMap["sessionId"].(string); ok {
					config.SessionID = sid
				}
				// Parse model
				if model, ok := cfgMap["model"].(string); ok {
					config.Model = types.ClaudeModel(model)
				}
				// Parse toolProfile
				if profile, ok := cfgMap["toolProfile"].(string); ok {
					config.ToolProfile = types.ClaudeToolProfile(profile)
				}
				// Parse allowedTools
				if tools, ok := cfgMap["allowedTools"].([]any); ok {
					config.AllowedTools = make([]string, 0, len(tools))
					for _, t := range tools {
						if s, ok := t.(string); ok {
							config.AllowedTools = append(config.AllowedTools, s)
						}
					}
				}
			}
		}
	}

	return config
}

// buildArgs constructs CLI arguments from ClaudeConfig
func (a *ClaudeAgent) buildArgs(config types.ClaudeConfig) []string {
	args := []string{}

	// Session continuation
	if config.Continue {
		args = append(args, "--continue")
	} else if config.SessionID != "" {
		args = append(args, "--resume", config.SessionID)
	}

	// Model selection
	if config.Model != "" && config.Model != types.ClaudeModelDefault {
		args = append(args, "--model", string(config.Model))
	}

	// Tool restrictions
	if len(config.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(config.AllowedTools, ","))
	} else if config.ToolProfile != "" && config.ToolProfile != types.ClaudeToolsDefault {
		tools := types.GetToolsForProfile(config.ToolProfile)
		if len(tools) > 0 {
			args = append(args, "--allowedTools", strings.Join(tools, ","))
		}
	}

	// Base args (prompt and output format)
	args = append(args, "-p", "{prompt}", "--output-format", "text")

	return args
}

// claudeSkills returns the skills Claude can perform
func claudeSkills() []types.Skill {
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
