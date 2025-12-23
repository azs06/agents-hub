package agents

import "a2a-go/internal/types"

func NewCodexAgent(baseURL string) *CLIAgent {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name: "Codex CLI",
		Description: "OpenAI Codex CLI wrapper",
		URL: baseURL + "/agents/codex",
		Version: "1.0.0",
		Provider: types.Provider{Name: "OpenAI"},
		Skills: []types.Skill{},
		Capabilities: types.AgentCapabilities{Streaming: false, PushNotifications: false, StateTransitionHistory: false},
	}
	return NewCLIAgent(CLIConfig{
		AgentID: "codex",
		Name: "Codex CLI",
		Exec: resolveExec("codex", "CODEX_CMD", "CODEX_EXEC"),
		HealthArgs: []string{"--version"},
		Args: []string{"exec", "{prompt}"},
		Card: card,
	})
}
