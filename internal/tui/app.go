package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"a2a-go/internal/hub"
	"a2a-go/internal/transport"
	"a2a-go/internal/types"
	"a2a-go/internal/utils"
)

const (
	tabStatus = iota
	tabAgents
	tabTasks
	tabSend
	tabHistory
	tabSettings
	tabCount
)

var (
	headerStyle  = lipgloss.NewStyle().Bold(true)
	footerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	logStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	confirmStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
)

type statusData struct {
	Version     string `json:"version"`
	Uptime      int    `json:"uptime"`
	Total       int    `json:"total"`
	Healthy     int    `json:"healthy"`
	Degraded    int    `json:"degraded"`
	Unhealthy   int    `json:"unhealthy"`
	Unknown     int    `json:"unknown"`
	ActiveTasks int    `json:"activeTasks"`
	TotalTasks  int    `json:"totalTasks"`
}

type agentData struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Card         types.AgentCard   `json:"card"`
	Health       types.AgentHealth `json:"health"`
	RegisteredAt string            `json:"registeredAt"`
}

type model struct {
	cfg    hub.Config
	logger *utils.Logger
	caller *hub.LocalCaller
	server *hub.Server
	ctx    context.Context
	cancel context.CancelFunc

	width     int
	height    int
	activeTab int

	status        statusData
	agents        []agentData
	tasks         []types.Task
	responses     []responseEntry
	sendLog       []sendEntry
	sendLogOffset int

	agentInput      textinput.Model
	msgInput        textarea.Model
	focusIndex      int
	agentsList      list.Model
	tasksList       list.Model
	responsesList   list.Model
	detailViewport  viewport.Model
	keys            keyMap
	help            help.Model
	showHelp        bool
	commandMode     bool
	commandInput    textinput.Model
	commandHistory  []string
	historyIndex    int
	commandIndex    int
	commandResults  []commandSpec
	spinner         spinner.Model
	refreshing      bool
	pendingRefresh  int
	showLogs        bool
	logs            []logEntry
	logViewport     viewport.Model
	logLines        []string
	showSendModal   bool
	agentIndex      int
	taskIndex       int
	historySel      int
	detailContent   string
	settingsInput   textinput.Model
	settingsMessage string
	confirmQuit     bool
	confirmMessage  string

	lastUpdated  time.Time
	errMsg       string
	sending      bool
	lastResponse string
}

type sendEntry struct {
	Role      string
	Agent     string
	Text      string
	Timestamp string
}

type statusMsg struct{ data statusData }

type agentsMsg struct{ data []agentData }

type tasksMsg struct{ data []types.Task }

type errMsg struct {
	err    error
	source string
}

type sentMsg struct{ text string }
type sendResultMsg struct{ entry responseEntry }
type refreshStartMsg struct{ count int }

type tickMsg time.Time

