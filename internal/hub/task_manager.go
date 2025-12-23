package hub

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"a2a-go/internal/types"
	"a2a-go/internal/utils"
)

type TaskManager struct {
	mu          sync.RWMutex
	tasks       map[string]*types.Task
	persistPath string
	persistMu   sync.Mutex
}

func NewTaskManager() *TaskManager {
	return &TaskManager{tasks: make(map[string]*types.Task)}
}

func (tm *TaskManager) SetPersistence(path string) {
	tm.persistPath = path
}

func (tm *TaskManager) Create(task *types.Task) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tasks[task.ID] = task
	tm.persistLocked()
}

func (tm *TaskManager) Get(id string) (*types.Task, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	task, ok := tm.tasks[id]
	return task, ok
}

func (tm *TaskManager) UpdateStatus(id string, state types.TaskState, msg *types.Message) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	task, ok := tm.tasks[id]
	if !ok {
		return errors.New("task not found")
	}
	task.Status.State = state
	task.Status.Message = msg
	task.Status.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	tm.persistLocked()
	return nil
}

func (tm *TaskManager) List(contextID string, state types.TaskState, limit, offset int) []types.Task {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]types.Task, 0)
	for _, task := range tm.tasks {
		if contextID != "" && task.ContextID != contextID {
			continue
		}
		if state != "" && task.Status.State != state {
			continue
		}
		result = append(result, *task)
	}
	if offset >= len(result) {
		return []types.Task{}
	}
	end := len(result)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return result[offset:end]
}

func (tm *TaskManager) Load() error {
	if tm.persistPath == "" {
		return nil
	}
	data, err := os.ReadFile(tm.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var stored []*types.Task
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for _, task := range stored {
		tm.tasks[task.ID] = task
	}
	return nil
}

func (tm *TaskManager) persistLocked() {
	if tm.persistPath == "" {
		return
	}
	tm.persistMu.Lock()
	defer tm.persistMu.Unlock()
	snapshot := make([]*types.Task, 0, len(tm.tasks))
	for _, task := range tm.tasks {
		snapshot = append(snapshot, task)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return
	}
	_ = utils.WriteFileAtomic(tm.persistPath, data, 0o644)
}
