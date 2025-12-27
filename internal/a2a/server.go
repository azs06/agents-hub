package a2a

import (
	"encoding/json"
	"net/http"
	"strings"

	"agents-hub/internal/hub"
	"agents-hub/internal/types"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
)

// A2AServer wraps the A2A protocol handler for HTTP
type A2AServer struct {
	handler a2asrv.RequestHandler
	server  *hub.Server
	baseURL string
}

// NewA2AServer creates a new A2A server
func NewA2AServer(server *hub.Server, baseURL string) (*A2AServer, error) {
	executor := NewHubExecutor(server)
	taskStore := NewTaskStoreAdapter(server.Tasks())

	handler := a2asrv.NewHandler(
		executor,
		a2asrv.WithTaskStore(taskStore),
	)

	return &A2AServer{
		handler: handler,
		server:  server,
		baseURL: baseURL,
	}, nil
}

// RegisterRoutes adds A2A protocol routes to the mux
// Note: /.well-known/agent.json, /.well-known/agents, /.well-known/agents/ are
// already registered by HTTPTransport to avoid route conflicts
func (s *A2AServer) RegisterRoutes(mux *http.ServeMux) {
	// JSON-RPC endpoint for A2A protocol
	mux.Handle("/a2a", a2asrv.NewJSONRPCHandler(s.handler))
}

// handleAgentCard returns the hub's agent card
func (s *A2AServer) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	card := s.buildHubAgentCard()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(card)
}

// handleAgentsList returns all registered agent cards
func (s *A2AServer) handleAgentsList(w http.ResponseWriter, r *http.Request) {
	agents := s.server.Registry().List()
	cards := make([]*sdka2a.AgentCard, 0, len(agents))

	for _, info := range agents {
		card, err := info.Agent.GetCard()
		if err == nil {
			sdkCard := ToSDKAgentCard(card)
			cards = append(cards, sdkCard)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cards)
}

// handleAgentByID returns a specific agent's card
func (s *A2AServer) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path: /.well-known/agents/{agentId}
	agentID := r.URL.Path[len("/.well-known/agents/"):]
	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	info, ok := s.server.Registry().Get(agentID)
	if !ok {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	card, err := info.Agent.GetCard()
	if err != nil {
		http.Error(w, "failed to get agent card", http.StatusInternalServerError)
		return
	}

	sdkCard := ToSDKAgentCard(card)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sdkCard)
}

// buildHubAgentCard creates the hub's agent card
func (s *A2AServer) buildHubAgentCard() *sdka2a.AgentCard {
	a2aURL := strings.TrimRight(s.baseURL, "/") + "/a2a"
	// Get all registered agents as skills
	agents := s.server.Registry().List()
	skills := make([]sdka2a.AgentSkill, 0, len(agents))

	for _, info := range agents {
		card, err := info.Agent.GetCard()
		if err == nil {
			skills = append(skills, sdka2a.AgentSkill{
				ID:          card.Name,
				Name:        card.Name,
				Description: card.Description,
				Tags:        []string{"agent"},
				InputModes:  []string{"text/plain"},
				OutputModes: []string{"text/plain"},
			})
		}
	}

	return &sdka2a.AgentCard{
		Name:            "Agents Hub",
		Description:     "Multi-agent orchestration hub supporting A2A protocol",
		URL:             a2aURL,
		Version:         "1.0.0",
		ProtocolVersion: "1.0",
		Provider: &sdka2a.AgentProvider{
			Org: "Local",
			URL: s.baseURL,
		},
		PreferredTransport: sdka2a.TransportProtocolJSONRPC,
		AdditionalInterfaces: []sdka2a.AgentInterface{
			{URL: a2aURL, Transport: sdka2a.TransportProtocolJSONRPC},
		},
		Capabilities: sdka2a.AgentCapabilities{
			Streaming:              true,
			PushNotifications:      false,
			StateTransitionHistory: true,
		},
		Skills:             skills,
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
	}
}

// GetInternalCard returns the hub's card in internal format
func (s *A2AServer) GetInternalCard() types.AgentCard {
	sdkCard := s.buildHubAgentCard()
	return FromSDKAgentCard(sdkCard)
}
