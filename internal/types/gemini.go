package types

// GeminiModel represents available Gemini models
type GeminiModel string

const (
	GeminiModelDefault GeminiModel = ""                 // Use Gemini's default
	GeminiModel15Pro   GeminiModel = "gemini-1.5-pro"   // Gemini 1.5 Pro
	GeminiModel15Flash GeminiModel = "gemini-1.5-flash" // Gemini 1.5 Flash
	GeminiModel20Flash GeminiModel = "gemini-2.0-flash" // Gemini 2.0 Flash
)

// GeminiConfig contains Gemini-specific execution options
type GeminiConfig struct {
	// Session continuation
	Resume    string `json:"resume,omitempty"`    // Use --resume <id>
	SessionID string `json:"sessionId,omitempty"` // Alias for Resume

	// Model selection
	Model GeminiModel `json:"model,omitempty"`

	// Execution mode
	Sandbox      bool   `json:"sandbox,omitempty"`      // Use --sandbox
	ApprovalMode string `json:"approvalMode,omitempty"` // Use --approval-mode <mode>

	// Capabilities
	AllowedTools       []string `json:"allowedTools,omitempty"`       // Use --allowed-tools
	IncludeDirectories []string `json:"includeDirectories,omitempty"` // Use --include-directories
}

// GeminiSettings contains persistent Gemini configuration
type GeminiSettings struct {
	DefaultModel        string   `json:"defaultModel,omitempty"`
	DefaultSandbox      bool     `json:"defaultSandbox,omitempty"`
	DefaultApprovalMode string   `json:"defaultApprovalMode,omitempty"` // default, auto_edit, yolo
	CustomAllowedTools  []string `json:"customAllowedTools,omitempty"`
	DefaultIncludeDirs  []string `json:"defaultIncludeDirs,omitempty"`
	ResumeSession       string   `json:"resumeSession,omitempty"`
}

// ValidGeminiModels returns all valid model options
func ValidGeminiModels() []GeminiModel {
	return []GeminiModel{GeminiModelDefault, GeminiModel15Pro, GeminiModel15Flash, GeminiModel20Flash}
}