func Run(cfg hub.Config, logger *utils.Logger) error {
	server := hub.NewServer(cfg, logger)
	server.RegisterHandlers()
	if err := server.LoadState(); err != nil {
		logger.Warnf("failed to load state: %v", err)
	}
	baseURL := fmt.Sprintf("http://%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)
	_ = server.InitAgents(baseURL)
	if err := server.WritePid(); err != nil {
		logger.Warnf("failed to write pid: %v", err)
	}
	orchestratorList := server.OrchestratorAgents()
	server.Registry().StartHealthChecks(30 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	if cfg.Socket.Enabled {
		unixTransport := transport.NewUnixTransport(cfg, server, logger)
		go func() {
			if err := unixTransport.Start(ctx); err != nil {
				logger.Errorf("unix transport error: %v", err)
			}
		}()
	}
	if cfg.HTTP.Enabled {
		httpTransport := transport.NewHTTPTransport(cfg, server, logger)
		go func() {
			if err := httpTransport.Start(ctx); err != nil {
				logger.Errorf("http transport error: %v", err)
			}
		}()
	}

	caller := hub.NewLocalCaller(server.Handler())
	agentInput := textinput.New()
	agentInput.Placeholder = "agent id"
	defaultAgent := server.LastAgent()
	if defaultAgent == "" {
		defaultAgent = "orchestrator"
	}
	agentInput.SetValue(defaultAgent)
	msgInput := textarea.New()
	msgInput.Placeholder = "message"
	msgInput.Focus()
	msgInput.Prompt = ""
	msgInput.ShowLineNumbers = false
	commandInput := textinput.New()
	commandInput.Placeholder = "command"
	commandInput.Prompt = "/ "
	spin := spinner.New()
	spin.Spinner = spinner.Line
	spin.Style = dimStyle
	settingsInput := textinput.New()
	settingsInput.Placeholder = "orchestrator agents (comma-separated)"
	settingsInput.SetValue(strings.Join(orchestratorList, ","))
	agentsList := newListModel()
	tasksList := newListModel()
	responsesList := newListModel()
	detailViewport := viewport.New(0, 0)
	logViewport := viewport.New(0, 6)

	m := model{
		cfg:             cfg,
		logger:          logger,
		caller:          caller,
		server:          server,
		ctx:             ctx,
		cancel:          cancel,
		activeTab:       tabSend,
		agentInput:      agentInput,
		msgInput:        msgInput,
		commandInput:    commandInput,
		focusIndex:      1,
		agentsList:      agentsList,
		tasksList:       tasksList,
		responsesList:   responsesList,
		detailViewport:  detailViewport,
		keys:            defaultKeyMap,
		help:            help.New(),
		commandHistory:  []string{},
		historyIndex:    0,
		commandIndex:    0,
		spinner:         spin,
		showLogs:        false,
		logs:            []logEntry{},
		logViewport:     logViewport,
		logLines:        []string{},
		sendLog:         []sendEntry{},
		settingsInput:   settingsInput,
		settingsMessage: "",
		showSendModal:   true,
	}
	m.updateMessagePrompt()

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	server.Registry().Stop()
	server.RemovePid()
	cancel()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Batch(refreshAllCmd(m.caller), tickCmd(), m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case statusMsg:
		m.status = msg.data
		m.lastUpdated = time.Now()
		m.finishRefresh()
	case agentsMsg:
		m.agents = msg.data
		m.lastUpdated = time.Now()
		m.agentsList.SetItems(buildAgentItems(m.agents))
		m.finishRefresh()
		m.updateDetailForTab(tabAgents)
	case tasksMsg:
		m.tasks = msg.data
		m.lastUpdated = time.Now()
		m.tasksList.SetItems(buildTaskItems(m.tasks))
		m.finishRefresh()
		m.updateDetailForTab(tabTasks)
	case errMsg:
		m.errMsg = msg.err.Error()
		m.sending = false
		m.addLog("error", msg.err.Error())
		if msg.source == "send" {
			m.appendSendEntry("error", "", msg.err.Error())
		}
		if msg.source == "refresh" {
			m.finishRefresh()
		}
	case sentMsg:
		m.errMsg = ""
		m.msgInput.SetValue("")
		m.lastUpdated = time.Now()
		m.sending = false
		m.appendSendEntry("agent", "", msg.text)
	case sendResultMsg:
		m.lastResponse = msg.entry.Text
		m.appendSendEntry("agent", msg.entry.Agent, msg.entry.Text)
		m.responses = append([]responseEntry{msg.entry}, m.responses...)
		m.responsesList.SetItems(buildResponseItems(m.responses))
		m.sending = false
		m.addLog("info", "response received from "+msg.entry.Agent)
		m.updateDetailForTab(tabHistory)
		return m, refreshAllCmd(m.caller)
	case refreshStartMsg:
		m.pendingRefresh += msg.count
		m.refreshing = m.pendingRefresh > 0
		return m, m.spinner.Tick
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.refreshing || m.sending {
			return m, cmd
		}
		return m, nil
	case tickMsg:
		return m, tea.Batch(refreshAllCmd(m.caller), tickCmd())
	case tea.KeyMsg:
		if msg.String() == "esc" && !m.commandMode {
			if m.confirmQuit {
				m.confirmQuit = false
				m.confirmMessage = ""
			}
			if m.showHelp {
				m.showHelp = false
			}
			if m.showLogs {
				m.showLogs = false
			}
			if m.showSendModal {
				m.showSendModal = false
				m.msgInput.Blur()
				m.agentInput.Blur()
			}
			if m.activeTab == tabSettings {
				m.setSettingsFocus(false)
			}
			if m.listFilteringActive() {
				_ = m.updateActiveList(msg)
			}
			m.commandMode = true
			m.commandInput.Focus()
			m.historyIndex = len(m.commandHistory)
			m.commandIndex = 0
			m.updateCommandResults()
			return m, nil
		}
		if m.confirmQuit {
			switch msg.String() {
			case "y", "enter":
				return m, tea.Quit
			case "n", "esc":
				m.confirmQuit = false
				m.confirmMessage = ""
				return m, nil
			}
		}
		if m.showSendModal && !m.commandMode {
			switch msg.String() {
			case "ctrl+p":
				m.commandMode = true
				m.commandInput.Focus()
				m.historyIndex = len(m.commandHistory)
				m.commandIndex = 0
				m.updateCommandResults()
				return m, nil
			case "esc":
				m.showSendModal = false
				m.msgInput.Blur()
				m.agentInput.Blur()
				return m, nil
			case "tab", "shift+tab":
				if m.focusIndex == 0 {
					m.server.UpdateLastAgent(m.agentInput.Value())
					m.focusIndex = 1
					m.agentInput.Blur()
					m.msgInput.Focus()
				} else {
					m.focusIndex = 0
					m.msgInput.Blur()
					m.agentInput.Focus()
				}
				return m, nil
			case "up", "down":
				if m.focusIndex != 1 || strings.TrimSpace(m.msgInput.Value()) == "" {
					m.scrollSendLog(msg.String())
					return m, nil
				}
			case "pgup", "pgdown", "ctrl+u", "ctrl+d":
				m.scrollSendLog(msg.String())
				return m, nil
			case "enter":
				return m, m.startSend(m.agentInput.Value(), m.msgInput.Value())
			case "/":
				if m.focusIndex == 1 && strings.TrimSpace(m.msgInput.Value()) == "" {
					m.commandMode = true
					m.commandInput.Focus()
					m.historyIndex = len(m.commandHistory)
					m.commandIndex = 0
					m.updateCommandResults()
					return m, nil
				}
			case "shift+enter":
				m.msgInput.InsertString("\n")
				return m, nil
			case "ctrl+s", "ctrl+enter", "alt+enter":
				return m, m.startSend(m.agentInput.Value(), m.msgInput.Value())
			}
			var cmd tea.Cmd
			m.agentInput, cmd = m.agentInput.Update(msg)
			m.msgInput, _ = m.msgInput.Update(msg)
			return m, cmd
		}
		if m.commandMode {
			switch msg.String() {
			case "esc":
				m.commandMode = false
				m.commandInput.Blur()
				m.commandInput.SetValue("")
				m.historyIndex = len(m.commandHistory)
				m.commandIndex = 0
				m.activeTab = tabSend
				m.showSendModal = true
				m.focusIndex = 1
				m.agentInput.Blur()
				m.msgInput.Focus()
				return m, nil
			case "enter":
				cmdText := strings.TrimSpace(m.commandInput.Value())
				if len(m.commandResults) > 0 && !strings.Contains(cmdText, " ") {
					cmdText = "/" + m.commandResults[m.commandIndex].Name
				}
				m.commandInput.SetValue("")
				m.commandInput.Blur()
				m.commandMode = false
				m.historyIndex = len(m.commandHistory)
				m.commandIndex = 0
				if cmdText == "" {
					m.activeTab = tabSend
					m.showSendModal = true
					m.focusIndex = 1
					m.agentInput.Blur()
					m.msgInput.Focus()
					return m, nil
				}
				m.appendCommandHistory(cmdText)
				return m, m.applyCommand(cmdText)
			case "up":
				if m.navigateCommandSelection(-1) {
					return m, nil
				}
				m.navigateHistory(-1)
				return m, nil
			case "down":
				if m.navigateCommandSelection(1) {
					return m, nil
				}
				m.navigateHistory(1)
				return m, nil
			}
			var cmd tea.Cmd
			m.commandInput, cmd = m.commandInput.Update(msg)
			m.updateCommandResults()
			return m, cmd
		}
		if m.listFilteringActive() {
			cmd := m.updateActiveList(msg)
			return m, cmd
		}
		if m.showHelp && (msg.String() == "?" || msg.String() == "esc") {
			m.showHelp = false
			return m, nil
		}
		if key.Matches(msg, m.keys.Help) {
			m.showHelp = !m.showHelp
			return m, nil
		}
		inputActive := m.activeTab == tabSend || m.activeTab == tabSettings
		if inputActive {
			if msg.String() == "ctrl+l" {
				m.showLogs = !m.showLogs
				m.logViewport.GotoBottom()
				return m, nil
			}
			if key.Matches(msg, m.keys.Command) {
				if msg.String() != "/" || (m.activeTab == tabSend && m.focusIndex == 1 && strings.TrimSpace(m.msgInput.Value()) == "") {
					m.commandMode = true
					m.commandInput.Focus()
					m.historyIndex = len(m.commandHistory)
					m.commandIndex = 0
					m.updateCommandResults()
					return m, nil
				}
			}
		}
		if !inputActive {
			if key.Matches(msg, m.keys.Command) {
				m.commandMode = true
				m.commandInput.Focus()
				m.historyIndex = len(m.commandHistory)
				m.commandIndex = 0
				m.updateCommandResults()
				return m, nil
			}
			if key.Matches(msg, m.keys.Send) {
				m.showSendModal = true
				m.focusIndex = 1
				m.agentInput.Blur()
				m.msgInput.Focus()
				return m, nil
			}
			if key.Matches(msg, m.keys.Search) && m.activeTab != tabSettings {
				cmd := m.updateActiveList(msg)
				return m, cmd
			}
			if key.Matches(msg, m.keys.Logs) {
				m.showLogs = !m.showLogs
				m.logViewport.GotoBottom()
				return m, nil
			}
			if key.Matches(msg, m.keys.Quit) {
				if m.sending || m.refreshing {
					m.confirmQuit = true
					m.confirmMessage = "Work in progress. Quit anyway? (y/n)"
					return m, nil
				}
				return m, tea.Quit
			}
		} else if msg.String() == "ctrl+c" || msg.String() == "ctrl+q" {
			return m, tea.Quit
		}
		switch msg.String() {
		case "r":
			return m, refreshAllCmd(m.caller)
		}
	}

	if m.activeTab == tabSettings {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "enter":
				agents := parseAgentList(m.settingsInput.Value())
				label := strings.Join(agents, ",")
				if label == "" {
					label = "none"
				}
				if m.server.UpdateOrchestratorAgents(agents) {
					m.settingsMessage = "Updated orchestrator delegates: " + label
				} else {
					m.settingsMessage = "Saved settings; restart to apply: " + label
				}
				m.settingsInput.SetValue(strings.Join(agents, ","))
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.settingsInput, cmd = m.settingsInput.Update(msg)
		return m, cmd
	}

	if m.activeTab == tabSend {
		var cmd tea.Cmd
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+enter", "alt+enter", "ctrl+s":
				return m, m.startSend(m.agentInput.Value(), m.msgInput.Value())
			case "enter":
				if m.focusIndex == 0 {
					m.server.UpdateLastAgent(m.agentInput.Value())
					m.focusIndex = 1
					m.agentInput.Blur()
					m.msgInput.Focus()
				}
			case "esc":
				m.focusIndex = 0
				m.msgInput.Blur()
				m.agentInput.Focus()
			}
		}
		m.agentInput, cmd = m.agentInput.Update(msg)
		m.msgInput, _ = m.msgInput.Update(msg)
		return m, cmd
	}

	if m.activeTab == tabAgents || m.activeTab == tabTasks || m.activeTab == tabHistory {
		cmd := m.updateActiveList(msg)
		return m, cmd
	}

	if m.showLogs {
		if keyMsg, ok := msg.(tea.KeyMsg); ok && isViewportKey(keyMsg) {
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(keyMsg)
			return m, cmd
		}
	}

	return m, nil
}

