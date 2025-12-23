package agents

import "a2a-go/internal/types"

func NewVibeAgent(baseURL string) *CLIAgent {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name: "Vibe CLI",
		Description: "Vibe CLI wrapper",
		URL: baseURL + "/agents/vibe",
		Version: "1.0.0",
		Provider: types.Provider{Name: "Mistral"},
		Skills: []types.Skill{},
		Capabilities: types.AgentCapabilities{Streaming: false, PushNotifications: false, StateTransitionHistory: false},
	}
	return NewCLIAgent(CLIConfig{
		AgentID: "vibe",
		Name: "Vibe CLI",
		Exec: resolveExec("vibe", "VIBE_CMD", "VIBE_EXEC"),
		HealthArgs: []string{"--help"},
		Args: []string{"-p", "{prompt}", "--output", "text"},
		Card: card,
	})
}
