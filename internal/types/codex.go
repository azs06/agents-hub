package types

// CodexSandboxMode represents Codex CLI sandbox modes.
type CodexSandboxMode string

const (
	CodexSandboxDefault        CodexSandboxMode = ""
	CodexSandboxReadOnly       CodexSandboxMode = "read-only"
	CodexSandboxWorkspaceWrite CodexSandboxMode = "workspace-write"
	CodexSandboxDangerFull     CodexSandboxMode = "danger-full-access"
)

// CodexApprovalPolicy represents Codex CLI approval policies.
type CodexApprovalPolicy string

const (
	CodexApprovalDefault   CodexApprovalPolicy = ""
	CodexApprovalUntrusted CodexApprovalPolicy = "untrusted"
	CodexApprovalOnFailure CodexApprovalPolicy = "on-failure"
	CodexApprovalOnRequest CodexApprovalPolicy = "on-request"
	CodexApprovalNever     CodexApprovalPolicy = "never"
)

// CodexConfig contains Codex CLI execution options.
type CodexConfig struct {
	Model           string              `json:"model,omitempty"`
	Profile         string              `json:"profile,omitempty"`
	SandboxMode     CodexSandboxMode    `json:"sandboxMode,omitempty"`
	ApprovalPolicy  CodexApprovalPolicy `json:"approvalPolicy,omitempty"`
	FullAuto        bool                `json:"fullAuto,omitempty"`
	BypassApprovals bool                `json:"bypassApprovals,omitempty"`
	Search          bool                `json:"search,omitempty"`
	WorkingDir      string              `json:"workingDir,omitempty"`
	AddDirs         []string            `json:"addDirs,omitempty"`
	ConfigOverrides []string            `json:"configOverrides,omitempty"`
	EnableFeatures  []string            `json:"enableFeatures,omitempty"`
	DisableFeatures []string            `json:"disableFeatures,omitempty"`
	SystemPrompt    string              `json:"systemPrompt,omitempty"`
	IncludeHistory  bool                `json:"includeHistory,omitempty"`
}

// CodexSettings contains persistent Codex configuration.
type CodexSettings struct {
	DefaultModel          string   `json:"defaultModel,omitempty"`
	DefaultProfile        string   `json:"defaultProfile,omitempty"`
	DefaultSandbox        string   `json:"defaultSandbox,omitempty"`
	DefaultApprovalPolicy string   `json:"defaultApprovalPolicy,omitempty"`
	EnableSearch          bool     `json:"enableSearch,omitempty"`
	FullAuto              bool     `json:"fullAuto,omitempty"`
	BypassApprovals       bool     `json:"bypassApprovals,omitempty"`
	DefaultWorkingDir     string   `json:"defaultWorkingDir,omitempty"`
	DefaultSystemPrompt   string   `json:"defaultSystemPrompt,omitempty"`
	DefaultAddDirs        []string `json:"defaultAddDirs,omitempty"`
	ConfigOverrides       []string `json:"configOverrides,omitempty"`
	EnableFeatures        []string `json:"enableFeatures,omitempty"`
	DisableFeatures       []string `json:"disableFeatures,omitempty"`
	IncludeHistory        bool     `json:"includeHistory,omitempty"`
}

// ValidCodexSandboxModes returns supported sandbox modes.
func ValidCodexSandboxModes() []CodexSandboxMode {
	return []CodexSandboxMode{CodexSandboxDefault, CodexSandboxReadOnly, CodexSandboxWorkspaceWrite, CodexSandboxDangerFull}
}

// ValidCodexApprovalPolicies returns supported approval policies.
func ValidCodexApprovalPolicies() []CodexApprovalPolicy {
	return []CodexApprovalPolicy{CodexApprovalDefault, CodexApprovalUntrusted, CodexApprovalOnFailure, CodexApprovalOnRequest, CodexApprovalNever}
}
