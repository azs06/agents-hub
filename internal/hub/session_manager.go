package hub

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"agents-hub/internal/utils"
)

// SessionManager handles session persistence and retrieval
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	dataDir  string
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// SetDataDir sets the directory for session storage
func (sm *SessionManager) SetDataDir(dir string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.dataDir = filepath.Join(dir, "sessions")
}

// Load loads all sessions from the sessions directory
func (sm *SessionManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.dataDir == "" {
		return nil
	}

	// Create sessions directory if it doesn't exist
	if err := os.MkdirAll(sm.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	entries, err := os.ReadDir(sm.dataDir)
	if err != nil {
		return fmt.Errorf("failed to read sessions directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(sm.dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip files that can't be read
		}

		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue // Skip invalid JSON
		}

		sm.sessions[session.ID] = &session
	}

	return nil
}

// Create creates a new session with a unique ID
func (sm *SessionManager) Create() (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id, err := generateUUID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	now := time.Now().UTC()
	session := &Session{
		ID:        id,
		ContextID: utils.NewID("ctx"), // generate linked hub context for agent history sharing
		CreatedAt: now,
		UpdatedAt: now,
		Entries:   []SessionEntry{},
	}

	sm.sessions[id] = session

	if err := sm.persistSession(session); err != nil {
		return nil, err
	}

	return session, nil
}

// Get retrieves a session by ID
func (sm *SessionManager) Get(id string) *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[id]
}

// List returns all sessions sorted by UpdatedAt descending
func (sm *SessionManager) List() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		sessions = append(sessions, s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions
}

// AddEntry adds an entry to a session and persists it
func (sm *SessionManager) AddEntry(sessionID string, entry SessionEntry) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Entries = append(session.Entries, entry)
	session.UpdatedAt = time.Now().UTC()

	return sm.persistSession(session)
}

// Delete removes a session
func (sm *SessionManager) Delete(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.sessions[id]; !ok {
		return fmt.Errorf("session not found: %s", id)
	}

	delete(sm.sessions, id)

	if sm.dataDir != "" {
		path := filepath.Join(sm.dataDir, id+".json")
		os.Remove(path) // Ignore error if file doesn't exist
	}

	return nil
}

// persistSession saves a session to disk
func (sm *SessionManager) persistSession(session *Session) error {
	if sm.dataDir == "" {
		return nil
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	path := filepath.Join(sm.dataDir, session.ID+".json")
	return utils.WriteFileAtomic(path, data, 0644)
}

// generateUUID generates a UUID v4
func generateUUID() (string, error) {
	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		return "", err
	}

	// Set version (4) and variant (RFC 4122)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}
