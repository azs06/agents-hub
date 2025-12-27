package agents

import (
	"strings"

	"agents-hub/internal/types"
)

// GeminiAgent wraps Gemini CLI with enhanced configuration
type GeminiAgent struct {
	*CLIAgent
	defaultConfig types.GeminiConfig
}

// NewGeminiAgent creates a new Gemini agent
func NewGeminiAgent(baseURL string) *GeminiAgent {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name:            "Gemini CLI",
		Description:     "Gemini CLI wrapper with flexible configuration",
		URL:             baseURL + "/agents/gemini",
		Version:         "1.0.0",
		Provider:        types.Provider{Name: "Google"},
		Skills:          geminiSkills(),
		Capabilities:    types.AgentCapabilities{Streaming: true, PushNotifications: false, StateTransitionHistory: true},
	}

	cliAgent := NewCLIAgent(CLIConfig{
		AgentID:        "gemini",
		Name:           "Gemini CLI",
		Exec:           resolveExec("gemini", "GEMINI_CMD", "GEMINI_EXEC"),
		HealthArgs:     []string{"--version"},
		Args:           []string{"{prompt}", "-o", "text"},
		Card:           card,
		PromptPatterns: codexPromptPatterns(),
	})

	return &GeminiAgent{
		CLIAgent:      cliAgent,
		defaultConfig: types.GeminiConfig{},
	}
}

// SetDefaultConfig sets the default configuration for this agent
func (a *GeminiAgent) SetDefaultConfig(config types.GeminiConfig) {
	a.defaultConfig = config
}

// Execute runs Gemini with dynamic arguments based on config
func (a *GeminiAgent) Execute(ctx types.ExecutionContext) (types.ExecutionResult, error) {
	config := a.extractGeminiConfig(ctx)
	args := a.buildArgs(config)
	return a.CLIAgent.ExecuteWithArgs(ctx, args)
}

// ExecuteStreaming runs Gemini with streaming and dynamic arguments
func (a *GeminiAgent) ExecuteStreaming(ctx types.ExecutionContext, output chan<- types.StreamEvent, input <-chan string) error {
	config := a.extractGeminiConfig(ctx)
	args := a.buildArgs(config)
	return a.CLIAgent.ExecuteStreamingWithArgs(ctx, args, output, input)
}

// extractGeminiConfig gets GeminiConfig from execution context metadata or defaults
func (a *GeminiAgent) extractGeminiConfig(ctx types.ExecutionContext) types.GeminiConfig {
	config := a.defaultConfig

	if ctx.UserMessage.Metadata != nil {
		if cfgRaw, ok := ctx.UserMessage.Metadata["geminiConfig"]; ok {
			if cfgMap, ok := cfgRaw.(map[string]any); ok {
				if model, ok := cfgMap["model"].(string); ok {
					config.Model = types.GeminiModel(model)
				}
				if resume, ok := cfgMap["resume"].(string); ok {
					config.Resume = resume
				} else if sid, ok := cfgMap["sessionId"].(string); ok {
					config.Resume = sid
				}
				if sandbox, ok := cfgMap["sandbox"].(bool); ok {
					config.Sandbox = sandbox
				}
				if mode, ok := cfgMap["approvalMode"].(string); ok {
					config.ApprovalMode = mode
				}
				if tools, ok := cfgMap["allowedTools"].([]any); ok {
					config.AllowedTools = make([]string, 0, len(tools))
					for _, t := range tools {
						if s, ok := t.(string); ok {
							config.AllowedTools = append(config.AllowedTools, s)
						}
					}
				}
				if dirs, ok := cfgMap["includeDirectories"].([]any); ok {
					config.IncludeDirectories = make([]string, 0, len(dirs))
					for _, d := range dirs {
						if s, ok := d.(string); ok {
							config.IncludeDirectories = append(config.IncludeDirectories, s)
						}
					}
				}
			}
		}
	}
	return config
}

// buildArgs constructs CLI arguments from GeminiConfig
func (a *GeminiAgent) buildArgs(config types.GeminiConfig) []string {
	args := []string{}

	if config.Resume != "" {
		args = append(args, "--resume", config.Resume)
	}

	if config.Model != "" && config.Model != types.GeminiModelDefault {
		args = append(args, "--model", string(config.Model))
	}

	if config.Sandbox {
		args = append(args, "--sandbox")
	}

	if config.ApprovalMode != "" {
		args = append(args, "--approval-mode", config.ApprovalMode)
	}

	if len(config.AllowedTools) > 0 {
		args = append(args, "--allowed-tools", strings.Join(config.AllowedTools, ","))
	}

	if len(config.IncludeDirectories) > 0 {
		args = append(args, "--include-directories", strings.Join(config.IncludeDirectories, ","))
	}

	// Base args
	args = append(args, "{prompt}", "-o", "text")

	return args
}

func geminiSkills() []types.Skill {
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
