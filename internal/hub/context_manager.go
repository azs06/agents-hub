package hub

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"a2a-go/internal/utils"
)

type Context struct {
	ID        string
	CreatedAt time.Time
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
