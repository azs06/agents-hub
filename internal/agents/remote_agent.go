package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"agents-hub/internal/types"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
)

// RemoteAgent wraps an external A2A agent
type RemoteAgent struct {
	id       string
	name     string
	cardURL  string
	card     *sdka2a.AgentCard
	client   *a2aclient.Client
	alias    string
}

// NewRemoteAgent creates a remote agent from an A2A agent card URL
func NewRemoteAgent(ctx context.Context, cardURL string, alias string) (*RemoteAgent, error) {
	// Fetch the agent card
	card, err := fetchAgentCard(ctx, cardURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent card: %w", err)
	}

	// Create client from card
	client, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Generate ID from name or alias
	name := card.Name
	id := alias
	if id == "" {
		id = "remote-" + sanitizeID(name)
	}

	return &RemoteAgent{
		id:      id,
		name:    name,
		cardURL: cardURL,
		card:    card,
		client:  client,
		alias:   alias,
	}, nil
}

// ID returns the agent's unique identifier
func (a *RemoteAgent) ID() string {
	return a.id
}

// Name returns the agent's display name
func (a *RemoteAgent) Name() string {
	return a.name
}

// Initialize sets up the remote agent
func (a *RemoteAgent) Initialize() error {
	return nil
}

// Shutdown cleans up the remote agent's resources
func (a *RemoteAgent) Shutdown() error {
	if a.client != nil {
		return a.client.Destroy()
	}
	return nil
}

// GetCard returns the agent's card
func (a *RemoteAgent) GetCard() (types.AgentCard, error) {
	if a.card == nil {
		return types.AgentCard{}, fmt.Errorf("agent card not available")
	}
	return fromSDKAgentCard(a.card), nil
}

// GetCapabilities returns the agent's runtime capabilities
func (a *RemoteAgent) GetCapabilities() types.RuntimeCapabilities {
	caps := types.RuntimeCapabilities{
		SupportsStreaming:    false,
		SupportsCancellation: true,
		MaxConcurrentTasks:   10,
		SupportedInputModes:  []string{"text/plain"},
		SupportedOutputModes: []string{"text/plain"},
	}

	if a.card != nil {
		caps.SupportsStreaming = a.card.Capabilities.Streaming
		if len(a.card.DefaultInputModes) > 0 {
			caps.SupportedInputModes = a.card.DefaultInputModes
		}
		if len(a.card.DefaultOutputModes) > 0 {
			caps.SupportedOutputModes = a.card.DefaultOutputModes
		}
	}

	return caps
}

// CheckHealth checks if the remote agent is healthy
func (a *RemoteAgent) CheckHealth() (types.AgentHealth, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to get the agent card to verify connectivity
	_, err := a.client.GetAgentCard(ctx)
	if err != nil {
		return types.AgentHealth{
			Status:       "unhealthy",
			LastCheck:    time.Now().UTC(),
			ErrorMessage: err.Error(),
		}, nil
	}

	return types.AgentHealth{
		Status:    "healthy",
		LastCheck: time.Now().UTC(),
	}, nil
}

