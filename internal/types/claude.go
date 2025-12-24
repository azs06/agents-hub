package types

// ClaudeModel represents available Claude models
type ClaudeModel string

const (
	ClaudeModelDefault ClaudeModel = ""       // Use Claude's default
	ClaudeModelOpus    ClaudeModel = "opus"   // Claude Opus
	ClaudeModelSonnet  ClaudeModel = "sonnet" // Claude Sonnet
	ClaudeModelHaiku   ClaudeModel = "haiku"  // Claude Haiku
)

// ClaudeToolProfile represents predefined tool restriction profiles
type ClaudeToolProfile string

const (
	ClaudeToolsDefault ClaudeToolProfile = ""       // All tools enabled (no restriction)
	ClaudeToolsSafe    ClaudeToolProfile = "safe"   // Read-only: Read, Glob, Grep, LSP
	ClaudeToolsNormal  ClaudeToolProfile = "normal" // Standard: excludes dangerous ops
	ClaudeToolsFull    ClaudeToolProfile = "full"   // Explicit all tools
)

// ToolProfiles maps profile names to allowed tool lists
var ToolProfiles = map[ClaudeToolProfile][]string{
	ClaudeToolsSafe:   {"Read", "Glob", "Grep", "LSP"},
	ClaudeToolsNormal: {"Read", "Glob", "Grep", "Edit", "Write", "LSP", "WebFetch", "WebSearch"},
	ClaudeToolsFull:   {}, // Empty means all tools (no --allowedTools flag)
}

// ClaudeConfig contains Claude-specific execution options
type ClaudeConfig struct {
	// Session continuation
	Continue  bool   `json:"continue,omitempty"`  // Use --continue flag
	SessionID string `json:"sessionId,omitempty"` // Use --resume <id>

	// Model selection
	Model ClaudeModel `json:"model,omitempty"`

	// Tool restrictions
	ToolProfile  ClaudeToolProfile `json:"toolProfile,omitempty"`
	AllowedTools []string          `json:"allowedTools,omitempty"` // Custom tool list (overrides profile)
}

// ClaudeSettings contains persistent Claude configuration
type ClaudeSettings struct {
	DefaultModel       string   `json:"defaultModel,omitempty"`       // opus, sonnet, haiku
	DefaultToolProfile string   `json:"defaultToolProfile,omitempty"` // safe, normal, full
	CustomAllowedTools []string `json:"customAllowedTools,omitempty"` // User-defined tool list
	EnableContinue     bool     `json:"enableContinue,omitempty"`     // Default continue behavior
}

// GetToolsForProfile returns the tool list for a given profile
func GetToolsForProfile(profile ClaudeToolProfile) []string {
	if tools, ok := ToolProfiles[profile]; ok {
		return tools
	}
	return nil
}

// ValidClaudeModels returns all valid model options
func ValidClaudeModels() []ClaudeModel {
	return []ClaudeModel{ClaudeModelDefault, ClaudeModelOpus, ClaudeModelSonnet, ClaudeModelHaiku}
}

// ValidToolProfiles returns all valid tool profile options
func ValidToolProfiles() []ClaudeToolProfile {
	return []ClaudeToolProfile{ClaudeToolsDefault, ClaudeToolsSafe, ClaudeToolsNormal, ClaudeToolsFull}
}
