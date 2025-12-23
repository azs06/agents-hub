package types

import "time"

type TaskState string

const (
	TaskStateSubmitted      TaskState = "submitted"
	TaskStateWorking        TaskState = "working"
	TaskStateInputRequired  TaskState = "input-required"
	TaskStateCompleted      TaskState = "completed"
	TaskStateCanceled       TaskState = "canceled"
	TaskStateFailed         TaskState = "failed"
	TaskStateRejected       TaskState = "rejected"
	TaskStateAuthRequired   TaskState = "auth-required"
	TaskStateUnknown        TaskState = "unknown"
)

type Message struct {
	Kind      string                 `json:"kind"`
	MessageID string                 `json:"messageId"`
	Role      string                 `json:"role"`
	Parts     []Part                 `json:"parts"`
	TaskID    string                 `json:"taskId,omitempty"`
	ContextID string                 `json:"contextId,omitempty"`
	Metadata  map[string]any         `json:"metadata,omitempty"`
}

type Part struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
	File *File  `json:"file,omitempty"`
	Data any    `json:"data,omitempty"`
}

type File struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Bytes    string `json:"bytes,omitempty"`
	URI      string `json:"uri,omitempty"`
}

type Task struct {
	Kind      string                 `json:"kind"`
	ID        string                 `json:"id"`
	ContextID string                 `json:"contextId"`
	Status    TaskStatus             `json:"status"`
	History   []Message              `json:"history,omitempty"`
	Artifacts []Artifact             `json:"artifacts,omitempty"`
	Metadata  map[string]any         `json:"metadata,omitempty"`
}

type TaskStatus struct {
	State     TaskState `json:"state"`
	Message   *Message  `json:"message,omitempty"`
	Timestamp string    `json:"timestamp"`
}

type Artifact struct {
	ArtifactID  string         `json:"artifactId"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parts       []Part         `json:"parts"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type AgentCard struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Name            string             `json:"name"`
	Description     string             `json:"description"`
	URL             string             `json:"url"`
	Version         string             `json:"version"`
	Provider        Provider           `json:"provider"`
	Skills          []Skill            `json:"skills"`
	Capabilities    AgentCapabilities  `json:"capabilities"`
	SecuritySchemes map[string]any     `json:"securitySchemes,omitempty"`
}

type Provider struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
}

type AgentCapabilities struct {
	Streaming             bool `json:"streaming"`
	PushNotifications     bool `json:"pushNotifications"`
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

type AgentHealth struct {
	Status       string    `json:"status"`
	LastCheck    time.Time `json:"lastCheck"`
	LatencyMs    int64     `json:"latencyMs,omitempty"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
}

type ExecutionContext struct {
	TaskID          string
	ContextID       string
	UserMessage     Message
	PreviousHistory []Message
	WorkingDir      string
	Timeout         time.Duration
}

type ExecutionResult struct {
	Task       Task
	Artifacts  []Artifact
	FinalState TaskState
}

type RuntimeCapabilities struct {
	SupportsStreaming     bool
	SupportsCancellation  bool
	MaxConcurrentTasks    int
	SupportedInputModes   []string
	SupportedOutputModes  []string
}
