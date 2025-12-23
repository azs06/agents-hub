package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"

	"a2a-go/internal/types"
)

type agentItem struct {
	data agentData
}

func (i agentItem) Title() string { return i.data.ID }
func (i agentItem) Description() string {
	return fmt.Sprintf("%s - %s", i.data.Name, i.data.Health.Status)
}
func (i agentItem) FilterValue() string { return i.data.ID + " " + i.data.Name }

type taskItem struct {
	data types.Task
}

func (i taskItem) Title() string { return i.data.ID }
func (i taskItem) Description() string {
	return fmt.Sprintf("%s - %s", i.data.Status.State, i.data.ContextID)
}
func (i taskItem) FilterValue() string { return i.data.ID + " " + i.data.ContextID }

type responseEntry struct {
	TaskID    string
	Agent     string
	Text      string
	Timestamp string
}

type responseItem struct {
	data responseEntry
}

func (i responseItem) Title() string {
	return fmt.Sprintf("%s - %s", i.data.Agent, i.data.TaskID)
}
func (i responseItem) Description() string {
	return previewText(i.data.Text, 80)
}
func (i responseItem) FilterValue() string { return i.data.Agent + " " + i.data.TaskID }

func buildAgentItems(in []agentData) []list.Item {
	items := make([]list.Item, 0, len(in))
	for _, agent := range in {
		items = append(items, agentItem{data: agent})
	}
	return items
}

func buildTaskItems(in []types.Task) []list.Item {
	items := make([]list.Item, 0, len(in))
	for _, task := range in {
		items = append(items, taskItem{data: task})
	}
	return items
}

func buildResponseItems(in []responseEntry) []list.Item {
	items := make([]list.Item, 0, len(in))
	for _, entry := range in {
		items = append(items, responseItem{data: entry})
	}
	return items
}

func renderAgentDetail(agent agentData) string {
	lastCheck := "unknown"
	if !agent.Health.LastCheck.IsZero() {
		lastCheck = agent.Health.LastCheck.Format(time.RFC822)
	}
	lines := []string{
		fmt.Sprintf("ID: %s", agent.ID),
		fmt.Sprintf("Name: %s", agent.Name),
		fmt.Sprintf("Health: %s", agent.Health.Status),
		fmt.Sprintf("Last check: %s", lastCheck),
		"",
		fmt.Sprintf("Provider: %s", agent.Card.Provider.Name),
		fmt.Sprintf("Version: %s", agent.Card.Version),
		fmt.Sprintf("URL: %s", agent.Card.URL),
	}
	return strings.Join(lines, "\n")
}

func renderTaskDetail(task types.Task) string {
	lines := []string{
		fmt.Sprintf("ID: %s", task.ID),
		fmt.Sprintf("State: %s", task.Status.State),
		fmt.Sprintf("Context: %s", task.ContextID),
		fmt.Sprintf("Timestamp: %s", task.Status.Timestamp),
		"",
		"Response:",
		extractTaskText(task),
	}
	return strings.Join(lines, "\n")
}

func renderResponseDetail(entry responseEntry) string {
	lines := []string{
		fmt.Sprintf("Task: %s", entry.TaskID),
		fmt.Sprintf("Agent: %s", entry.Agent),
		fmt.Sprintf("Timestamp: %s", entry.Timestamp),
		"",
		entry.Text,
	}
	return strings.Join(lines, "\n")
}

func previewText(text string, limit int) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}
