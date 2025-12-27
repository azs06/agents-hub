package hub

import (
	"context"
	"fmt"
	"sync"

	"agents-hub/internal/agents"
)

// RemoteAgentRegistry manages external A2A agents
type RemoteAgentRegistry struct {
	mu           sync.RWMutex
	remoteAgents map[string]*agents.RemoteAgent
	mainRegistry *AgentRegistry
}

// NewRemoteAgentRegistry creates a new remote agent registry
func NewRemoteAgentRegistry(mainRegistry *AgentRegistry) *RemoteAgentRegistry {
	return &RemoteAgentRegistry{
		remoteAgents: make(map[string]*agents.RemoteAgent),
		mainRegistry: mainRegistry,
	}
}

// DiscoverAndRegister fetches an agent card and registers the remote agent
func (r *RemoteAgentRegistry) DiscoverAndRegister(ctx context.Context, cardURL string, alias string) error {
	agent, err := agents.NewRemoteAgent(ctx, cardURL, alias)
	if err != nil {
		return fmt.Errorf("failed to create remote agent: %w", err)
	}

	r.mu.Lock()
	// Check if already registered
	if existing, ok := r.remoteAgents[agent.ID()]; ok {
		existing.Shutdown()
	}
	r.remoteAgents[agent.ID()] = agent
	r.mu.Unlock()

	// Register in main registry so it's available for orchestration
	if err := r.mainRegistry.Register(agent); err != nil {
		r.mu.Lock()
		delete(r.remoteAgents, agent.ID())
		r.mu.Unlock()
		return fmt.Errorf("failed to register remote agent: %w", err)
	}

	return nil
}

// RemoveRemoteAgent unregisters a remote agent
func (r *RemoteAgentRegistry) RemoveRemoteAgent(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, ok := r.remoteAgents[id]
	if !ok {
		return fmt.Errorf("remote agent not found: %s", id)
	}

	agent.Shutdown()
	delete(r.remoteAgents, id)

	// Note: we don't remove from main registry as it doesn't support removal
	return nil
}

// Get returns a remote agent by ID
func (r *RemoteAgentRegistry) Get(id string) (*agents.RemoteAgent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.remoteAgents[id]
	return agent, ok
}

// List returns all remote agents
func (r *RemoteAgentRegistry) List() []*agents.RemoteAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*agents.RemoteAgent, 0, len(r.remoteAgents))
	for _, agent := range r.remoteAgents {
		result = append(result, agent)
	}
	return result
}

// RemoteAgentInfo contains information about a remote agent
type RemoteAgentInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	CardURL string `json:"cardUrl"`
	Alias   string `json:"alias"`
}

// ListInfo returns info about all remote agents
func (r *RemoteAgentRegistry) ListInfo() []RemoteAgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]RemoteAgentInfo, 0, len(r.remoteAgents))
	for _, agent := range r.remoteAgents {
		result = append(result, RemoteAgentInfo{
			ID:      agent.ID(),
			Name:    agent.Name(),
			CardURL: agent.CardURL(),
			Alias:   agent.Alias(),
		})
	}
	return result
}

// Shutdown shuts down all remote agents
func (r *RemoteAgentRegistry) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, agent := range r.remoteAgents {
		agent.Shutdown()
	}
	r.remoteAgents = make(map[string]*agents.RemoteAgent)
}
