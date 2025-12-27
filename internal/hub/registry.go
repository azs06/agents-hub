package hub

import (
	"sync"
	"time"

	"agents-hub/internal/agents"
	"agents-hub/internal/types"
	"agents-hub/internal/utils"
)

type AgentInfo struct {
	Agent        agents.Agent
	Card         types.AgentCard
	Health       types.AgentHealth
	RegisteredAt time.Time
}

type AgentRegistry struct {
	mu      sync.RWMutex
	agents  map[string]*AgentInfo
	logger  *utils.Logger
	stopCh  chan struct{}
}

func NewAgentRegistry(logger *utils.Logger) *AgentRegistry {
	return &AgentRegistry{agents: make(map[string]*AgentInfo), logger: logger, stopCh: make(chan struct{})}
}

func (ar *AgentRegistry) Register(agent agents.Agent) error {
	card, err := agent.GetCard()
	if err != nil {
		return err
	}
	info := &AgentInfo{Agent: agent, Card: card, RegisteredAt: time.Now().UTC()}
	health, _ := agent.CheckHealth()
	info.Health = health
	ar.mu.Lock()
	ar.agents[agent.ID()] = info
	ar.mu.Unlock()
	return nil
}

func (ar *AgentRegistry) Get(id string) (*AgentInfo, bool) {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	info, ok := ar.agents[id]
	return info, ok
}

func (ar *AgentRegistry) List() []AgentInfo {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	result := make([]AgentInfo, 0, len(ar.agents))
	for _, info := range ar.agents {
		result = append(result, *info)
	}
	return result
}

func (ar *AgentRegistry) StartHealthChecks(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				ar.checkAll()
			case <-ar.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (ar *AgentRegistry) Stop() {
	close(ar.stopCh)
}

func (ar *AgentRegistry) checkAll() {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	for _, info := range ar.agents {
		health, err := info.Agent.CheckHealth()
		if err != nil {
			health.Status = "unhealthy"
			health.ErrorMessage = err.Error()
		}
		info.Health = health
	}
}
