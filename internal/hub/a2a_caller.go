package hub

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"agents-hub/internal/jsonrpc"
	"agents-hub/internal/types"

	sdka2a "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
)

type a2aMessageSendRequest struct {
	Message       types.Message `json:"message"`
	Configuration struct {
		HistoryLength int    `json:"historyLength"`
		TimeoutMs     int    `json:"timeout"`
		WorkingDir    string `json:"workingDirectory"`
	} `json:"configuration"`
}

// A2ARoutingCaller prefers A2A for message/send and falls back to local JSON-RPC when unavailable.
type A2ARoutingCaller struct {
	local  *LocalCaller
	client *a2aclient.Client
}

func NewA2ARoutingCaller(local *LocalCaller, baseURL string, httpEnabled bool) *A2ARoutingCaller {
	caller := &A2ARoutingCaller{local: local}
	if !httpEnabled || strings.TrimSpace(baseURL) == "" {
		return caller
	}
	a2aURL := strings.TrimRight(baseURL, "/") + "/a2a"
	client, err := a2aclient.NewFromEndpoints(context.Background(), []sdka2a.AgentInterface{
		{URL: a2aURL, Transport: sdka2a.TransportProtocolJSONRPC},
	})
	if err != nil {
		return caller
	}
	caller.client = client
	return caller
}

func (c *A2ARoutingCaller) Call(ctx context.Context, method string, params []byte) (jsonrpc.Response, error) {
	if method != "message/send" || c.client == nil {
		return c.local.Call(ctx, method, params)
	}

	var req a2aMessageSendRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return jsonrpc.Response{JSONRPC: "2.0", Error: &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "invalid params"}}, nil
	}
	if req.Message.Kind != "message" {
		return jsonrpc.Response{JSONRPC: "2.0", Error: &jsonrpc.RPCError{Code: jsonrpc.ErrInvalidParams, Message: "message required"}}, nil
	}

	if strings.TrimSpace(req.Configuration.WorkingDir) != "" {
		if req.Message.Metadata == nil {
			req.Message.Metadata = make(map[string]any)
		}
		if _, ok := req.Message.Metadata["workingDirectory"]; !ok {
			req.Message.Metadata["workingDirectory"] = req.Configuration.WorkingDir
		}
	}

	sdkMsg := toSDKMessage(req.Message)
	cfg := &sdka2a.MessageSendConfig{}
	if req.Configuration.HistoryLength > 0 {
		history := req.Configuration.HistoryLength
		cfg.HistoryLength = &history
	}
	paramsMsg := &sdka2a.MessageSendParams{Message: sdkMsg}
	if cfg.HistoryLength != nil {
		paramsMsg.Config = cfg
	}

	callCtx := ctx
	if req.Configuration.TimeoutMs > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(callCtx, time.Duration(req.Configuration.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	result, err := c.client.SendMessage(callCtx, paramsMsg)
	if err != nil {
		return jsonrpc.Response{JSONRPC: "2.0", Error: &jsonrpc.RPCError{Code: jsonrpc.ErrInternalError, Message: err.Error()}}, nil
	}

	var payload any
	switch resp := result.(type) {
	case *sdka2a.Task:
		payload = fromSDKTask(resp)
	case *sdka2a.Message:
		payload = fromSDKMessage(resp)
	default:
		payload = result
	}

	return jsonrpc.Response{JSONRPC: "2.0", Result: payload, ID: "internal"}, nil
}

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
		switch p.Kind {
		case "text":
			result = append(result, &sdka2a.TextPart{Text: p.Text})
		case "file":
			if p.File == nil {
				continue
			}
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
				result = append(result, &sdka2a.FilePart{File: fileContent})
			}
		case "data":
			if dataMap, ok := p.Data.(map[string]any); ok {
				result = append(result, &sdka2a.DataPart{Data: dataMap})
			}
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
		switch pt := p.(type) {
		case *sdka2a.TextPart:
			result = append(result, types.Part{Kind: "text", Text: pt.Text})
		case *sdka2a.FilePart:
			file := &types.File{}
			if pt.File != nil {
				switch fc := pt.File.(type) {
				case *sdka2a.FileBytes:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.Bytes = fc.Bytes
				case *sdka2a.FileURI:
					file.Name = fc.Name
					file.MimeType = fc.MimeType
					file.URI = fc.URI
				}
			}
			result = append(result, types.Part{Kind: "file", File: file})
		case *sdka2a.DataPart:
			result = append(result, types.Part{Kind: "data", Data: pt.Data})
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
	if len(task.Artifacts) > 0 {
		result.Artifacts = make([]types.Artifact, len(task.Artifacts))
		for i, art := range task.Artifacts {
			result.Artifacts[i] = fromSDKArtifact(art)
		}
	}
	if task.Status.Message != nil {
		msg := fromSDKMessage(task.Status.Message)
		result.Status.Message = &msg
	}
	return result
}

func fromSDKArtifact(art *sdka2a.Artifact) types.Artifact {
	if art == nil {
		return types.Artifact{}
	}
	return types.Artifact{
		ArtifactID:  string(art.ID),
		Name:        art.Name,
		Description: art.Description,
		Parts:       fromSDKParts(art.Parts),
		Metadata:    art.Metadata,
	}
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
