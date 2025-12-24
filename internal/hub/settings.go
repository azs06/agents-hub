package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"a2a-go/internal/types"
	"a2a-go/internal/utils"
)

type Settings struct {
	OrchestratorAgents []string             `json:"orchestratorAgents"`
	LastAgent          string               `json:"lastAgent"`
	Claude             types.ClaudeSettings `json:"claude,omitempty"`
	Codex              types.CodexSettings  `json:"codex,omitempty"`
	Gemini             types.GeminiSettings `json:"gemini,omitempty"`
	Vibe               types.VibeSettings   `json:"vibe,omitempty"`
}

func (s *Server) SettingsPath() string {
	return filepath.Join(s.cfg.DataDir, "settings.json")
}

func (s *Server) LoadSettings() error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}
	data, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return err
	}
	s.settings = settings
	if settings.OrchestratorAgents != nil {
		s.cfg.Orchestrator.Agents = append([]string{}, settings.OrchestratorAgents...)
	} else {
		s.settings.OrchestratorAgents = append([]string{}, s.cfg.Orchestrator.Agents...)
	}
	_ = s.UpdateOrchestratorAgents(s.cfg.Orchestrator.Agents)
	return nil
}

func (s *Server) SaveSettings() error {
	if err := s.EnsureDataDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	return utils.WriteFileAtomic(s.SettingsPath(), data, 0o644)
}

func (s *Server) updateSettingsAgents(ids []string) {
	s.settings.OrchestratorAgents = append([]string{}, ids...)
}

func (s *Server) UpdateLastAgent(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if s.settings.LastAgent == id {
		return
	}
	s.settings.LastAgent = id
	if err := s.SaveSettings(); err != nil {
		s.logger.Warnf("failed to save settings: %v", err)
	}
}

func (s *Server) LastAgent() string {
	return s.settings.LastAgent
}

// ClaudeSettings returns the current Claude configuration
func (s *Server) ClaudeSettings() types.ClaudeSettings {
	return s.settings.Claude
}

