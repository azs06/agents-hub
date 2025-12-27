package a2a

import (
	"time"

	"a2a-go/internal/types"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
)

// ToSDKMessage converts internal Message to A2A SDK Message
func ToSDKMessage(msg types.Message) *sdka2a.Message {
	sdkMsg := &sdka2a.Message{
		ID:        msg.MessageID,
		Role:      sdka2a.MessageRole(msg.Role),
		Parts:     ToSDKParts(msg.Parts),
		TaskID:    sdka2a.TaskID(msg.TaskID),
		ContextID: msg.ContextID,
		Metadata:  msg.Metadata,
	}
	return sdkMsg
}

// FromSDKMessage converts A2A SDK Message to internal Message
func FromSDKMessage(msg *sdka2a.Message) types.Message {
	if msg == nil {
		return types.Message{}
	}
	return types.Message{
		Kind:      "message",
		MessageID: msg.ID,
		Role:      string(msg.Role),
		Parts:     FromSDKParts(msg.Parts),
		TaskID:    string(msg.TaskID),
		ContextID: msg.ContextID,
		Metadata:  msg.Metadata,
	}
}

// ToSDKParts converts internal Parts to A2A SDK ContentParts
func ToSDKParts(parts []types.Part) sdka2a.ContentParts {
	if len(parts) == 0 {
		return nil
	}
	result := make(sdka2a.ContentParts, 0, len(parts))
	for _, p := range parts {
		switch p.Kind {
		case "text":
			result = append(result, &sdka2a.TextPart{
				Text: p.Text,
			})
		case "file":
			if p.File != nil {
				var fileContent sdka2a.FilePartContent
				if p.File.Bytes != "" {
					fileContent = &sdka2a.FileBytes{
						FileMeta: sdka2a.FileMeta{
							Name:     p.File.Name,
							MimeType: p.File.MimeType,
						},
						Bytes: p.File.Bytes,
					}
				} else if p.File.URI != "" {
					fileContent = &sdka2a.FileURI{
						FileMeta: sdka2a.FileMeta{
							Name:     p.File.Name,
							MimeType: p.File.MimeType,
						},
						URI: p.File.URI,
					}
				}
				if fileContent != nil {
					result = append(result, &sdka2a.FilePart{
						File: fileContent,
					})
				}
			}
		case "data":
			if dataMap, ok := p.Data.(map[string]any); ok {
				result = append(result, &sdka2a.DataPart{
					Data: dataMap,
				})
			}
		}
	}
	return result
}

// FromSDKParts converts A2A SDK ContentParts to internal Parts
func FromSDKParts(parts sdka2a.ContentParts) []types.Part {
	if len(parts) == 0 {
		return nil
	}
	result := make([]types.Part, 0, len(parts))
	for _, p := range parts {
		switch pt := p.(type) {
		case *sdka2a.TextPart:
			result = append(result, types.Part{
				Kind: "text",
				Text: pt.Text,
			})
		case sdka2a.TextPart:
			result = append(result, types.Part{
				Kind: "text",
				Text: pt.Text,
			})
		case *sdka2a.FilePart:
			file := &types.File{}
			if pt.File != nil {
				switch fc := pt.File.(type) {
				case *sdka2a.FileBytes:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.Bytes = fc.Bytes
				case sdka2a.FileBytes:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.Bytes = fc.Bytes
				case *sdka2a.FileURI:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.URI = fc.URI
				case sdka2a.FileURI:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.URI = fc.URI
				}
			}
			result = append(result, types.Part{
				Kind: "file",
				File: file,
			})
		case sdka2a.FilePart:
			file := &types.File{}
			if pt.File != nil {
				switch fc := pt.File.(type) {
				case *sdka2a.FileBytes:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.Bytes = fc.Bytes
				case sdka2a.FileBytes:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.Bytes = fc.Bytes
				case *sdka2a.FileURI:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.URI = fc.URI
				case sdka2a.FileURI:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.URI = fc.URI
				}
			}
			result = append(result, types.Part{
				Kind: "file",
				File: file,
			})
		case *sdka2a.DataPart:
			result = append(result, types.Part{
				Kind: "data",
				Data: pt.Data,
			})
		case sdka2a.DataPart:
			result = append(result, types.Part{
				Kind: "data",
				Data: pt.Data,
			})
		}
	}
	return result
}

// ToSDKTask converts internal Task to A2A SDK Task
func ToSDKTask(task types.Task) *sdka2a.Task {
	sdkTask := &sdka2a.Task{
		ID:        sdka2a.TaskID(task.ID),
		ContextID: task.ContextID,
		Status: sdka2a.TaskStatus{
			State:     toSDKTaskState(task.Status.State),
			Timestamp: parseTimestamp(task.Status.Timestamp),
		},
		Metadata: task.Metadata,
	}

	// Convert history
	if len(task.History) > 0 {
		sdkTask.History = make([]*sdka2a.Message, len(task.History))
		for i, msg := range task.History {
			sdkTask.History[i] = ToSDKMessage(msg)
		}
	}

	// Convert artifacts
	if len(task.Artifacts) > 0 {
		sdkTask.Artifacts = make([]*sdka2a.Artifact, len(task.Artifacts))
		for i, art := range task.Artifacts {
			sdkTask.Artifacts[i] = ToSDKArtifact(art)
		}
	}

	// Convert status message if present
	if task.Status.Message != nil {
		sdkTask.Status.Message = ToSDKMessage(*task.Status.Message)
	}

	return sdkTask
}

