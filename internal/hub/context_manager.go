package hub

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"a2a-go/internal/types"
	"a2a-go/internal/utils"
)

type Context struct {
	ID        string           `json:"id"`
	CreatedAt time.Time        `json:"createdAt"`
	History   []types.Message  `json:"history,omitempty"`
}

type ContextManager struct {
	mu          sync.RWMutex
	contexts    map[string]Context
	persistPath string
	persistMu   sync.Mutex
}

func NewContextManager() *ContextManager {
	return &ContextManager{contexts: make(map[string]Context)}
}

func (cm *ContextManager) SetPersistence(path string) {
	cm.persistPath = path
}

func (cm *ContextManager) Get(id string) (Context, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	ctx, ok := cm.contexts[id]
	return ctx, ok
}

func (cm *ContextManager) Create(id string) Context {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	ctx := Context{ID: id, CreatedAt: time.Now().UTC()}
	cm.contexts[id] = ctx
	cm.persistLocked()
	return ctx
}

func (cm *ContextManager) List(limit int) []Context {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]Context, 0, len(cm.contexts))
	for _, ctx := range cm.contexts {
		result = append(result, ctx)
	}
	if limit > 0 && limit < len(result) {
		return result[:limit]
	}
	return result
}

// AddMessage appends a message to the context history
func (cm *ContextManager) AddMessage(contextID string, msg types.Message) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ctx, ok := cm.contexts[contextID]
	if !ok {
		// Create context if it doesn't exist
		ctx = Context{
			ID:        contextID,
			CreatedAt: time.Now().UTC(),
			History:   []types.Message{},
		}
	}

	ctx.History = append(ctx.History, msg)
	cm.contexts[contextID] = ctx
	cm.persistLocked()
	return nil
}

// GetHistory returns the full history for a context
func (cm *ContextManager) GetHistory(contextID string) []types.Message {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	ctx, ok := cm.contexts[contextID]
	if !ok {
		return nil
	}
	return ctx.History
}

// GetHistoryWithLimit returns history with a maximum number of messages
func (cm *ContextManager) GetHistoryWithLimit(contextID string, limit int) []types.Message {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	ctx, ok := cm.contexts[contextID]
	if !ok {
		return nil
	}

	if limit <= 0 || limit >= len(ctx.History) {
		return ctx.History
	}

	// Return the most recent messages
	return ctx.History[len(ctx.History)-limit:]
}

func (cm *ContextManager) Load() error {
	if cm.persistPath == "" {
		return nil
	}
	data, err := os.ReadFile(cm.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var stored []Context
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, ctx := range stored {
		cm.contexts[ctx.ID] = ctx
	}
	return nil
}

func (cm *ContextManager) persistLocked() {
	if cm.persistPath == "" {
		return
	}
	cm.persistMu.Lock()
	defer cm.persistMu.Unlock()
	snapshot := make([]Context, 0, len(cm.contexts))
	for _, ctx := range cm.contexts {
		snapshot = append(snapshot, ctx)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return
	}
	_ = utils.WriteFileAtomic(cm.persistPath, data, 0o644)
}
