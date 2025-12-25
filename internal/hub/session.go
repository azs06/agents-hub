package hub

import "time"

// Session represents a conversation session with a unique ID
type Session struct {
	ID        string         `json:"id"`
	ContextID string         `json:"contextId"` // linked hub context for agent history sharing
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Entries   []SessionEntry `json:"entries"`
}

// SessionEntry represents a single message in a session
type SessionEntry struct {
	Role      string `json:"role"`      // "user", "agent", "error", "user-input"
	Agent     string `json:"agent"`     // agent ID
	Text      string `json:"text"`      // message content
	Timestamp string `json:"timestamp"` // RFC3339 format
}

// ShortID returns the first 8 characters of the session ID for display
func (s *Session) ShortID() string {
	if len(s.ID) >= 8 {
		return s.ID[:8]
	}
	return s.ID
}
