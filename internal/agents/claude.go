package agents

import "a2a-go/internal/types"

func NewClaudeAgent(baseURL string) *CLIAgent {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name: "Claude Code CLI",
		Description: "Claude Code CLI wrapper",
		URL: baseURL + "/agents/claude-code",
		Version: "1.0.0",
		Provider: types.Provider{Name: "Anthropic"},
		Skills: []types.Skill{},
		Capabilities: types.AgentCapabilities{Streaming: false, PushNotifications: false, StateTransitionHistory: false},
	}
	return NewCLIAgent(CLIConfig{
		AgentID: "claude-code",
		Name: "Claude Code CLI",
		Exec: resolveExecWithFallback("claude", []string{"/Users/soikat/.claude/local/claude"}, "CLAUDE_CMD", "CLAUDE_EXEC"),
		HealthArgs: []string{"--version"},
		Args: []string{"-p", "{prompt}", "--output-format", "text"},
		Card: card,
	})
}