func (m model) View() string {
	header := headerStyle.Render("A2A Hub")
	statusBar := m.renderStatusBar()
	viewLine := dimStyle.Render("View: " + m.viewName())
	errLine := ""
	if m.errMsg != "" {
		errLine = errStyle.Render(m.errMsg)
	}
	confirmLine := ""
	if m.confirmQuit {
		confirmLine = confirmStyle.Render(m.confirmMessage)
	}
	body := ""
	switch m.activeTab {
	case tabStatus:
		body = m.viewStatus()
	case tabAgents:
		body = m.viewAgents()
	case tabTasks:
		body = m.viewTasks()
	case tabSend:
		if m.showSendModal {
			body = ""
		} else {
			body = m.viewSend()
		}
	case tabHistory:
		body = m.viewHistory()
	case tabSettings:
		body = m.viewSettings()
	}
	footer := footerStyle.Render(m.help.ShortHelpView(m.keys.ShortHelp()))
	if m.showHelp {
		body = strings.Join([]string{body, "", m.help.FullHelpView(m.keys.FullHelp())}, "\n")
	}
	if m.showLogs {
		body = strings.Join([]string{body, "", m.renderLogPanel(m.logViewport.Height)}, "\n")
	}
	content := strings.Join([]string{
		header,
		statusBar,
		viewLine,
		errLine,
		confirmLine,
		"",
		body,
		"",
		footer,
	}, "\n")
	base := renderCentered(content, m.width, m.height)
	if m.showSendModal {
		base = overlayModal(dimStyle.Render(base), m.renderSendModal(), m.width, m.height)
	}
	if m.commandMode {
		return overlayModal(dimStyle.Render(base), m.renderCommandModal(), m.width, m.height)
	}
	return base
}

