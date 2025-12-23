package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	NextTab key.Binding
	PrevTab key.Binding
	Up      key.Binding
	Down    key.Binding
	Refresh key.Binding
	Quit    key.Binding
	Help    key.Binding
	Command key.Binding
	Search  key.Binding
	Logs    key.Binding
	Send    key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Command, k.Search, k.Send, k.Refresh, k.Logs, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Command, k.Search, k.Send, k.Refresh, k.Logs, k.Help, k.Quit},
	}
}

var defaultKeyMap = keyMap{
	NextTab: key.NewBinding(
		key.WithKeys("tab", "right"),
		key.WithHelp("tab", "next tab"),
	),
	PrevTab: key.NewBinding(
		key.WithKeys("shift+tab", "left"),
		key.WithHelp("shift+tab", "prev tab"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+q", "ctrl+c"),
		key.WithHelp("q/ctrl+q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Command: key.NewBinding(
		key.WithKeys("/", "ctrl+p"),
		key.WithHelp("/", "command"),
	),
	Search: key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "filter"),
	),
	Logs: key.NewBinding(
		key.WithKeys("l", "ctrl+l"),
		key.WithHelp("l/ctrl+l", "logs"),
	),
	Send: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "send"),
	),
}
