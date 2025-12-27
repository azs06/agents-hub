package a2a

import (
	"context"
	"fmt"
	"time"

	"agents-hub/internal/hub"
	"agents-hub/internal/types"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
)

// HubExecutor wraps the hub server to implement AgentExecutor
type HubExecutor struct {
	server *hub.Server
}

// NewHubExecutor creates a new HubExecutor
func NewHubExecutor(server *hub.Server) *HubExecutor {
	return &HubExecutor{
		server: server,
	}
}

// Execute implements a2asrv.AgentExecutor
func (e *HubExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	// Extract target agent from message metadata
	targetAgent := e.extractTargetAgent(reqCtx)
	if targetAgent == "" {
		return e.writeFailure(ctx, reqCtx, queue, "metadata.targetAgent required")
	}

	// Find agent in registry
	agentInfo, ok := e.server.Registry().Get(targetAgent)
	if !ok {
		return e.writeFailure(ctx, reqCtx, queue, "agent not found: "+targetAgent)
	}

	// Write "submitted" status if this is a new task
	if reqCtx.StoredTask == nil {
		event := sdka2a.NewStatusUpdateEvent(reqCtx, sdka2a.TaskStateSubmitted, nil)
		if err := queue.Write(ctx, event); err != nil {
			return fmt.Errorf("failed to write state submitted: %w", err)
		}
	}

	// Write "working" status
	event := sdka2a.NewStatusUpdateEvent(reqCtx, sdka2a.TaskStateWorking, nil)
	if err := queue.Write(ctx, event); err != nil {
		return fmt.Errorf("failed to write state working: %w", err)
	}

	// Convert RequestContext to internal ExecutionContext
	execCtx := e.toExecutionContext(reqCtx)

	// Execute agent
	result, err := agentInfo.Agent.Execute(execCtx)
	if err != nil {
		return e.writeFailure(ctx, reqCtx, queue, err.Error())
	}

	// Write completion status with response message
	var responseMsg *sdka2a.Message
	if result.Task.Status.Message != nil {
		responseMsg = ToSDKMessage(*result.Task.Status.Message)
	}

	finalEvent := sdka2a.NewStatusUpdateEvent(reqCtx, sdka2a.TaskStateCompleted, responseMsg)
	finalEvent.Final = true
	if err := queue.Write(ctx, finalEvent); err != nil {
		return fmt.Errorf("failed to write state completed: %w", err)
	}

	return nil
}

// Cancel implements a2asrv.AgentExecutor
func (e *HubExecutor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	// Extract agent from stored task
	var targetAgent string
	if reqCtx.StoredTask != nil && reqCtx.StoredTask.Metadata != nil {
		if agent, ok := reqCtx.StoredTask.Metadata["targetAgent"].(string); ok {
			targetAgent = agent
		}
	}

	if targetAgent != "" {
		agentInfo, ok := e.server.Registry().Get(targetAgent)
		if ok {
			agentInfo.Agent.Cancel(string(reqCtx.TaskID))
		}
	}

	// Write canceled status
	event := sdka2a.NewStatusUpdateEvent(reqCtx, sdka2a.TaskStateCanceled, nil)
	event.Final = true
	if err := queue.Write(ctx, event); err != nil {
		return fmt.Errorf("failed to write state canceled: %w", err)
	}

	return nil
}

// extractTargetAgent extracts the target agent ID from the request
func (e *HubExecutor) extractTargetAgent(reqCtx *a2asrv.RequestContext) string {
	// Check message metadata
	if reqCtx.Message != nil && reqCtx.Message.Metadata != nil {
		if agent, ok := reqCtx.Message.Metadata["targetAgent"].(string); ok {
			return agent
		}
	}

	// Check request metadata
	if reqCtx.Metadata != nil {
		if agent, ok := reqCtx.Metadata["targetAgent"].(string); ok {
			return agent
		}
	}

	// Check stored task metadata
	if reqCtx.StoredTask != nil && reqCtx.StoredTask.Metadata != nil {
		if agent, ok := reqCtx.StoredTask.Metadata["targetAgent"].(string); ok {
			return agent
		}
	}

	return ""
}

// toExecutionContext converts A2A SDK RequestContext to internal ExecutionContext
func (e *HubExecutor) toExecutionContext(reqCtx *a2asrv.RequestContext) types.ExecutionContext {
	userMsg := FromSDKMessage(reqCtx.Message)

	// Get history from context manager
	var history []types.Message
	if reqCtx.ContextID != "" {
		history = e.server.Contexts().GetHistory(reqCtx.ContextID)
	}

	// Also include history from stored task if available
	if reqCtx.StoredTask != nil && len(reqCtx.StoredTask.History) > 0 {
		for _, msg := range reqCtx.StoredTask.History {
			history = append(history, FromSDKMessage(msg))
		}
	}

	return types.ExecutionContext{
		TaskID:          string(reqCtx.TaskID),
		ContextID:       reqCtx.ContextID,
		UserMessage:     userMsg,
		PreviousHistory: history,
		Timeout:         0, // Use agent default
	}
}

// writeFailure writes a failure event to the queue
func (e *HubExecutor) writeFailure(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue, errMsg string) error {
	errorMessage := sdka2a.NewMessage(sdka2a.MessageRoleAgent, &sdka2a.TextPart{Text: errMsg})
	errorMessage.ID = "error-" + string(reqCtx.TaskID)
	errorMessage.TaskID = reqCtx.TaskID
	errorMessage.ContextID = reqCtx.ContextID
	errorMessage.Metadata = map[string]any{
		"error":     true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	event := sdka2a.NewStatusUpdateEvent(reqCtx, sdka2a.TaskStateFailed, errorMessage)
	event.Final = true
	if err := queue.Write(ctx, event); err != nil {
		return fmt.Errorf("failed to write failure event: %w", err)
	}
	return nil
}