func (m *model) applyCommand(input string) tea.Cmd {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	parts := splitArgs(input)
	if len(parts) == 0 {
		return nil
	}
	command := strings.TrimLeft(parts[0], "/:")
	if command == "q" {
		command = "quit"
	}
	switch strings.ToLower(command) {
	case "status":
		m.activeTab = tabStatus
		m.showSendModal = false
		m.setSettingsFocus(false)
		return refreshAllCmd(m.caller)
	case "agents":
		m.activeTab = tabAgents
		m.showSendModal = false
		m.setSettingsFocus(false)
		return refreshAllCmd(m.caller)
	case "tasks":
		m.activeTab = tabTasks
		m.showSendModal = false
		m.setSettingsFocus(false)
		return refreshAllCmd(m.caller)
	case "history":
		m.activeTab = tabHistory
		m.showSendModal = false
		m.setSettingsFocus(false)
		return nil
	case "settings":
		m.activeTab = tabSettings
		m.showSendModal = false
		m.setSettingsFocus(true)
		return nil
	case "send":
		m.activeTab = tabSend
		m.showSendModal = true
		m.focusIndex = 1
		m.agentInput.Blur()
		m.msgInput.Focus()
		m.setSettingsFocus(false)
		if len(parts) >= 3 {
			agent := parts[1]
			message := strings.Join(parts[2:], " ")
			m.agentInput.SetValue(agent)
			m.server.UpdateLastAgent(agent)
			return m.startSend(agent, message)
		}
		return nil
	case "agent":
		m.activeTab = tabSend
		m.showSendModal = true
		m.focusIndex = 1
		m.agentInput.Blur()
		m.msgInput.Focus()
		m.setSettingsFocus(false)
		if len(parts) >= 2 {
			m.agentInput.SetValue(parts[1])
			m.server.UpdateLastAgent(parts[1])
		}
		return nil
	case "refresh":
		if m.activeTab == tabSend {
			m.showSendModal = true
			m.focusIndex = 1
			m.agentInput.Blur()
			m.msgInput.Focus()
		}
		return refreshAllCmd(m.caller)
	case "help":
		m.showHelp = true
		return nil
	case "quit", "exit":
		return tea.Quit
	default:
		m.errMsg = fmt.Sprintf("unknown command: %s", input)
		m.addLog("warn", m.errMsg)
		return nil
	}
}