// Execute runs a task on the remote agent
func (a *RemoteAgent) Execute(ctx types.ExecutionContext) (types.ExecutionResult, error) {
	// Convert internal message to SDK message
	sdkMsg := toSDKMessage(ctx.UserMessage)
	sdkMsg.ContextID = ctx.ContextID
	sdkMsg.TaskID = sdka2a.TaskID(ctx.TaskID)

	// Include conversation history in metadata for remote agent
	if len(ctx.PreviousHistory) > 0 {
		historyJSON, err := json.Marshal(ctx.PreviousHistory)
		if err == nil {
			if sdkMsg.Metadata == nil {
				sdkMsg.Metadata = make(map[string]any)
			}
			sdkMsg.Metadata["conversationHistory"] = string(historyJSON)
		}
	}

	// Set up context with timeout
	execCtx := context.Background()
	if ctx.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(execCtx, ctx.Timeout)
		defer cancel()
	}

	// Send message to remote agent
	result, err := a.client.SendMessage(execCtx, &sdka2a.MessageSendParams{
		Message: sdkMsg,
	})
	if err != nil {
		return types.ExecutionResult{
			FinalState: types.TaskStateFailed,
			Task: types.Task{
				ID:        ctx.TaskID,
				ContextID: ctx.ContextID,
				Status: types.TaskStatus{
					State:     types.TaskStateFailed,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				},
			},
		}, fmt.Errorf("remote agent execution failed: %w", err)
	}

	// Handle result - could be Message or Task
	var task types.Task
	switch r := result.(type) {
	case *sdka2a.Message:
		// Create task from message response
		msg := fromSDKMessage(r)
		msg.Metadata = addAgentID(msg.Metadata, a.id)
		task = types.Task{
			Kind:      "task",
			ID:        ctx.TaskID,
			ContextID: ctx.ContextID,
			Status: types.TaskStatus{
				State:     types.TaskStateCompleted,
				Message:   &msg,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			},
			History: []types.Message{msg},
		}
	case *sdka2a.Task:
		// Convert task from SDK
		task = fromSDKTask(r)
		task.ID = ctx.TaskID
		task.ContextID = ctx.ContextID
		if task.Status.Message != nil {
			task.Status.Message.Metadata = addAgentID(task.Status.Message.Metadata, a.id)
		}
	default:
		return types.ExecutionResult{
			FinalState: types.TaskStateFailed,
		}, fmt.Errorf("unexpected result type from remote agent")
	}

	return types.ExecutionResult{
		Task:       task,
		Artifacts:  task.Artifacts,
		FinalState: task.Status.State,
	}, nil
}

// Cancel cancels a running task
func (a *RemoteAgent) Cancel(taskID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := a.client.CancelTask(ctx, &sdka2a.TaskIDParams{
		ID: sdka2a.TaskID(taskID),
	})
	return err == nil, err
}

// CardURL returns the URL where this agent's card was fetched from
func (a *RemoteAgent) CardURL() string {
	return a.cardURL
}

// Alias returns the local alias for this agent
func (a *RemoteAgent) Alias() string {
	return a.alias
}

// fetchAgentCard fetches an agent card from a URL
func fetchAgentCard(ctx context.Context, url string) (*sdka2a.AgentCard, error) {
	// Ensure URL ends with agent.json for well-known path
	if !strings.HasSuffix(url, ".json") && !strings.Contains(url, "/.well-known/") {
		if strings.HasSuffix(url, "/") {
			url += ".well-known/agent.json"
		} else {
			url += "/.well-known/agent.json"
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch agent card: status %d", resp.StatusCode)
	}

	var card sdka2a.AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("failed to decode agent card: %w", err)
	}

	return &card, nil
}

// sanitizeID converts a name to a valid ID
func sanitizeID(name string) string {
	// Convert to lowercase
	id := strings.ToLower(name)
	// Replace spaces and special chars with hyphens
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	id = reg.ReplaceAllString(id, "-")
	// Trim leading/trailing hyphens
	id = strings.Trim(id, "-")
	// Limit length
	if len(id) > 32 {
		id = id[:32]
	}
	return id
}

// addAgentID adds agent ID to metadata
func addAgentID(metadata map[string]any, agentID string) map[string]any {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["agentId"] = agentID
	metadata["remoteAgent"] = true
	return metadata
}

// Type conversion helpers (duplicated to avoid import cycle)

func toSDKMessage(msg types.Message) *sdka2a.Message {
	return &sdka2a.Message{
		ID:        msg.MessageID,
		Role:      sdka2a.MessageRole(msg.Role),
		Parts:     toSDKParts(msg.Parts),
		TaskID:    sdka2a.TaskID(msg.TaskID),
		ContextID: msg.ContextID,
		Metadata:  msg.Metadata,
	}
}

