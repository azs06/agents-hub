package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"a2a-go/internal/utils"
)

type Settings struct {
	OrchestratorAgents []string `json:"orchestratorAgents"`
	LastAgent          string   `json:"lastAgent"`
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