func (m model) renderCommandPalette() string {
	lines := []string{
		m.commandInput.View(),
	}
	if len(m.commandResults) > 0 {
		lines = append(lines, "")
		for i, cmd := range m.commandResults {
			line := fmt.Sprintf("%s - %s", cmd.Usage, cmd.Description)
			if i == m.commandIndex {
				lines = append(lines, confirmStyle.Render("> "+line))
			} else {
				lines = append(lines, dimStyle.Render("  "+line))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) renderCommandModal() string {
	width, height := modalSize(m.width, m.height)
	m.commandInput.Width = width - 6
	title := headerStyle.Render("Command")
	body := strings.Join([]string{
		m.renderCommandPalette(),
		"",
		dimStyle.Render("Type /status, /agents, /tasks, /history, /settings, /send <agent> <msg>"),
	}, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(width).
		Height(height).
		Render(strings.Join([]string{title, "", body}, "\n"))
	return box
}

func (m model) commandSuggestions() []string {
	if len(m.commandResults) == 0 {
		return nil
	}
	lines := make([]string, 0, len(m.commandResults))
	for _, cmd := range m.commandResults {
		lines = append(lines, fmt.Sprintf("%s - %s", cmd.Usage, cmd.Description))
	}
	return lines
}

func (m *model) updateCommandResults() {
	input := strings.TrimSpace(m.commandInput.Value())
	candidates := commandCatalog
	if input == "" {
		m.commandResults = candidates[:min(8, len(candidates))]
		m.commandIndex = 0
		return
	}
	parts := splitArgs(input)
	prefix := strings.TrimLeft(strings.ToLower(parts[0]), "/:")
	filtered := make([]commandSpec, 0, len(candidates))
	for _, cmd := range candidates {
		if strings.HasPrefix(cmd.Name, prefix) {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) > 8 {
		filtered = filtered[:8]
	}
	m.commandResults = filtered
	if m.commandIndex >= len(filtered) {
		m.commandIndex = 0
	}
}

func (m *model) navigateCommandSelection(delta int) bool {
	if len(m.commandResults) == 0 {
		return false
	}
	next := m.commandIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.commandResults) {
		next = len(m.commandResults) - 1
	}
	if next == m.commandIndex {
		return false
	}
	m.commandIndex = next
	return true
}

type commandSpec struct {
	Name        string
	Usage       string
	Description string
}

var commandCatalog = []commandSpec{
	{Name: "status", Usage: "/status", Description: "show hub status"},
	{Name: "agents", Usage: "/agents", Description: "show agents list"},
	{Name: "tasks", Usage: "/tasks", Description: "show tasks list"},
	{Name: "history", Usage: "/history", Description: "show response history"},
	{Name: "settings", Usage: "/settings", Description: "show runtime settings"},
	{Name: "send", Usage: "/send <agent> <msg>", Description: "send a message"},
	{Name: "agent", Usage: "/agent <id>", Description: "set agent in Send tab"},
	{Name: "refresh", Usage: "/refresh", Description: "refresh data"},
	{Name: "help", Usage: "/help", Description: "show help overlay"},
	{Name: "quit", Usage: "/quit", Description: "exit the TUI"},
	{Name: "q", Usage: "/q", Description: "exit the TUI"},
}

func (m *model) appendCommandHistory(cmd string) {
	if cmd == "" {
		return
	}
	if len(m.commandHistory) > 0 && m.commandHistory[len(m.commandHistory)-1] == cmd {
		return
	}
	m.commandHistory = append(m.commandHistory, cmd)
	m.historyIndex = len(m.commandHistory)
}

func (m *model) navigateHistory(delta int) {
	if len(m.commandHistory) == 0 {
		return
	}
	if m.historyIndex < 0 || m.historyIndex > len(m.commandHistory) {
		m.historyIndex = len(m.commandHistory)
	}
	next := m.historyIndex + delta
	if next < 0 {
		next = 0
	}
	if next > len(m.commandHistory) {
		next = len(m.commandHistory)
	}
	m.historyIndex = next
	if next == len(m.commandHistory) {
		m.commandInput.SetValue("")
		m.commandInput.CursorEnd()
		return
	}
	m.commandInput.SetValue(m.commandHistory[next])
	m.commandInput.CursorEnd()
}

func splitArgs(input string) []string {
	var args []string
	var buf strings.Builder
	var quote rune
	escaped := false
	for _, r := range input {
		switch {
		case escaped:
			buf.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				buf.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t':
			if buf.Len() > 0 {
				args = append(args, buf.String())
				buf.Reset()
			}
		default:
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		args = append(args, buf.String())
	}
	return args
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *model) finishRefresh() {
	if m.pendingRefresh > 0 {
		m.pendingRefresh--
	}
	if m.pendingRefresh <= 0 {
		m.pendingRefresh = 0
		m.refreshing = false
	}
}

type logEntry struct {
	Time    time.Time
	Level   string
	Message string
}

func (m *model) addLog(level, message string) {
	entry := logEntry{Time: time.Now().UTC(), Level: level, Message: message}
	m.logs = append(m.logs, entry)
	if len(m.logs) > 200 {
		m.logs = m.logs[len(m.logs)-200:]
	}
	m.rebuildLogLines()
	m.logViewport.GotoBottom()
}

func (m model) renderLogPanel(maxLines int) string {
	if len(m.logs) == 0 {
		return logStyle.Render("No logs yet.")
	}
	if maxLines <= 0 {
		maxLines = 6
	}
	width, _ := contentSize(m.width, m.height)
	if width <= 0 {
		width = 80
	}
	if width < 30 {
		width = 30
	}
	lines := make([]string, 0, len(m.logLines))
	for _, line := range m.logLines {
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return logStyle.Render("No logs yet.")
	}
	content := strings.Join(lines, "\n")
	header := dimStyle.Render("Logs (scroll with pgup/pgdown)")
	m.logViewport.Height = maxLines
	m.logViewport.Width = width
	m.logViewport.SetContent(content)
	panel := strings.Join([]string{header, m.logViewport.View()}, "\n")
	return logStyle.Render(panel)
}

func (m *model) rebuildLogLines() {
	lines := make([]string, 0, len(m.logs))
	for _, entry := range m.logs {
		level := strings.ToUpper(entry.Level)
		prefix := fmt.Sprintf("%s %-5s", entry.Time.Format("15:04:05"), level)
		lines = append(lines, fmt.Sprintf("%s  %s", prefix, entry.Message))
	}
	m.logLines = lines
}

func (m model) viewStatus() string {
	width, _ := m.bodySize()
	left := []string{
		fmt.Sprintf("Version: %s", m.status.Version),
		fmt.Sprintf("Uptime: %ds", m.status.Uptime),
		fmt.Sprintf("Agents: %d", m.status.Total),
		fmt.Sprintf("Healthy: %d", m.status.Healthy),
		fmt.Sprintf("Degraded: %d", m.status.Degraded),
		fmt.Sprintf("Unhealthy: %d", m.status.Unhealthy),
		fmt.Sprintf("Unknown: %d", m.status.Unknown),
		fmt.Sprintf("Tasks: %d", m.status.TotalTasks),
	}
	right := []string{
		"Hub status summary",
		"",
		fmt.Sprintf("Active tasks: %d", m.status.ActiveTasks),
	}
	if !m.lastUpdated.IsZero() {
		right = append(right, fmt.Sprintf("Last refresh: %s", m.lastUpdated.Format(time.RFC822)))
	}
	return renderTwoPane(width, strings.Join(left, "\n"), strings.Join(right, "\n"))
}

func (m model) viewAgents() string {
	leftWidth, rightWidth, height, stacked := m.paneSizes()
	if stacked {
		listHeight := height / 2
		if listHeight < 4 {
			listHeight = 4
		}
		detailHeight := height - listHeight - 1
		if detailHeight < 4 {
			detailHeight = 4
			listHeight = height - detailHeight - 1
		}
		m.agentsList.SetSize(leftWidth, listHeight)
		m.detailViewport.Width = leftWidth
		m.detailViewport.Height = detailHeight
		return strings.Join([]string{
			m.agentsList.View(),
			dimStyle.Render(strings.Repeat("─", leftWidth)),
			m.detailViewport.View(),
		}, "\n")
	}
	m.agentsList.SetSize(leftWidth, height)
	m.detailViewport.Width = rightWidth
	m.detailViewport.Height = height
	return lipgloss.JoinHorizontal(lipgloss.Top, m.agentsList.View(), m.detailViewport.View())
}

func (m model) viewTasks() string {
	leftWidth, rightWidth, height, stacked := m.paneSizes()
	if stacked {
		listHeight := height / 2
		if listHeight < 4 {
			listHeight = 4
		}
		detailHeight := height - listHeight - 1
		if detailHeight < 4 {
			detailHeight = 4
			listHeight = height - detailHeight - 1
		}
		m.tasksList.SetSize(leftWidth, listHeight)
		m.detailViewport.Width = leftWidth
		m.detailViewport.Height = detailHeight
		return strings.Join([]string{
			m.tasksList.View(),
			dimStyle.Render(strings.Repeat("─", leftWidth)),
			m.detailViewport.View(),
		}, "\n")
	}
	m.tasksList.SetSize(leftWidth, height)
	m.detailViewport.Width = rightWidth
	m.detailViewport.Height = height
	return lipgloss.JoinHorizontal(lipgloss.Top, m.tasksList.View(), m.detailViewport.View())
}

func (m model) viewSend() string {
	width, height := m.bodySize()
	if width <= 0 {
		width = 80
	}
	if width < 30 {
		width = 30
	}
	if height <= 0 {
		height = 24
	}
	m.agentInput.Width = width - 4
	m.msgInput.SetWidth(width - 4)
	msgHeight := 4
	if height > 22 {
		msgHeight = 5
	}
	m.msgInput.SetHeight(msgHeight)
	availableHeight := height - 6
	if availableHeight < 8 {
		availableHeight = 8
	}
	logHeight := availableHeight - (4 + msgHeight)
	if logHeight < 1 {
		logHeight = 1
	}
	log := m.renderSendLog(width-4, logHeight)
	lines := []string{
		"Agent:",
		m.agentInput.View(),
		log,
		"Message:",
		m.msgInput.View(),
		"enter to send, shift+enter for newline, up/down scroll log, esc to edit agent",
	}
	return strings.Join(lines, "\n")
}

func (m model) renderSendModal() string {
	width, height := modalSize(m.width, m.height)

	inputWidth, msgHeight, logHeight := sendModalLayout(width, height)
	m.agentInput.Width = inputWidth
	m.msgInput.SetWidth(inputWidth)
	m.msgInput.SetHeight(msgHeight)
	log := m.renderSendLog(inputWidth, logHeight)

	title := headerStyle.Render("Send Message")
	body := strings.Join([]string{
		"Agent:",
		m.agentInput.View(),
		log,
		"Message:",
		m.msgInput.View(),
		"enter to send, shift+enter for newline, up/down scroll log, esc to close",
	}, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(width).
		Height(height).
		Render(strings.Join([]string{title, "", body}, "\n"))
	return box
}

func modalSize(width, height int) (int, int) {
	return panelSize(width, height)
}

func contentSize(width, height int) (int, int) {
	panelWidth, panelHeight := panelSize(width, height)
	contentWidth := panelWidth - 6
	contentHeight := panelHeight - 4
	if contentWidth < 1 {
		contentWidth = 1
	}
	if contentHeight < 1 {
		contentHeight = 1
	}
	return contentWidth, contentHeight
}

func (m model) bodySize() (int, int) {
	return bodySize(m.width, m.height)
}

func bodySize(width, height int) (int, int) {
	contentWidth, contentHeight := contentSize(width, height)
	bodyHeight := contentHeight - 8
	if bodyHeight < 4 {
		bodyHeight = 4
	}
	return contentWidth, bodyHeight
}

func panelSize(width, height int) (int, int) {
	if width <= 0 || height <= 0 {
		return width, height
	}
	padW := width / 10
	padH := height / 10
	if padW < 1 {
		padW = 1
	}
	if padH < 1 {
		padH = 1
	}
	if width-2*padW < 1 {
		padW = 0
	}
	if height-2*padH < 1 {
		padH = 0
	}
	return width - 2*padW, height - 2*padH
}

func sendModalLayout(width, height int) (int, int, int) {
	inputWidth := width - 6
	if inputWidth < 20 {
		inputWidth = 20
	}
	msgHeight := 3
	if height >= 16 {
		msgHeight = 4
	}
	contentHeight := height - 4
	logHeight := contentHeight - (4 + msgHeight)
	if logHeight < 1 {
		logHeight = 1
	}
	return inputWidth, msgHeight, logHeight
}

func (m model) viewHistory() string {
	leftWidth, rightWidth, height, stacked := m.paneSizes()
	if stacked {
		listHeight := height / 2
		if listHeight < 4 {
			listHeight = 4
		}
		detailHeight := height - listHeight - 1
		if detailHeight < 4 {
			detailHeight = 4
			listHeight = height - detailHeight - 1
		}
		m.responsesList.SetSize(leftWidth, listHeight)
		m.detailViewport.Width = leftWidth
		m.detailViewport.Height = detailHeight
		return strings.Join([]string{
			m.responsesList.View(),
			dimStyle.Render(strings.Repeat("─", leftWidth)),
			m.detailViewport.View(),
		}, "\n")
	}
	m.responsesList.SetSize(leftWidth, height)
	m.detailViewport.Width = rightWidth
	m.detailViewport.Height = height
	return lipgloss.JoinHorizontal(lipgloss.Top, m.responsesList.View(), m.detailViewport.View())
}

func (m model) viewSettings() string {
	m.settingsInput.Width = 60
	currentDelegates := strings.Join(m.server.OrchestratorAgents(), ",")
	lines := []string{
		headerStyle.Render("Runtime Settings"),
		"",
		fmt.Sprintf("Data dir: %s", m.server.Config().DataDir),
		fmt.Sprintf("Socket: %s (enabled: %t)", m.server.Config().Socket.Path, m.server.Config().Socket.Enabled),
		fmt.Sprintf("HTTP: %s:%d (enabled: %t)", m.server.Config().HTTP.Host, m.server.Config().HTTP.Port, m.server.Config().HTTP.Enabled),
		"",
		"Orchestrator delegates (comma-separated):",
		m.settingsInput.View(),
		fmt.Sprintf("Current delegates: %s", currentDelegates),
		"Press enter to apply changes (use 'none' to disable)",
	}
	if m.settingsMessage != "" {
		lines = append(lines, "", m.settingsMessage)
	}
	lines = append(lines, "", "Agent executables:")
	lines = append(lines, m.renderExecList())
	return strings.Join(lines, "\n")
}

func (m model) renderStatusBar() string {
	parts := []string{}
	if m.refreshing || m.sending {
		parts = append(parts, m.spinner.View())
	}
	parts = append(parts,
		fmt.Sprintf("v%s", m.status.Version),
		fmt.Sprintf("agents %d/%d", m.status.Healthy, m.status.Total),
		fmt.Sprintf("tasks %d", m.status.TotalTasks),
	)
	if !m.lastUpdated.IsZero() {
		parts = append(parts, "refreshed "+m.lastUpdated.Format("15:04:05"))
	}
	line := strings.Join(parts, "  ")
	width, _ := contentSize(m.width, m.height)
	if width > 0 {
		return dimStyle.Width(width).Render(line)
	}
	return dimStyle.Render(line)
}

func (m model) paneSizes() (int, int, int, bool) {
	width, height := m.bodySize()
	if height < 6 {
		height = 6
	}
	if width <= 0 {
		return 30, 50, height, false
	}
	if width < 80 {
		return width, 0, height, true
	}
	leftWidth := int(float64(width) * 0.35)
	if leftWidth < 24 {
		leftWidth = 24
	}
	if leftWidth > width-20 {
		leftWidth = width / 2
	}
	rightWidth := width - leftWidth - 2
	if rightWidth < 20 {
		rightWidth = 20
		leftWidth = width - rightWidth - 2
		if leftWidth < 20 {
			leftWidth = 20
		}
	}
	return leftWidth, rightWidth, height, false
}

func renderTwoPane(width int, left, right string) string {
	if width <= 0 {
		return strings.TrimSpace(left + "\n\n" + right)
	}
	if width < 80 {
		separator := strings.Repeat("─", width)
		return strings.Join([]string{left, separator, right}, "\n")
	}
	leftWidth := int(float64(width) * 0.35)
	if leftWidth < 24 {
		leftWidth = 24
	}
	if leftWidth > width-20 {
		leftWidth = width / 2
	}
	rightWidth := width - leftWidth - 2
	if rightWidth < 20 {
		rightWidth = 20
		leftWidth = width - rightWidth - 2
		if leftWidth < 20 {
			leftWidth = 20
		}
	}
	leftView := lipgloss.NewStyle().Width(leftWidth).Render(left)
	rightView := lipgloss.NewStyle().Width(rightWidth).Render(right)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)
}

func (m model) viewName() string {
	switch m.activeTab {
	case tabStatus:
		return "Status"
	case tabAgents:
		return "Agents"
	case tabTasks:
		return "Tasks"
	case tabSend:
		return "Send"
	case tabHistory:
		return "History"
	case tabSettings:
		return "Settings"
	default:
		return "Unknown"
	}
}

func renderCentered(content string, width, height int) string {
	if width <= 0 || height <= 0 {
		return content
	}
	panelWidth, panelHeight := panelSize(width, height)
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(panelWidth).
		Height(panelHeight).
		Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

func newListModel() list.Model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	l.SetShowHelp(false)
	return l
}

func (m model) listFilteringActive() bool {
	switch m.activeTab {
	case tabAgents:
		return m.agentsList.FilterState() == list.Filtering
	case tabTasks:
		return m.tasksList.FilterState() == list.Filtering
	case tabHistory:
		return m.responsesList.FilterState() == list.Filtering
	default:
		return false
	}
}

func (m *model) updateActiveList(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	var prevIndex int
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "enter" && !m.listFilteringActive() {
		if m.activeTab == tabAgents {
			if item, ok := m.agentsList.SelectedItem().(agentItem); ok {
				m.agentInput.SetValue(item.data.ID)
				m.server.UpdateLastAgent(item.data.ID)
				m.showSendModal = true
				m.focusIndex = 1
				m.agentInput.Blur()
				m.msgInput.Focus()
				return nil
			}
		}
	}
	switch m.activeTab {
	case tabAgents:
		prevIndex = m.agentsList.Index()
		m.agentsList, cmd = m.agentsList.Update(msg)
		if prevIndex != m.agentsList.Index() {
			m.updateDetailForTab(tabAgents)
		}
	case tabTasks:
		prevIndex = m.tasksList.Index()
		m.tasksList, cmd = m.tasksList.Update(msg)
		if prevIndex != m.tasksList.Index() {
			m.updateDetailForTab(tabTasks)
		}
	case tabHistory:
		prevIndex = m.responsesList.Index()
		m.responsesList, cmd = m.responsesList.Update(msg)
		if prevIndex != m.responsesList.Index() {
			m.updateDetailForTab(tabHistory)
		}
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok && isViewportKey(keyMsg) {
		m.detailViewport, _ = m.detailViewport.Update(keyMsg)
	}
	return cmd
}

func isViewportKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return true
	default:
		return false
	}
}

func (m *model) updateDetailForTab(tab int) {
	switch tab {
	case tabAgents:
		content := "No agents registered."
		if item, ok := m.agentsList.SelectedItem().(agentItem); ok {
			content = renderAgentDetail(item.data)
			m.agentIndex = m.agentsList.Index()
		}
		m.setDetailContent(content)
	case tabTasks:
		content := "No tasks yet."
		if item, ok := m.tasksList.SelectedItem().(taskItem); ok {
			content = renderTaskDetail(item.data)
			m.taskIndex = m.tasksList.Index()
		}
		m.setDetailContent(content)
	case tabHistory:
		content := "No responses yet."
		if item, ok := m.responsesList.SelectedItem().(responseItem); ok {
			content = renderResponseDetail(item.data)
			m.historySel = m.responsesList.Index()
		}
		m.setDetailContent(content)
	}
}

func (m *model) setDetailContent(content string) {
	if content == m.detailContent {
		return
	}
	m.detailContent = content
	m.detailViewport.SetContent(content)
	m.detailViewport.GotoTop()
}

func overlayModal(base, modal string, width, height int) string {
	if width <= 0 || height <= 0 {
		return base + "\n\n" + modal
	}
	baseLines := normalizeLines(base, width, height)
	modalCanvas := lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
	modalLines := normalizeLines(modalCanvas, width, height)
	for i := 0; i < height; i++ {
		if strings.TrimSpace(stripANSI(modalLines[i])) != "" {
			baseLines[i] = modalLines[i]
		}
	}
	return strings.Join(baseLines, "\n")
}

func (m model) renderExecList() string {
	infos := m.server.AgentsList()
	if len(infos) == 0 {
		return "No agents registered."
	}
	lines := make([]string, 0, len(infos))
	for _, info := range infos {
		execPath := "internal"
		if provider, ok := info.Agent.(interface{ ExecPath() string }); ok {
			execPath = provider.ExecPath()
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", info.Agent.ID(), execPath))
	}
	return strings.Join(lines, "\n")
}

func (m *model) setSettingsFocus(active bool) {
	if active {
		m.settingsInput.Focus()
		return
	}
	m.settingsInput.Blur()
}

func (m *model) updateMessagePrompt() {
	m.msgInput.Prompt = ""
	m.msgInput.SetPromptFunc(0, func(_ int) string {
		return ""
	})
}

func (m *model) startSend(agent, message string) tea.Cmd {
	agent = strings.TrimSpace(agent)
	message = strings.TrimSpace(message)
	if agent == "" || message == "" {
		return nil
	}
	m.errMsg = ""
	m.lastResponse = ""
	m.sending = true
	m.server.UpdateLastAgent(agent)
	m.appendSendEntry("user", agent, message)
	m.msgInput.SetValue("")
	m.msgInput.CursorEnd()
	return tea.Batch(sendCmd(m.caller, agent, message), m.spinner.Tick)
}

func (m *model) appendSendEntry(role, agent, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	m.sendLog = append(m.sendLog, sendEntry{
		Role:      role,
		Agent:     agent,
		Text:      text,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	m.sendLogOffset = 0
}

func (m model) renderSendLog(width, height int) string {
	if height <= 0 {
		return ""
	}
	if width <= 0 {
		width = 20
	}
	wrapWidth := width - 2
	if wrapWidth < 1 {
		wrapWidth = width
	}
	lines := m.sendLogLines(wrapWidth)
	if len(lines) == 0 {
		lines = []string{dimStyle.Render("No messages yet.")}
	}
	maxOffset := max(0, len(lines)-height)
	offset := m.sendLogOffset
	if offset > maxOffset {
		offset = maxOffset
	}
	start := len(lines) - height - offset
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(lines) {
		end = len(lines)
	}
	view := lines[start:end]
	if len(view) < height {
		pad := make([]string, height-len(view))
		view = append(pad, view...)
	}
	return strings.Join(view, "\n")
}

func (m model) sendLogLines(wrapWidth int) []string {
	lines := make([]string, 0, len(m.sendLog)*3)
	for _, entry := range m.sendLog {
		label := entry.Agent
		switch entry.Role {
		case "user":
			label = "You"
			if entry.Agent != "" {
				label = "You -> " + entry.Agent
			}
			lines = append(lines, confirmStyle.Render(label))
		case "error":
			lines = append(lines, errStyle.Render("Error"))
		default:
			if label == "" {
				label = "Agent"
			}
			lines = append(lines, headerStyle.Render(label))
		}
		wrapped := ansi.Wrap(entry.Text, wrapWidth, "")
		for _, line := range strings.Split(wrapped, "\n") {
			lines = append(lines, "  "+line)
		}
		lines = append(lines, "")
	}
	if m.sending {
		lines = append(lines, dimStyle.Render("Waiting for response "+m.spinner.View()))
	}
	if len(lines) > 0 && strings.TrimSpace(stripANSI(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (m *model) scrollSendLog(key string) {
	width, height := modalSize(m.width, m.height)
	inputWidth, _, logHeight := sendModalLayout(width, height)
	if logHeight <= 0 {
		return
	}
	wrapWidth := inputWidth - 2
	if wrapWidth < 1 {
		wrapWidth = inputWidth
	}
	lines := m.sendLogLines(wrapWidth)
	if len(lines) == 0 {
		lines = []string{dimStyle.Render("No messages yet.")}
	}
	maxOffset := max(0, len(lines)-logHeight)
	delta := 0
	switch key {
	case "pgup":
		delta = logHeight
	case "pgdown":
		delta = -logHeight
	case "up":
		delta = 1
	case "down":
		delta = -1
	case "ctrl+u":
		delta = max(1, logHeight/2)
	case "ctrl+d":
		delta = -max(1, logHeight/2)
	}
	if delta == 0 {
		return
	}
	m.sendLogOffset += delta
	if m.sendLogOffset < 0 {
		m.sendLogOffset = 0
	}
	if m.sendLogOffset > maxOffset {
		m.sendLogOffset = maxOffset
	}
}

func parseAgentList(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" || strings.EqualFold(input, "none") {
		return nil
	}
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func normalizeLines(input string, width, height int) []string {
	lines := strings.Split(input, "\n")
	out := make([]string, height)
	pad := lipgloss.NewStyle().Width(width)
	for i := 0; i < height; i++ {
		if i < len(lines) {
			out[i] = pad.Render(lines[i])
		} else {
			out[i] = pad.Render("")
		}
	}
	return out
}

func stripANSI(input string) string {
	return ansi.Strip(input)
}

func refreshAllCmd(caller *hub.LocalCaller) tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return refreshStartMsg{count: 3} },
		fetchStatusCmd(caller),
		fetchAgentsCmd(caller),
		fetchTasksCmd(caller),
	)
}

func fetchStatusCmd(caller *hub.LocalCaller) tea.Cmd {
	return func() tea.Msg {
		resp, err := caller.Call(context.Background(), "hub/status", nil)
		if err != nil {
			return errMsg{err: err, source: "refresh"}
		}
		if resp.Error != nil {
			return errMsg{err: fmt.Errorf(resp.Error.Message), source: "refresh"}
		}
		var status statusData
		if err := decodeResult(resp.Result, &status); err != nil {
			return errMsg{err: err, source: "refresh"}
		}
		return statusMsg{status}
	}
}

func fetchAgentsCmd(caller *hub.LocalCaller) tea.Cmd {
	return func() tea.Msg {
		params, _ := json.Marshal(map[string]any{"includeHealth": true})
		resp, err := caller.Call(context.Background(), "hub/agents/list", params)
		if err != nil {
			return errMsg{err: err, source: "refresh"}
		}
		if resp.Error != nil {
			return errMsg{err: fmt.Errorf(resp.Error.Message), source: "refresh"}
		}
		var agents []agentData
		if err := decodeResult(resp.Result, &agents); err != nil {
			return errMsg{err: err, source: "refresh"}
		}
		return agentsMsg{agents}
	}
}

func fetchTasksCmd(caller *hub.LocalCaller) tea.Cmd {
	return func() tea.Msg {
		params, _ := json.Marshal(map[string]any{"limit": 50, "offset": 0})
		resp, err := caller.Call(context.Background(), "hub/tasks/list", params)
		if err != nil {
			return errMsg{err: err, source: "refresh"}
		}
		if resp.Error != nil {
			return errMsg{err: fmt.Errorf(resp.Error.Message), source: "refresh"}
		}
		var tasks []types.Task
		if err := decodeResult(resp.Result, &tasks); err != nil {
			return errMsg{err: err, source: "refresh"}
		}
		return tasksMsg{tasks}
	}
}

func sendCmd(caller *hub.LocalCaller, agent, message string) tea.Cmd {
	return func() tea.Msg {
		msg := types.Message{
			Kind:      "message",
			MessageID: utils.NewID("msg"),
			Role:      "user",
			Parts:     []types.Part{{Kind: "text", Text: message}},
			Metadata:  map[string]any{"targetAgent": agent},
		}
		params, _ := json.Marshal(map[string]any{
			"message":       msg,
			"configuration": map[string]any{"historyLength": 10},
		})
		resp, err := caller.Call(context.Background(), "message/send", params)
		if err != nil {
			return errMsg{err: err, source: "send"}
		}
		if resp.Error != nil {
			return errMsg{err: fmt.Errorf(resp.Error.Message), source: "send"}
		}
		var task types.Task
		if err := decodeResult(resp.Result, &task); err == nil {
			text := extractTaskText(task)
			entry := responseEntry{
				TaskID:    task.ID,
				Agent:     agent,
				Text:      text,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}
			return sendResultMsg{entry: entry}
		}
		return sentMsg{text: "sent"}
	}
}

func extractTaskText(task types.Task) string {
	if task.Status.Message == nil {
		return string(task.Status.State)
	}
	parts := make([]string, 0, len(task.Status.Message.Parts))
	for _, part := range task.Status.Message.Parts {
		if part.Kind == "text" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func decodeResult(input any, target any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