// FromSDKTask converts A2A SDK Task to internal Task
func FromSDKTask(task *sdka2a.Task) types.Task {
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

	// Convert history
	if len(task.History) > 0 {
		result.History = make([]types.Message, len(task.History))
		for i, msg := range task.History {
			result.History[i] = FromSDKMessage(msg)
		}
	}

	// Convert artifacts
	if len(task.Artifacts) > 0 {
		result.Artifacts = make([]types.Artifact, len(task.Artifacts))
		for i, art := range task.Artifacts {
			result.Artifacts[i] = FromSDKArtifact(art)
		}
	}

	// Convert status message if present
	if task.Status.Message != nil {
		msg := FromSDKMessage(task.Status.Message)
		result.Status.Message = &msg
	}

	return result
}

// ToSDKArtifact converts internal Artifact to A2A SDK Artifact
func ToSDKArtifact(art types.Artifact) *sdka2a.Artifact {
	return &sdka2a.Artifact{
		ID:          sdka2a.ArtifactID(art.ArtifactID),
		Name:        art.Name,
		Description: art.Description,
		Parts:       ToSDKParts(art.Parts),
		Metadata:    art.Metadata,
	}
}

// FromSDKArtifact converts A2A SDK Artifact to internal Artifact
func FromSDKArtifact(art *sdka2a.Artifact) types.Artifact {
	if art == nil {
		return types.Artifact{}
	}
	return types.Artifact{
		ArtifactID:  string(art.ID),
		Name:        art.Name,
		Description: art.Description,
		Parts:       FromSDKParts(art.Parts),
		Metadata:    art.Metadata,
	}
}

// ToSDKAgentCard converts internal AgentCard to A2A SDK AgentCard
func ToSDKAgentCard(card types.AgentCard) *sdka2a.AgentCard {
	sdkCard := &sdka2a.AgentCard{
		Name:            card.Name,
		Description:     card.Description,
		URL:             card.URL,
		Version:         card.Version,
		ProtocolVersion: card.ProtocolVersion,
		Provider: &sdka2a.AgentProvider{
			Org: card.Provider.Name,
			URL: card.Provider.URL,
		},
		Capabilities: sdka2a.AgentCapabilities{
			Streaming:              card.Capabilities.Streaming,
			PushNotifications:      card.Capabilities.PushNotifications,
			StateTransitionHistory: card.Capabilities.StateTransitionHistory,
		},
	}

	// Convert skills
	if len(card.Skills) > 0 {
		sdkCard.Skills = make([]sdka2a.AgentSkill, len(card.Skills))
		for i, skill := range card.Skills {
			sdkCard.Skills[i] = sdka2a.AgentSkill{
				ID:          skill.ID,
				Name:        skill.Name,
				Description: skill.Description,
				Tags:        skill.Tags,
				InputModes:  skill.InputModes,
				OutputModes: skill.OutputModes,
			}
		}
	}

	return sdkCard
}

// FromSDKAgentCard converts A2A SDK AgentCard to internal AgentCard
func FromSDKAgentCard(card *sdka2a.AgentCard) types.AgentCard {
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

	// Convert skills
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

// toSDKTaskState converts internal TaskState to A2A SDK TaskState
func toSDKTaskState(state types.TaskState) sdka2a.TaskState {
	switch state {
	case types.TaskStateSubmitted:
		return sdka2a.TaskStateSubmitted
	case types.TaskStateWorking:
		return sdka2a.TaskStateWorking
	case types.TaskStateInputRequired:
		return sdka2a.TaskStateInputRequired
	case types.TaskStateCompleted:
		return sdka2a.TaskStateCompleted
	case types.TaskStateCanceled:
		return sdka2a.TaskStateCanceled
	case types.TaskStateFailed:
		return sdka2a.TaskStateFailed
	case types.TaskStateRejected:
		return sdka2a.TaskStateRejected
	case types.TaskStateAuthRequired:
		return sdka2a.TaskStateAuthRequired
	default:
		return sdka2a.TaskStateUnspecified
	}
}

// fromSDKTaskState converts A2A SDK TaskState to internal TaskState
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

// parseTimestamp converts RFC3339 string to *time.Time
func parseTimestamp(ts string) *time.Time {
	if ts == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return nil
	}
	return &t
}

// formatTimestamp converts *time.Time to RFC3339 string
func formatTimestamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
