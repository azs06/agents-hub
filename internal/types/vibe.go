package types

// VibeConfig contains Vibe-specific execution options
// Note: Vibe CLI is configured primarily through ~/.vibe/config.toml
// The CLI only accepts --prompt for non-interactive mode and --agent for custom agents
type VibeConfig struct {
	// Agent configuration name (from ~/.vibe/agents/)
	Agent string `json:"agent,omitempty"`

	// NonInteractive runs in non-interactive mode with auto-approve
	// Uses --prompt flag instead of positional argument
	NonInteractive bool `json:"nonInteractive,omitempty"`

	// AutoApprove enables automatic tool approval (Shift+Tab toggle in interactive mode)
	// In non-interactive mode (--prompt), this is always enabled
	AutoApprove bool `json:"autoApprove,omitempty"`

	// IncludeHistory prepends conversation history to the prompt
	IncludeHistory bool `json:"includeHistory,omitempty"`

	// SystemPrompt is prepended to the user prompt
	// Note: This is sent as part of the prompt, not a CLI flag
	SystemPrompt string `json:"systemPrompt,omitempty"`
}

// VibeSettings contains persistent Vibe configuration
type VibeSettings struct {
	// DefaultAgent is the agent configuration to use by default
	DefaultAgent string `json:"defaultAgent,omitempty"`

	// NonInteractive runs in non-interactive mode by default
	NonInteractive bool `json:"nonInteractive,omitempty"`

	// AutoApprove enables automatic tool approval by default
	AutoApprove bool `json:"autoApprove,omitempty"`

	// IncludeHistory includes conversation history by default
	IncludeHistory bool `json:"includeHistory,omitempty"`

	// DefaultSystemPrompt is prepended to prompts by default
	DefaultSystemPrompt string `json:"defaultSystemPrompt,omitempty"`
}

// ValidVibeAgents returns common agent configuration names
// Users can create custom agents in ~/.vibe/agents/
func ValidVibeAgents() []string {
	return []string{"", "default", "coder", "reviewer", "architect"}
}