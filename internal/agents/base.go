package agents

import (
	"agents-hub/internal/types"
)

type Agent interface {
	ID() string
	Name() string
	Initialize() error
	Shutdown() error
	GetCard() (types.AgentCard, error)
	GetCapabilities() types.RuntimeCapabilities
	CheckHealth() (types.AgentHealth, error)
	Execute(ctx types.ExecutionContext) (types.ExecutionResult, error)
	Cancel(taskID string) (bool, error)
}