func fromSDKMessage(msg *sdka2a.Message) types.Message {
	if msg == nil {
		return types.Message{}
	}
	return types.Message{
		Kind:      "message",
		MessageID: msg.ID,
		Role:      string(msg.Role),
		Parts:     fromSDKParts(msg.Parts),
		TaskID:    string(msg.TaskID),
		ContextID: msg.ContextID,
		Metadata:  msg.Metadata,
	}
}

func toSDKParts(parts []types.Part) sdka2a.ContentParts {
	if len(parts) == 0 {
		return nil
	}
	result := make(sdka2a.ContentParts, 0, len(parts))
	for _, p := range parts {
		if p.Kind == "text" {
			result = append(result, &sdka2a.TextPart{Text: p.Text})
		}
	}
	return result
}

func fromSDKParts(parts sdka2a.ContentParts) []types.Part {
	if len(parts) == 0 {
		return nil
	}
	result := make([]types.Part, 0, len(parts))
	for _, p := range parts {
		if pt, ok := p.(*sdka2a.TextPart); ok {
			result = append(result, types.Part{Kind: "text", Text: pt.Text})
		}
	}
	return result
}

func fromSDKTask(task *sdka2a.Task) types.Task {
	if task == nil {
		return types.Task{}
	}
	result := types.Task{
		Kind:      "task",
		ID:        string(task.ID),
		ContextID: task.ContextID,
		Status: types.TaskStatus{
			State:     fromSDKTaskState(task.Status.State),
			Timestamp: formatTimestamp(task.Status.Timestamp),
		},
		Metadata: task.Metadata,
	}
	if len(task.History) > 0 {
		result.History = make([]types.Message, len(task.History))
		for i, msg := range task.History {
			result.History[i] = fromSDKMessage(msg)
		}
	}
	if task.Status.Message != nil {
		msg := fromSDKMessage(task.Status.Message)
		result.Status.Message = &msg
	}
	return result
}

func fromSDKAgentCard(card *sdka2a.AgentCard) types.AgentCard {
	if card == nil {
		return types.AgentCard{}
	}
	result := types.AgentCard{
		Name:            card.Name,
		Description:     card.Description,
		URL:             card.URL,
		Version:         card.Version,
		ProtocolVersion: card.ProtocolVersion,
		Capabilities: types.AgentCapabilities{
			Streaming:              card.Capabilities.Streaming,
			PushNotifications:      card.Capabilities.PushNotifications,
			StateTransitionHistory: card.Capabilities.StateTransitionHistory,
		},
	}
	if card.Provider != nil {
		result.Provider = types.Provider{
			Name: card.Provider.Org,
			URL:  card.Provider.URL,
		}
	}
	if len(card.Skills) > 0 {
		result.Skills = make([]types.Skill, len(card.Skills))
		for i, skill := range card.Skills {
			result.Skills[i] = types.Skill{
				ID:          skill.ID,
				Name:        skill.Name,
				Description: skill.Description,
				Tags:        skill.Tags,
				InputModes:  skill.InputModes,
				OutputModes: skill.OutputModes,
			}
		}
	}
	return result
}

func fromSDKTaskState(state sdka2a.TaskState) types.TaskState {
	switch state {
	case sdka2a.TaskStateSubmitted:
		return types.TaskStateSubmitted
	case sdka2a.TaskStateWorking:
		return types.TaskStateWorking
	case sdka2a.TaskStateInputRequired:
		return types.TaskStateInputRequired
	case sdka2a.TaskStateCompleted:
		return types.TaskStateCompleted
	case sdka2a.TaskStateCanceled:
		return types.TaskStateCanceled
	case sdka2a.TaskStateFailed:
		return types.TaskStateFailed
	case sdka2a.TaskStateRejected:
		return types.TaskStateRejected
	case sdka2a.TaskStateAuthRequired:
		return types.TaskStateAuthRequired
	default:
		return types.TaskStateUnknown
	}
}

func formatTimestamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
