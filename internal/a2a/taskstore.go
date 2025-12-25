package a2a

import (
	"context"

	"a2a-go/internal/hub"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
)

// TaskStoreAdapter wraps the internal TaskManager to implement a2asrv.TaskStore
type TaskStoreAdapter struct {
	manager *hub.TaskManager
}

// NewTaskStoreAdapter creates a new TaskStoreAdapter
func NewTaskStoreAdapter(manager *hub.TaskManager) *TaskStoreAdapter {
	return &TaskStoreAdapter{manager: manager}
}

// Save stores a task
func (s *TaskStoreAdapter) Save(ctx context.Context, task *sdka2a.Task) error {
	internalTask := FromSDKTask(task)
	s.manager.Create(&internalTask)
	return nil
}

// Get retrieves a task by ID
func (s *TaskStoreAdapter) Get(ctx context.Context, taskID sdka2a.TaskID) (*sdka2a.Task, error) {
	task, ok := s.manager.Get(string(taskID))
	if !ok {
		return nil, sdka2a.ErrTaskNotFound
	}
	sdkTask := ToSDKTask(*task)
	return sdkTask, nil
}
