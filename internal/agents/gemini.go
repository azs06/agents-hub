package agents

import "a2a-go/internal/types"

func NewGeminiAgent(baseURL string) *CLIAgent {
	card := types.AgentCard{
		ProtocolVersion: "1.0",
		Name: "Gemini CLI",
		Description: "Gemini CLI wrapper",
		URL: baseURL + "/agents/gemini",
		Version: "1.0.0",
		Provider: types.Provider{Name: "Google"},
		Skills: []types.Skill{},
		Capabilities: types.AgentCapabilities{Streaming: false, PushNotifications: false, StateTransitionHistory: false},
	}
	return NewCLIAgent(CLIConfig{
		AgentID: "gemini",
		Name: "Gemini CLI",
		Exec: resolveExec("gemini", "GEMINI_CMD", "GEMINI_EXEC"),
		HealthArgs: []string{"--version"},
		Args: []string{"{prompt}", "-o", "text"},
		Card: card,
	})
}