// UpdateClaudeSettings updates Claude configuration and persists it
func (s *Server) UpdateClaudeSettings(settings types.ClaudeSettings) error {
	s.settings.Claude = settings
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateClaudeModel updates the default Claude model
func (s *Server) UpdateClaudeModel(model string) error {
	s.settings.Claude.DefaultModel = model
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateClaudeToolProfile updates the default tool profile
func (s *Server) UpdateClaudeToolProfile(profile string) error {
	s.settings.Claude.DefaultToolProfile = profile
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateClaudeContinue updates the continue mode setting
func (s *Server) UpdateClaudeContinue(enabled bool) error {
	s.settings.Claude.EnableContinue = enabled
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// GetClaudeConfig builds a ClaudeConfig from current settings
func (s *Server) GetClaudeConfig() types.ClaudeConfig {
	return types.ClaudeConfig{
		Continue:     s.settings.Claude.EnableContinue,
		Model:        types.ClaudeModel(s.settings.Claude.DefaultModel),
		ToolProfile:  types.ClaudeToolProfile(s.settings.Claude.DefaultToolProfile),
		AllowedTools: s.settings.Claude.CustomAllowedTools,
	}
}

// CodexSettings returns the current Codex configuration.
func (s *Server) CodexSettings() types.CodexSettings {
	return s.settings.Codex
}

// UpdateCodexSettings updates Codex configuration and persists it.
func (s *Server) UpdateCodexSettings(settings types.CodexSettings) error {
	s.settings.Codex = settings
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateCodexModel updates the default Codex model.
func (s *Server) UpdateCodexModel(model string) error {
	s.settings.Codex.DefaultModel = model
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateCodexProfile updates the default Codex profile.
func (s *Server) UpdateCodexProfile(profile string) error {
	s.settings.Codex.DefaultProfile = profile
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateCodexSandbox updates the default Codex sandbox mode.
func (s *Server) UpdateCodexSandbox(mode string) error {
	s.settings.Codex.DefaultSandbox = mode
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateCodexApprovalPolicy updates the default Codex approval policy.
func (s *Server) UpdateCodexApprovalPolicy(policy string) error {
	s.settings.Codex.DefaultApprovalPolicy = policy
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateCodexSearch updates Codex search toggle.
func (s *Server) UpdateCodexSearch(enabled bool) error {
	s.settings.Codex.EnableSearch = enabled
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// GetCodexConfig builds a CodexConfig from current settings.
func (s *Server) GetCodexConfig() types.CodexConfig {
	return types.CodexConfig{
		Model:           s.settings.Codex.DefaultModel,
		Profile:         s.settings.Codex.DefaultProfile,
		SandboxMode:     types.CodexSandboxMode(s.settings.Codex.DefaultSandbox),
		ApprovalPolicy:  types.CodexApprovalPolicy(s.settings.Codex.DefaultApprovalPolicy),
		Search:          s.settings.Codex.EnableSearch,
		FullAuto:        s.settings.Codex.FullAuto,
		BypassApprovals: s.settings.Codex.BypassApprovals,
		WorkingDir:      s.settings.Codex.DefaultWorkingDir,
		SystemPrompt:    s.settings.Codex.DefaultSystemPrompt,
		AddDirs:         append([]string{}, s.settings.Codex.DefaultAddDirs...),
		ConfigOverrides: append([]string{}, s.settings.Codex.ConfigOverrides...),
		EnableFeatures:  append([]string{}, s.settings.Codex.EnableFeatures...),
		DisableFeatures: append([]string{}, s.settings.Codex.DisableFeatures...),
		IncludeHistory:  s.settings.Codex.IncludeHistory,
	}
}

// GeminiSettings returns the current Gemini configuration.
func (s *Server) GeminiSettings() types.GeminiSettings {
	return s.settings.Gemini
}

// UpdateGeminiSettings updates Gemini configuration and persists it.
func (s *Server) UpdateGeminiSettings(settings types.GeminiSettings) error {
	s.settings.Gemini = settings
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateGeminiModel updates the default Gemini model.
func (s *Server) UpdateGeminiModel(model string) error {
	s.settings.Gemini.DefaultModel = model
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateGeminiSandbox updates the default Gemini sandbox mode.
func (s *Server) UpdateGeminiSandbox(enabled bool) error {
	s.settings.Gemini.DefaultSandbox = enabled
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateGeminiApprovalMode updates the default Gemini approval mode.
func (s *Server) UpdateGeminiApprovalMode(mode string) error {
	s.settings.Gemini.DefaultApprovalMode = mode
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateGeminiResume updates the Gemini session to resume.
func (s *Server) UpdateGeminiResume(sessionID string) error {
	s.settings.Gemini.ResumeSession = sessionID
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// GetGeminiConfig builds a GeminiConfig from current settings.
func (s *Server) GetGeminiConfig() types.GeminiConfig {
	return types.GeminiConfig{
		Model:        types.GeminiModel(s.settings.Gemini.DefaultModel),
		Sandbox:      s.settings.Gemini.DefaultSandbox,
		ApprovalMode: s.settings.Gemini.DefaultApprovalMode,
		AllowedTools: s.settings.Gemini.CustomAllowedTools,
		Resume:       s.settings.Gemini.ResumeSession,
	}
}

// VibeSettings returns the current Vibe configuration
func (s *Server) VibeSettings() types.VibeSettings {
	return s.settings.Vibe
}

// UpdateVibeSettings updates Vibe configuration and persists it
func (s *Server) UpdateVibeSettings(settings types.VibeSettings) error {
	s.settings.Vibe = settings
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateVibeAgent updates the default Vibe agent configuration
func (s *Server) UpdateVibeAgent(agent string) error {
	s.settings.Vibe.DefaultAgent = agent
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateVibeNonInteractive updates the non-interactive mode toggle
func (s *Server) UpdateVibeNonInteractive(enabled bool) error {
	s.settings.Vibe.NonInteractive = enabled
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateVibeAutoApprove updates the auto-approve toggle
func (s *Server) UpdateVibeAutoApprove(enabled bool) error {
	s.settings.Vibe.AutoApprove = enabled
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateVibeIncludeHistory updates the include history toggle
func (s *Server) UpdateVibeIncludeHistory(enabled bool) error {
	s.settings.Vibe.IncludeHistory = enabled
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// UpdateVibeSystemPrompt updates the default system prompt
func (s *Server) UpdateVibeSystemPrompt(prompt string) error {
	s.settings.Vibe.DefaultSystemPrompt = prompt
	s.applySettingsToAgents()
	return s.SaveSettings()
}

// GetVibeConfig builds a VibeConfig from current settings
func (s *Server) GetVibeConfig() types.VibeConfig {
	return types.VibeConfig{
		Agent:          s.settings.Vibe.DefaultAgent,
		NonInteractive: s.settings.Vibe.NonInteractive,
		AutoApprove:    s.settings.Vibe.AutoApprove,
		IncludeHistory: s.settings.Vibe.IncludeHistory,
		SystemPrompt:   s.settings.Vibe.DefaultSystemPrompt,
	}
}