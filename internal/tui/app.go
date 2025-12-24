package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
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

const (
	settingsFieldOrchestrator = iota
	settingsFieldClaudeModel
	settingsFieldClaudeTools
	settingsFieldClaudeContinue
	settingsFieldCodexModel
	settingsFieldCodexProfile
	settingsFieldCodexSandbox
	settingsFieldCodexApproval
	settingsFieldCodexSearch
	settingsFieldGeminiModel
	settingsFieldGeminiSandbox
	settingsFieldGeminiApproval
	settingsFieldCount
)

var (
	headerStyle     = lipgloss.NewStyle().Bold(true)
	footerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	logStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	confirmStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	inputBackground = lipgloss.AdaptiveColor{Light: "252", Dark: "236"}
	msgBoxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Background(inputBackground)
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
	cfg          hub.Config
	logger       *utils.Logger
	caller       *hub.LocalCaller
	server       *hub.Server
	ctx          context.Context
	cancel       context.CancelFunc
	sessionStart time.Time

	width     int
	height    int
	activeTab int

	status        statusData
	agents        []agentData
	tasks         []types.Task
	responses     []responseEntry
	sendLog       []sendEntry
	sendViewport  viewport.Model
	sendLogSeeded bool

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
	altScreen       bool
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

	// Claude settings
	claudeModelInput   textinput.Model
	claudeToolsInput   textinput.Model
	claudeContinue     bool
	settingsFocusIndex int

	// Codex settings
	codexModelInput    textinput.Model
	codexProfileInput  textinput.Model
	codexSandboxInput  textinput.Model
	codexApprovalInput textinput.Model
	codexSearch        bool

	// Gemini settings
	geminiModelInput    textinput.Model
	geminiApprovalInput textinput.Model
	geminiSandbox       bool

	confirmQuit    bool
	confirmMessage string

	lastUpdated  time.Time
	errMsg       string
	sending      bool
	lastResponse string

	// Multi-agent support
	activeAgents  map[string]string // agentID -> task text (currently running)
	agentProgress map[string]string // agentID -> "working"/"completed"/"failed"

	// Streaming support
	streamChannels map[string]*AgentStream // agentID -> stream channels
	streamBuffer   map[string][]string     // agentID -> buffered output lines
	focusedAgent   string                  // Which agent has input focus
	pendingPrompts []string                // Queue of agents waiting for input
}

// AgentStream holds the channels for streaming communication with an agent
type AgentStream struct {
	Output chan types.StreamEvent
	Input  chan string
	Done   bool
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

// agentResultMsg is sent when an individual agent completes (for multi-agent dispatch)
type agentResultMsg struct {
	agentID string
	text    string
	err     error
}

// streamEventMsg wraps a streaming event from an agent
type streamEventMsg struct {
	agentID string
	event   types.StreamEvent
}

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
	msgInput.FocusedStyle.Base = msgInput.FocusedStyle.Base.Background(inputBackground)
	msgInput.BlurredStyle.Base = msgInput.BlurredStyle.Base.Background(inputBackground)
	msgInput.FocusedStyle.CursorLine = msgInput.FocusedStyle.CursorLine.Background(inputBackground)
	msgInput.BlurredStyle.CursorLine = msgInput.BlurredStyle.CursorLine.Background(inputBackground)
	commandInput := textinput.New()
	commandInput.Placeholder = "command"
	commandInput.Prompt = "/ "
	spin := spinner.New()
	spin.Spinner = spinner.Line
	spin.Style = dimStyle
	settingsInput := textinput.New()
	settingsInput.Placeholder = "orchestrator agents (comma-separated)"
	settingsInput.SetValue(strings.Join(orchestratorList, ","))

	// Claude settings inputs
	claudeSettings := server.ClaudeSettings()
	claudeModelInput := textinput.New()
	claudeModelInput.Placeholder = "opus, sonnet, haiku (blank for default)"
	claudeModelInput.SetValue(claudeSettings.DefaultModel)
	claudeModelInput.Width = 40

	claudeToolsInput := textinput.New()
	claudeToolsInput.Placeholder = "safe, normal, full (blank for default)"
	claudeToolsInput.SetValue(claudeSettings.DefaultToolProfile)
	claudeToolsInput.Width = 40

	// Codex settings inputs
	codexSettings := server.CodexSettings()
	codexModelInput := textinput.New()
	codexModelInput.Placeholder = "model (blank for default)"
	codexModelInput.SetValue(codexSettings.DefaultModel)
	codexModelInput.Width = 40

	codexProfileInput := textinput.New()
	codexProfileInput.Placeholder = "profile (blank for default)"
	codexProfileInput.SetValue(codexSettings.DefaultProfile)
	codexProfileInput.Width = 40

	codexSandboxInput := textinput.New()
	codexSandboxInput.Placeholder = "read-only, workspace-write, danger-full-access"
	codexSandboxInput.SetValue(codexSettings.DefaultSandbox)
	codexSandboxInput.Width = 40

	codexApprovalInput := textinput.New()
	codexApprovalInput.Placeholder = "untrusted, on-failure, on-request, never"
	codexApprovalInput.SetValue(codexSettings.DefaultApprovalPolicy)
	codexApprovalInput.Width = 40

	// Gemini settings inputs
	geminiSettings := server.GeminiSettings()
	geminiModelInput := textinput.New()
	geminiModelInput.Placeholder = "gemini-1.5-pro, gemini-1.5-flash (blank for default)"
	geminiModelInput.SetValue(geminiSettings.DefaultModel)
	geminiModelInput.Width = 40

	geminiApprovalInput := textinput.New()
	geminiApprovalInput.Placeholder = "default, auto_edit, yolo (blank for default)"
	geminiApprovalInput.SetValue(geminiSettings.DefaultApprovalMode)
	geminiApprovalInput.Width = 40

	agentsList := newListModel()
	tasksList := newListModel()
	responsesList := newListModel()
	detailViewport := viewport.New(0, 0)
	logViewport := viewport.New(0, 6)
	sendViewport := viewport.New(0, 0)

	m := model{
		cfg:                cfg,
		logger:             logger,
		caller:             caller,
		server:             server,
		ctx:                ctx,
		cancel:             cancel,
		sessionStart:       time.Now().UTC(),
		activeTab:          tabSend,
		agentInput:         agentInput,
		msgInput:           msgInput,
		commandInput:       commandInput,
		focusIndex:         1,
		agentsList:         agentsList,
		tasksList:          tasksList,
		responsesList:      responsesList,
		detailViewport:     detailViewport,
		keys:               defaultKeyMap,
		help:               help.New(),
		commandHistory:     []string{},
		historyIndex:       0,
		commandIndex:       0,
		spinner:            spin,
		showLogs:           false,
		altScreen:          true,
		logs:               []logEntry{},
		logViewport:        logViewport,
		logLines:           []string{},
		sendLog:            []sendEntry{},
		sendViewport:       sendViewport,
		settingsInput:      settingsInput,
		settingsMessage:    "",
		claudeModelInput:   claudeModelInput,
		claudeToolsInput:   claudeToolsInput,
		claudeContinue:     claudeSettings.EnableContinue,
		codexModelInput:    codexModelInput,
		codexProfileInput:  codexProfileInput,
		codexSandboxInput:  codexSandboxInput,
		codexApprovalInput: codexApprovalInput,
		codexSearch:        codexSettings.EnableSearch,
		geminiModelInput:   geminiModelInput,
		geminiApprovalInput: geminiApprovalInput,
		geminiSandbox:      geminiSettings.DefaultSandbox,
		settingsFocusIndex: 0,
		showSendModal:      true,
		activeAgents:       make(map[string]string),
		agentProgress:      make(map[string]string),
		streamChannels:     make(map[string]*AgentStream),
		streamBuffer:       make(map[string][]string),
		pendingPrompts:     []string{},
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
		m.syncSendViewport()
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
		m.seedSendLogFromTasks()
	case errMsg:
		m.errMsg = msg.err.Error()
		m.sending = false
		m.syncSendViewport()
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
		m.sending = false
		m.appendSendEntry("agent", msg.entry.Agent, msg.entry.Text)
		m.responses = append([]responseEntry{msg.entry}, m.responses...)
		m.responsesList.SetItems(buildResponseItems(m.responses))
		m.addLog("info", "response received from "+msg.entry.Agent)
		m.updateDetailForTab(tabHistory)
		return m, refreshAllCmd(m.caller)
	case agentResultMsg:
		// Handle individual agent result from multi-agent dispatch (non-streaming fallback)
		if msg.err != nil {
			m.appendSendEntry("error", msg.agentID, msg.err.Error())
			m.agentProgress[msg.agentID] = "failed"
			m.addLog("error", msg.agentID+": "+msg.err.Error())
		} else {
			m.appendSendEntry("agent", msg.agentID, msg.text)
			m.agentProgress[msg.agentID] = "completed"
			m.addLog("info", "response received from "+msg.agentID)
		}
		delete(m.activeAgents, msg.agentID)

		// Check if all agents are done
		if len(m.activeAgents) == 0 {
			m.sending = false
		}
		m.syncSendViewport()
		return m, nil
	case streamEventMsg:
		// Handle streaming events from agents
		event := msg.event
		switch event.Kind {
		case "output":
			m.appendStreamLine(msg.agentID, event.Text)
			m.syncSendViewport()
			m.sendViewport.GotoBottom() // Auto-scroll
		case "prompt":
			// Focus mode: first agent to ask gets focus
			if m.focusedAgent == "" {
				m.focusedAgent = msg.agentID
			} else if m.focusedAgent != msg.agentID {
				// Queue other agents waiting for input
				m.pendingPrompts = append(m.pendingPrompts, msg.agentID)
			}
			m.appendStreamLine(msg.agentID, event.Text)
			m.updateFocusIndicator()
			m.syncSendViewport()
			m.sendViewport.GotoBottom()
		case "complete":
			m.finishAgentStream(msg.agentID)
			// If this was focused agent, move to next in queue
			if m.focusedAgent == msg.agentID && len(m.pendingPrompts) > 0 {
				m.focusedAgent = m.pendingPrompts[0]
				m.pendingPrompts = m.pendingPrompts[1:]
				m.updateFocusIndicator()
			} else if m.focusedAgent == msg.agentID {
				m.focusedAgent = ""
				m.updateFocusIndicator()
			}
			m.syncSendViewport()
		case "error":
			m.appendSendEntry("error", msg.agentID, event.Text)
			m.finishAgentStream(msg.agentID)
			m.syncSendViewport()
		}
		return m, m.listenAllStreams()
	case refreshStartMsg:
		m.pendingRefresh += msg.count
		m.refreshing = m.pendingRefresh > 0
		return m, m.spinner.Tick
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.refreshing || m.sending {
			if m.sending {
				m.syncSendViewport()
			}
			return m, cmd
		}
		return m, nil
	case tickMsg:
		return m, tea.Batch(refreshAllCmd(m.caller), tickCmd())
	case tea.MouseMsg:
		// Handle mouse wheel scrolling in viewports
		if msg.Type == tea.MouseWheelUp || msg.Type == tea.MouseWheelDown {
			if m.showSendModal || m.activeTab == tabSend {
				var cmd tea.Cmd
				m.sendViewport, cmd = m.sendViewport.Update(msg)
				return m, cmd
			}
			if m.showLogs {
				var cmd tea.Cmd
				m.logViewport, cmd = m.logViewport.Update(msg)
				return m, cmd
			}
			if m.activeTab == tabAgents || m.activeTab == tabTasks || m.activeTab == tabHistory {
				var cmd tea.Cmd
				m.detailViewport, cmd = m.detailViewport.Update(msg)
				return m, cmd
			}
		}
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
				m.syncSendViewport()
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
		if key.Matches(msg, m.keys.Screen) {
			m.altScreen = !m.altScreen
			if m.altScreen {
				return m, tea.EnterAltScreen
			}
			return m, tea.ExitAltScreen
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
				m.syncSendViewport()
				return m, nil
			case "tab", "shift+tab":
				// Focus mode: switch between agents waiting for input
				if m.focusedAgent != "" && len(m.pendingPrompts) > 0 {
					// Move current to end of queue, take next
					m.pendingPrompts = append(m.pendingPrompts, m.focusedAgent)
					m.focusedAgent = m.pendingPrompts[0]
					m.pendingPrompts = m.pendingPrompts[1:]
					m.updateFocusIndicator()
					m.syncSendViewport()
					return m, nil
				}
				// Normal tab behavior: switch between agent input and message input
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
					return m, m.scrollSendViewport(msg)
				}
			case "pgup", "pgdown", "ctrl+u", "ctrl+d":
				return m, m.scrollSendViewport(msg)
			case "enter":
				// Focus mode: send input to focused agent
				if m.focusedAgent != "" {
					text := m.msgInput.Value()
					if text != "" {
						if stream, ok := m.streamChannels[m.focusedAgent]; ok && !stream.Done {
							stream.Input <- text
						}
						m.appendSendEntry("user-input", m.focusedAgent, text)
						m.msgInput.SetValue("")
						m.syncSendViewport()
						m.sendViewport.GotoBottom()
					}
					return m, nil
				}
				// Normal mode: start new send
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
				m.syncSendViewport()
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
					m.syncSendViewport()
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
				m.syncSendViewport()
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
			case "tab":
				// Move to next settings field
				m.settingsFocusIndex = (m.settingsFocusIndex + 1) % settingsFieldCount
				m.updateSettingsFieldFocus()
				return m, nil
			case "shift+tab":
				// Move to previous settings field
				m.settingsFocusIndex = (m.settingsFocusIndex + settingsFieldCount - 1) % settingsFieldCount
				m.updateSettingsFieldFocus()
				return m, nil
			case " ":
				// Toggle checkboxes if focused on them
				if m.settingsFocusIndex == settingsFieldClaudeContinue {
					m.claudeContinue = !m.claudeContinue
					if err := m.server.UpdateClaudeContinue(m.claudeContinue); err != nil {
						m.settingsMessage = "Failed to save: " + err.Error()
					} else {
						m.settingsMessage = fmt.Sprintf("Continue mode: %t", m.claudeContinue)
					}
					return m, nil
				}
				                if m.settingsFocusIndex == settingsFieldCodexSearch {
									m.codexSearch = !m.codexSearch
									if err := m.server.UpdateCodexSearch(m.codexSearch); err != nil {
										m.settingsMessage = "Failed to save: " + err.Error()
									} else {
										m.settingsMessage = fmt.Sprintf("Codex search: %t", m.codexSearch)
									}
									return m, nil
								}
								if m.settingsFocusIndex == settingsFieldGeminiSandbox {
									m.geminiSandbox = !m.geminiSandbox
									if err := m.server.UpdateGeminiSandbox(m.geminiSandbox); err != nil {
										m.settingsMessage = "Failed to save: " + err.Error()
									} else {
										m.settingsMessage = fmt.Sprintf("Gemini sandbox: %t", m.geminiSandbox)
									}
									return m, nil
								}
							case "enter":
								switch m.settingsFocusIndex {
				                // ...
								case settingsFieldGeminiModel:
									model := strings.TrimSpace(m.geminiModelInput.Value())
									if err := m.server.UpdateGeminiModel(model); err != nil {
										m.settingsMessage = "Failed to save: " + err.Error()
									} else if model == "" {
										m.settingsMessage = "Gemini model: default"
									} else {
										m.settingsMessage = "Gemini model: " + model
									}
								case settingsFieldGeminiSandbox:
									m.geminiSandbox = !m.geminiSandbox
									if err := m.server.UpdateGeminiSandbox(m.geminiSandbox); err != nil {
										m.settingsMessage = "Failed to save: " + err.Error()
									} else {
										m.settingsMessage = fmt.Sprintf("Gemini sandbox: %t", m.geminiSandbox)
									}
								case settingsFieldGeminiApproval:
									mode := strings.TrimSpace(m.geminiApprovalInput.Value())
									if mode != "" && mode != "default" && mode != "auto_edit" && mode != "yolo" {
										m.settingsMessage = "Invalid mode: use default, auto_edit, yolo, or blank"
										return m, nil
									}
									if err := m.server.UpdateGeminiApprovalMode(mode); err != nil {
										m.settingsMessage = "Failed to save: " + err.Error()
									} else if mode == "" {
										m.settingsMessage = "Gemini approval: default"
									} else {
										m.settingsMessage = "Gemini approval: " + mode
									}
								}
								return m, nil
							}
						}
						// Update the focused input
						var cmd tea.Cmd
						switch m.settingsFocusIndex {
						case settingsFieldOrchestrator:
							m.settingsInput, cmd = m.settingsInput.Update(msg)
						case settingsFieldClaudeModel:
							m.claudeModelInput, cmd = m.claudeModelInput.Update(msg)
						case settingsFieldClaudeTools:
							m.claudeToolsInput, cmd = m.claudeToolsInput.Update(msg)
						case settingsFieldCodexModel:
							m.codexModelInput, cmd = m.codexModelInput.Update(msg)
						case settingsFieldCodexProfile:
							m.codexProfileInput, cmd = m.codexProfileInput.Update(msg)
						case settingsFieldCodexSandbox:
							m.codexSandboxInput, cmd = m.codexSandboxInput.Update(msg)
						case settingsFieldCodexApproval:
							m.codexApprovalInput, cmd = m.codexApprovalInput.Update(msg)
						case settingsFieldGeminiModel:
							m.geminiModelInput, cmd = m.geminiModelInput.Update(msg)
						case settingsFieldGeminiApproval:
							m.geminiApprovalInput, cmd = m.geminiApprovalInput.Update(msg)
						}
						return m, cmd
					}
	if m.activeTab == tabSend {
		var cmd tea.Cmd
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+enter", "alt+enter", "ctrl+s":
				return m, m.startSend(m.agentInput.Value(), m.msgInput.Value())
			case "up", "down", "pgup", "pgdown", "ctrl+u", "ctrl+d":
				if m.focusIndex != 1 || strings.TrimSpace(m.msgInput.Value()) == "" {
					return m, m.scrollSendViewport(key)
				}
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
		m.syncSendViewport()
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
		m.syncSendViewport()
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
			m.syncSendViewport()
		}
		return refreshAllCmd(m.caller)
	case "help":
		m.showHelp = true
		return nil
	case "quit", "exit":
		return tea.Quit
	case "claude-model":
		if len(parts) >= 2 {
			model := strings.ToLower(parts[1])
			if model != "opus" && model != "sonnet" && model != "haiku" && model != "" {
				m.errMsg = "Invalid model. Use: opus, sonnet, haiku, or blank"
				return nil
			}
			if err := m.server.UpdateClaudeModel(model); err != nil {
				m.errMsg = "Failed to save: " + err.Error()
			} else if model == "" {
				m.settingsMessage = "Claude model: default"
			} else {
				m.settingsMessage = "Claude model: " + model
			}
			m.claudeModelInput.SetValue(model)
		} else {
			m.errMsg = "Usage: /claude-model <opus|sonnet|haiku>"
		}
		return nil
	case "claude-tools":
		if len(parts) >= 2 {
			profile := strings.ToLower(parts[1])
			if profile != "safe" && profile != "normal" && profile != "full" && profile != "" {
				m.errMsg = "Invalid profile. Use: safe, normal, full, or blank"
				return nil
			}
			if err := m.server.UpdateClaudeToolProfile(profile); err != nil {
				m.errMsg = "Failed to save: " + err.Error()
			} else if profile == "" {
				m.settingsMessage = "Claude tools: all (default)"
			} else {
				m.settingsMessage = "Claude tools: " + profile
			}
			m.claudeToolsInput.SetValue(profile)
		} else {
			m.errMsg = "Usage: /claude-tools <safe|normal|full>"
		}
		return nil
	case "claude-continue":
		m.claudeContinue = !m.claudeContinue
		if err := m.server.UpdateClaudeContinue(m.claudeContinue); err != nil {
			m.errMsg = "Failed to save: " + err.Error()
		} else {
			m.settingsMessage = fmt.Sprintf("Claude continue mode: %t", m.claudeContinue)
		}
		return nil
	case "codex-model":
		if len(parts) >= 2 {
			model := strings.TrimSpace(strings.Join(parts[1:], " "))
			if err := m.server.UpdateCodexModel(model); err != nil {
				m.errMsg = "Failed to save: " + err.Error()
			} else if model == "" {
				m.settingsMessage = "Codex model: default"
			} else {
				m.settingsMessage = "Codex model: " + model
			}
			m.codexModelInput.SetValue(model)
		} else {
			m.errMsg = "Usage: /codex-model <model>"
		}
		return nil
	case "codex-profile":
		if len(parts) >= 2 {
			profile := strings.TrimSpace(strings.Join(parts[1:], " "))
			if err := m.server.UpdateCodexProfile(profile); err != nil {
				m.errMsg = "Failed to save: " + err.Error()
			} else if profile == "" {
				m.settingsMessage = "Codex profile: default"
			} else {
				m.settingsMessage = "Codex profile: " + profile
			}
			m.codexProfileInput.SetValue(profile)
		} else {
			m.errMsg = "Usage: /codex-profile <profile>"
		}
		return nil
	case "codex-sandbox":
		if len(parts) >= 2 {
			mode := strings.TrimSpace(strings.Join(parts[1:], " "))
			if mode != "" && mode != "read-only" && mode != "workspace-write" && mode != "danger-full-access" {
				m.errMsg = "Invalid sandbox. Use: read-only, workspace-write, danger-full-access, or blank"
				return nil
			}
			if err := m.server.UpdateCodexSandbox(mode); err != nil {
				m.errMsg = "Failed to save: " + err.Error()
			} else if mode == "" {
				m.settingsMessage = "Codex sandbox: default"
			} else {
				m.settingsMessage = "Codex sandbox: " + mode
			}
			m.codexSandboxInput.SetValue(mode)
		} else {
			m.errMsg = "Usage: /codex-sandbox <read-only|workspace-write|danger-full-access>"
		}
		return nil
	case "codex-approval":
		if len(parts) >= 2 {
			policy := strings.TrimSpace(strings.Join(parts[1:], " "))
			if policy != "" && policy != "untrusted" && policy != "on-failure" && policy != "on-request" && policy != "never" {
				m.errMsg = "Invalid approval. Use: untrusted, on-failure, on-request, never, or blank"
				return nil
			}
			if err := m.server.UpdateCodexApprovalPolicy(policy); err != nil {
				m.errMsg = "Failed to save: " + err.Error()
			} else if policy == "" {
				m.settingsMessage = "Codex approval: default"
			} else {
				m.settingsMessage = "Codex approval: " + policy
			}
			m.codexApprovalInput.SetValue(policy)
		} else {
			m.errMsg = "Usage: /codex-approval <untrusted|on-failure|on-request|never>"
		}
		return nil
	case "codex-search":
		m.codexSearch = !m.codexSearch
		if err := m.server.UpdateCodexSearch(m.codexSearch); err != nil {
			m.errMsg = "Failed to save: " + err.Error()
		} else {
			m.settingsMessage = fmt.Sprintf("Codex search: %t", m.codexSearch)
		}
		return nil
	case "gemini-model":
		if len(parts) >= 2 {
			model := strings.TrimSpace(strings.Join(parts[1:], " "))
			if err := m.server.UpdateGeminiModel(model); err != nil {
				m.errMsg = "Failed to save: " + err.Error()
			} else if model == "" {
				m.settingsMessage = "Gemini model: default"
			} else {
				m.settingsMessage = "Gemini model: " + model
			}
			m.geminiModelInput.SetValue(model)
		} else {
			m.errMsg = "Usage: /gemini-model <model>"
		}
		return nil
	case "gemini-resume":
		if len(parts) >= 2 {
			sessionID := strings.TrimSpace(parts[1])
			if err := m.server.UpdateGeminiResume(sessionID); err != nil {
				m.errMsg = "Failed to save: " + err.Error()
			} else {
				m.settingsMessage = "Gemini session resumed: " + sessionID
			}
		} else {
			m.errMsg = "Usage: /gemini-resume <id>"
		}
		return nil
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
	// Claude settings commands
	{Name: "claude-model", Usage: "/claude-model <opus|sonnet|haiku>", Description: "set Claude model"},
	{Name: "claude-tools", Usage: "/claude-tools <safe|normal|full>", Description: "set Claude tool profile"},
	{Name: "claude-continue", Usage: "/claude-continue", Description: "toggle Claude continue mode"},
	// Codex settings commands
	{Name: "codex-model", Usage: "/codex-model <model>", Description: "set Codex model"},
	{Name: "codex-profile", Usage: "/codex-profile <profile>", Description: "set Codex config profile"},
	{Name: "codex-sandbox", Usage: "/codex-sandbox <mode>", Description: "set Codex sandbox mode"},
	{Name: "codex-approval", Usage: "/codex-approval <policy>", Description: "set Codex approval policy"},
	{Name: "codex-search", Usage: "/codex-search", Description: "toggle Codex web search"},
	// Gemini settings commands
	{Name: "gemini-model", Usage: "/gemini-model <model>", Description: "set Gemini model"},
	{Name: "gemini-resume", Usage: "/gemini-resume <id>", Description: "resume a Gemini session"},
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
	inputWidth, msgHeight, logHeight, activityHeight := sendViewLayout(width, height)
	m.agentInput.Width = inputWidth
	// Account for border width when setting textarea dimensions
	m.msgInput.SetWidth(inputWidth - 2)
	m.msgInput.SetHeight(msgHeight)
	log := m.renderSendLog(inputWidth, logHeight)
	activity := m.renderTaskActivity(inputWidth, activityHeight)
	separator := dimStyle.Render(strings.Repeat("─", inputWidth))
	msgBox := msgBoxStyle.Width(inputWidth).Render(m.msgInput.View())
	lines := []string{
		"Agent:",
		m.agentInput.View(),
		log,
		separator,
		"Activity:",
		activity,
		"Message:",
		msgBox,
		"enter to send, shift+enter for newline, up/down scroll log, esc to edit agent",
	}
	return strings.Join(lines, "\n")
}

func (m model) renderSendModal() string {
	width, height := modalSize(m.width, m.height)

	inputWidth, msgHeight, logHeight, activityHeight := sendModalLayout(width, height)
	m.agentInput.Width = inputWidth
	// Account for border width when setting textarea dimensions
	m.msgInput.SetWidth(inputWidth - 2)
	m.msgInput.SetHeight(msgHeight)
	log := m.renderSendLog(inputWidth, logHeight)
	activity := m.renderTaskActivity(inputWidth, activityHeight)
	separator := dimStyle.Render(strings.Repeat("─", inputWidth))
	msgBox := msgBoxStyle.Width(inputWidth).Render(m.msgInput.View())

	title := headerStyle.Render("Send Message")
	body := strings.Join([]string{
		"Agent:",
		m.agentInput.View(),
		log,
		separator,
		"Activity:",
		activity,
		"Message:",
		msgBox,
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
	// Minimal padding: just 1 unit for border on each side
	return width - 2, height - 2
}

func sendViewLayout(width, height int) (int, int, int, int) {
	if width <= 0 {
		width = 80
	}
	if width < 30 {
		width = 30
	}
	if height <= 0 {
		height = 24
	}
	inputWidth := width - 4
	if inputWidth < 20 {
		inputWidth = 20
	}
	msgHeight := 6
	if height > 30 {
		msgHeight = 8
	}
	availableHeight := height - (msgHeight + 6)
	logHeight, activityHeight := splitSendHeights(availableHeight)
	return inputWidth, msgHeight, logHeight, activityHeight
}

func sendModalLayout(width, height int) (int, int, int, int) {
	inputWidth := width - 6
	if inputWidth < 20 {
		inputWidth = 20
	}
	msgHeight := 5
	if height >= 20 {
		msgHeight = 6
	}
	contentHeight := height - 4
	availableHeight := contentHeight - (msgHeight + 6)
	logHeight, activityHeight := splitSendHeights(availableHeight)
	return inputWidth, msgHeight, logHeight, activityHeight
}

func splitSendHeights(available int) (int, int) {
	if available < 2 {
		available = 2
	}
	minLog := 3
	minActivity := 2
	if available < minLog+minActivity {
		logHeight := available / 2
		if logHeight < 1 {
			logHeight = 1
		}
		activityHeight := available - logHeight
		if activityHeight < 1 {
			activityHeight = 1
		}
		return logHeight, activityHeight
	}
	// Give more space to log, less to activity
	activityHeight := available / 4
	if activityHeight < minActivity {
		activityHeight = minActivity
	}
	logHeight := available - activityHeight
	if logHeight < minLog {
		logHeight = minLog
		activityHeight = available - logHeight
	}
	return logHeight, activityHeight
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

	// Focus indicators
	orchIndicator := "  "
	modelIndicator := "  "
	toolsIndicator := "  "
	contIndicator := "  "
	codexModelIndicator := "  "
	codexProfileIndicator := "  "
	codexSandboxIndicator := "  "
	codexApprovalIndicator := "  "
	codexSearchIndicator := "  "
	geminiModelIndicator := "  "
	geminiSandboxIndicator := "  "
	geminiApprovalIndicator := "  "
	switch m.settingsFocusIndex {
	case settingsFieldOrchestrator:
		orchIndicator = "> "
	case settingsFieldClaudeModel:
		modelIndicator = "> "
	case settingsFieldClaudeTools:
		toolsIndicator = "> "
	case settingsFieldClaudeContinue:
		contIndicator = "> "
	case settingsFieldCodexModel:
		codexModelIndicator = "> "
	case settingsFieldCodexProfile:
		codexProfileIndicator = "> "
	case settingsFieldCodexSandbox:
		codexSandboxIndicator = "> "
	case settingsFieldCodexApproval:
		codexApprovalIndicator = "> "
	case settingsFieldCodexSearch:
		codexSearchIndicator = "> "
	case settingsFieldGeminiModel:
		geminiModelIndicator = "> "
	case settingsFieldGeminiSandbox:
		geminiSandboxIndicator = "> "
	case settingsFieldGeminiApproval:
		geminiApprovalIndicator = "> "
	}

	// Continue mode checkbox
	continueCheck := "[ ]"
	if m.claudeContinue {
		continueCheck = "[x]"
	}

	codexSearchCheck := "[ ]"
	if m.codexSearch {
		codexSearchCheck = "[x]"
	}

	geminiSandboxCheck := "[ ]"
	if m.geminiSandbox {
		geminiSandboxCheck = "[x]"
	}

	lines := []string{
		headerStyle.Render("Runtime Settings"),
		"",
		fmt.Sprintf("Data dir: %s", m.server.Config().DataDir),
		fmt.Sprintf("Socket: %s (enabled: %t)", m.server.Config().Socket.Path, m.server.Config().Socket.Enabled),
		fmt.Sprintf("HTTP: %s:%d (enabled: %t)", m.server.Config().HTTP.Host, m.server.Config().HTTP.Port, m.server.Config().HTTP.Enabled),
		"",
		headerStyle.Render("Orchestrator"),
		orchIndicator + "Delegates (comma-separated):",
		"  " + m.settingsInput.View(),
		fmt.Sprintf("  Current: %s", currentDelegates),
		"",
		headerStyle.Render("Claude Settings"),
		modelIndicator + "Model:",
		"  " + m.claudeModelInput.View(),
		dimStyle.Render("  Options: opus, sonnet, haiku (blank = default)"),
		toolsIndicator + "Tool Profile:",
		"  " + m.claudeToolsInput.View(),
		dimStyle.Render("  Options: safe (read-only), normal, full (blank = all tools)"),
		contIndicator + "Continue Mode: " + continueCheck,
		dimStyle.Render("  Resume previous conversation context"),
		"",
		headerStyle.Render("Codex Settings"),
		codexModelIndicator + "Model:",
		"  " + m.codexModelInput.View(),
		dimStyle.Render("  Any model id (blank = default)"),
		codexProfileIndicator + "Profile:",
		"  " + m.codexProfileInput.View(),
		dimStyle.Render("  Config profile from config.toml (blank = default)"),
		codexSandboxIndicator + "Sandbox:",
		"  " + m.codexSandboxInput.View(),
		dimStyle.Render("  read-only, workspace-write, danger-full-access (blank = default)"),
		codexApprovalIndicator + "Approval Policy:",
		"  " + m.codexApprovalInput.View(),
		dimStyle.Render("  untrusted, on-failure, on-request, never (blank = default)"),
		codexSearchIndicator + "Web Search: " + codexSearchCheck,
		dimStyle.Render("  Enable web_search tool"),
		"",
		headerStyle.Render("Gemini Settings"),
		geminiModelIndicator + "Model:",
		"  " + m.geminiModelInput.View(),
		dimStyle.Render("  gemini-1.5-pro, gemini-1.5-flash, gemini-2.0-flash (blank = default)"),
		geminiSandboxIndicator + "Sandbox: " + geminiSandboxCheck,
		dimStyle.Render("  Run in sandbox"),
		geminiApprovalIndicator + "Approval Mode:",
		"  " + m.geminiApprovalInput.View(),
		dimStyle.Render("  default, auto_edit, yolo (blank = default)"),
		"",
		dimStyle.Render("Tab/Shift+Tab to navigate, Enter to apply, Space to toggle"),
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
				m.syncSendViewport()
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
		m.updateSettingsFieldFocus()
		return
	}
	m.settingsInput.Blur()
	m.claudeModelInput.Blur()
	m.claudeToolsInput.Blur()
	m.codexModelInput.Blur()
	m.codexProfileInput.Blur()
	m.codexSandboxInput.Blur()
	m.codexApprovalInput.Blur()
	m.geminiModelInput.Blur()
	m.geminiApprovalInput.Blur()
}

func (m *model) updateSettingsFieldFocus() {
	// Blur all fields first
	m.settingsInput.Blur()
	m.claudeModelInput.Blur()
	m.claudeToolsInput.Blur()
	m.codexModelInput.Blur()
	m.codexProfileInput.Blur()
	m.codexSandboxInput.Blur()
	m.codexApprovalInput.Blur()
	m.geminiModelInput.Blur()
	m.geminiApprovalInput.Blur()

	// Focus the selected field
	switch m.settingsFocusIndex {
	case settingsFieldOrchestrator:
		m.settingsInput.Focus()
	case settingsFieldClaudeModel:
		m.claudeModelInput.Focus()
	case settingsFieldClaudeTools:
		m.claudeToolsInput.Focus()
	case settingsFieldCodexModel:
		m.codexModelInput.Focus()
	case settingsFieldCodexProfile:
		m.codexProfileInput.Focus()
	case settingsFieldCodexSandbox:
		m.codexSandboxInput.Focus()
	case settingsFieldCodexApproval:
		m.codexApprovalInput.Focus()
	case settingsFieldGeminiModel:
		m.geminiModelInput.Focus()
	case settingsFieldGeminiApproval:
		m.geminiApprovalInput.Focus()
		// checkbox fields don't get focus
	}
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
	if message == "" {
		return nil
	}

	// Check for @agent mentions in the message
	mentions := parseMentions(message)
	if len(mentions) > 0 {
		return m.startMultiAgentSend(mentions)
	}

	// Single agent flow - use streaming
	if agent == "" {
		return nil
	}
	m.errMsg = ""
	m.lastResponse = ""
	m.sending = true
	m.server.UpdateLastAgent(agent)
	m.appendSendEntry("user", agent, message)
	m.msgInput.SetValue("")
	m.msgInput.CursorEnd()

	// Clear previous streaming state
	m.streamChannels = make(map[string]*AgentStream)
	m.streamBuffer = make(map[string][]string)
	m.focusedAgent = ""
	m.pendingPrompts = []string{}

	// Create stream channels for this agent
	stream := &AgentStream{
		Output: make(chan types.StreamEvent, 100),
		Input:  make(chan string, 10),
		Done:   false,
	}
	m.streamChannels[agent] = stream

	// Start streaming execution in background
	return tea.Batch(
		m.spinner.Tick,
		startStreamingCmd(m.server, agent, message, stream),
		listenAgentStream(agent, stream.Output),
	)
}

// startMultiAgentSend dispatches tasks to multiple agents concurrently with streaming
func (m *model) startMultiAgentSend(mentions map[string]string) tea.Cmd {
	m.errMsg = ""
	m.lastResponse = ""
	m.sending = true

	// Clear and set up tracking
	m.activeAgents = make(map[string]string)
	m.agentProgress = make(map[string]string)
	m.streamChannels = make(map[string]*AgentStream)
	m.streamBuffer = make(map[string][]string)
	m.focusedAgent = ""
	m.pendingPrompts = []string{}

	// Build list of agent names for display
	var agentNames []string
	for agentID, task := range mentions {
		m.activeAgents[agentID] = task
		m.agentProgress[agentID] = "working"
		agentNames = append(agentNames, agentID)
	}

	// Append user message summary to log
	m.appendSendEntry("user", strings.Join(agentNames, ", "), formatMentionsSummary(mentions))
	m.msgInput.SetValue("")
	m.msgInput.CursorEnd()

	// Create batch of commands - one per agent with streaming
	cmds := []tea.Cmd{m.spinner.Tick}
	for agentID, task := range mentions {
		stream := &AgentStream{
			Output: make(chan types.StreamEvent, 100),
			Input:  make(chan string, 10),
			Done:   false,
		}
		m.streamChannels[agentID] = stream
		cmds = append(cmds, startStreamingCmd(m.server, agentID, task, stream))
		cmds = append(cmds, listenAgentStream(agentID, stream.Output))
	}
	return tea.Batch(cmds...)
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
	m.syncSendViewport()
}

// appendStreamLine adds a line to an agent's streaming buffer and updates the display
func (m *model) appendStreamLine(agentID, text string) {
	if m.streamBuffer == nil {
		m.streamBuffer = make(map[string][]string)
	}
	m.streamBuffer[agentID] = append(m.streamBuffer[agentID], text)
}

// finishAgentStream marks an agent's stream as done and consolidates output
func (m *model) finishAgentStream(agentID string) {
	if stream, ok := m.streamChannels[agentID]; ok {
		stream.Done = true
	}
	// Consolidate buffer into a single send entry
	if lines, ok := m.streamBuffer[agentID]; ok && len(lines) > 0 {
		text := strings.Join(lines, "\n")
		m.appendSendEntry("agent", agentID, text)
		delete(m.streamBuffer, agentID)
	}
	delete(m.activeAgents, agentID)
	m.agentProgress[agentID] = "completed"

	// Check if all agents are done
	allDone := true
	for _, stream := range m.streamChannels {
		if !stream.Done {
			allDone = false
			break
		}
	}
	if allDone {
		m.sending = false
	}
}

// updateFocusIndicator updates the agent input to show which agent has focus
func (m *model) updateFocusIndicator() {
	if m.focusedAgent != "" {
		m.agentInput.SetValue(m.focusedAgent + " (responding)")
	} else if len(m.streamChannels) > 0 {
		// Show the first active streaming agent
		for agentID := range m.streamChannels {
			m.agentInput.SetValue(agentID)
			break
		}
	}
}

func (m model) renderSendLog(width, height int) string {
	if height <= 0 {
		return ""
	}
	return m.sendViewport.View()
}

func (m model) renderTaskActivity(width, height int) string {
	if height <= 0 {
		return ""
	}
	if width <= 0 {
		width = 20
	}
	tasks := append([]types.Task{}, m.tasks...)
	if len(tasks) == 0 {
		return padLines([]string{dimStyle.Render("No tasks yet.")}, height)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Status.Timestamp > tasks[j].Status.Timestamp
	})
	wrapWidth := width - 2
	if wrapWidth < 1 {
		wrapWidth = width
	}
	lines := make([]string, 0, height)
	for _, task := range tasks {
		if len(lines) >= height {
			break
		}
		agent := taskTargetAgent(task)
		if agent == "" {
			agent = "unknown"
		}
		label := fmt.Sprintf("%s  %s  %s", task.Status.State, agent, shortTaskID(task.ID))
		wrapped := ansi.Wrap(label, wrapWidth, "")
		for _, line := range strings.Split(wrapped, "\n") {
			if len(lines) >= height {
				break
			}
			lines = append(lines, line)
		}
	}
	return padLines(lines, height)
}

func taskTargetAgent(task types.Task) string {
	if task.Metadata != nil {
		if value, ok := task.Metadata["targetAgent"].(string); ok && value != "" {
			return value
		}
	}
	for _, msg := range task.History {
		if msg.Metadata == nil {
			continue
		}
		if value, ok := msg.Metadata["targetAgent"].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func shortTaskID(id string) string {
	if len(id) <= 16 {
		return id
	}
	return id[:6] + "..." + id[len(id)-6:]
}

func padLines(lines []string, height int) string {
	if len(lines) < height {
		pad := make([]string, height-len(lines))
		lines = append(lines, pad...)
	} else if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
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
		case "user-input":
			// User input during streaming
			label = "You (input)"
			if entry.Agent != "" {
				label = fmt.Sprintf("You -> %s (input)", entry.Agent)
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

	// Show streaming output from active agents
	if m.sending && len(m.streamBuffer) > 0 {
		for agentID, buffer := range m.streamBuffer {
			if len(buffer) == 0 {
				continue
			}
			// Show agent header with focus indicator
			focusIndicator := ""
			if m.focusedAgent == agentID {
				focusIndicator = " ● FOCUS"
			} else if contains(m.pendingPrompts, agentID) {
				focusIndicator = " ⏳ waiting"
			} else {
				focusIndicator = " ↓ streaming"
			}
			lines = append(lines, headerStyle.Render(agentID+focusIndicator))

			// Show buffered lines
			for _, line := range buffer {
				wrapped := ansi.Wrap(line, wrapWidth, "")
				for _, wrappedLine := range strings.Split(wrapped, "\n") {
					lines = append(lines, "  "+wrappedLine)
				}
			}
			lines = append(lines, "")
		}
	}

	if m.sending {
		if len(m.streamChannels) > 0 {
			// Streaming mode: show active agents
			activeCount := 0
			for _, stream := range m.streamChannels {
				if !stream.Done {
					activeCount++
				}
			}
			if activeCount > 0 {
				lines = append(lines, dimStyle.Render(fmt.Sprintf("%s %d agent(s) active", m.spinner.View(), activeCount)))
			}
		} else if len(m.activeAgents) > 0 {
			// Multi-agent mode (non-streaming fallback)
			lines = append(lines, dimStyle.Render("Working:"))
			for agentID := range m.activeAgents {
				status := m.agentProgress[agentID]
				lines = append(lines, dimStyle.Render(fmt.Sprintf("  %s %s: %s", m.spinner.View(), agentID, status)))
			}
		} else {
			// Single agent mode
			lines = append(lines, dimStyle.Render("Waiting for response "+m.spinner.View()))
		}
	}
	if len(lines) > 0 && strings.TrimSpace(stripANSI(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// contains checks if a string slice contains a value
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func (m *model) sendLogLayout() (int, int) {
	if m.showSendModal {
		width, height := modalSize(m.width, m.height)
		inputWidth, _, logHeight, _ := sendModalLayout(width, height)
		return inputWidth, logHeight
	}
	width, height := m.bodySize()
	inputWidth, _, logHeight, _ := sendViewLayout(width, height)
	return inputWidth, logHeight
}

func (m *model) syncSendViewport() {
	if !m.showSendModal && m.activeTab != tabSend {
		return
	}
	width, height := m.sendLogLayout()
	if width <= 0 || height <= 0 {
		return
	}
	wrapWidth := width - 2
	if wrapWidth < 1 {
		wrapWidth = width
	}
	lines := m.sendLogLines(wrapWidth)
	if len(lines) == 0 {
		lines = []string{dimStyle.Render("No messages yet.")}
	}
	atBottom := m.sendViewport.AtBottom()
	m.sendViewport.Width = width
	m.sendViewport.Height = height
	m.sendViewport.SetContent(strings.Join(lines, "\n"))
	m.sendViewport.SetYOffset(m.sendViewport.YOffset)

	// Auto-scroll to bottom when streaming or if already at bottom
	if atBottom || m.sending || m.focusedAgent != "" || len(m.streamBuffer) > 0 {
		m.sendViewport.GotoBottom()
	}
}

func (m *model) scrollSendViewport(msg tea.KeyMsg) tea.Cmd {
	m.syncSendViewport()
	var cmd tea.Cmd
	m.sendViewport, cmd = m.sendViewport.Update(msg)
	return cmd
}

func (m *model) seedSendLogFromTasks() {
	if m.sendLogSeeded {
		return
	}
	m.sendLogSeeded = true
	if len(m.tasks) == 0 {
		return
	}
	tasks := append([]types.Task{}, m.tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return taskStatusTime(tasks[i]).Before(taskStatusTime(tasks[j]))
	})
	entries := make([]sendEntry, 0, len(tasks)*2)
	for _, task := range tasks {
		statusTime := taskStatusTime(task)
		if !statusTime.IsZero() && statusTime.After(m.sessionStart) {
			continue
		}
		entries = append(entries, sendEntriesFromTask(task)...)
	}
	if len(entries) == 0 {
		return
	}
	m.sendLog = append(entries, m.sendLog...)
	m.syncSendViewport()
}

func sendEntriesFromTask(task types.Task) []sendEntry {
	agent := taskTargetAgent(task)
	if agent == "" {
		agent = "unknown"
	}
	entries := []sendEntry{}
	for _, msg := range task.History {
		if msg.Role != "user" {
			continue
		}
		text := messageText(msg)
		if text == "" {
			continue
		}
		entries = append(entries, sendEntry{
			Role:      "user",
			Agent:     agent,
			Text:      text,
			Timestamp: task.Status.Timestamp,
		})
		break
	}
	responseText := strings.TrimSpace(extractTaskText(task))
	if responseText != "" {
		entries = append(entries, sendEntry{
			Role:      "agent",
			Agent:     agent,
			Text:      responseText,
			Timestamp: task.Status.Timestamp,
		})
	}
	return entries
}

func messageText(msg types.Message) string {
	parts := make([]string, 0, len(msg.Parts))
	for _, part := range msg.Parts {
		if part.Kind == "text" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func taskStatusTime(task types.Task) time.Time {
	if task.Status.Timestamp == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, task.Status.Timestamp)
	if err != nil {
		return time.Time{}
	}
	return parsed
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

// parseMentions parses @agent mentions from text
// Single agent: "@vibe say something to @gemini" -> {"vibe": "say something to @gemini"}
// Multi-agent: "@claude write API, @gemini write UI" -> {"claude": "write API", "gemini": "write UI"}
// Multi-agent: "@claude task1 and @gemini task2" -> {"claude": "task1", "gemini": "task2"}
func parseMentions(text string) map[string]string {
	text = strings.TrimSpace(text)
	result := make(map[string]string)

	// First, check if this is a simple single-agent message: @agent <message>
	singlePattern := regexp.MustCompile(`^@(\w+)\s+(.+)$`)
	if match := singlePattern.FindStringSubmatch(text); len(match) == 3 {
		agentID := strings.ToLower(match[1])
		task := strings.TrimSpace(match[2])
		// Check if task contains other @mentions with their own tasks (multi-agent pattern)
		if !containsValidMultiMention(task) {
			result[agentID] = task
			return result
		}
	}

	// Multi-agent: split by comma or " and "
	// Pattern: @agent task, @agent2 task2
	parts := splitMentionsByDelimiters(text)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if match := regexp.MustCompile(`^@(\w+)\s+(.+)$`).FindStringSubmatch(part); len(match) == 3 {
			result[strings.ToLower(match[1])] = strings.TrimSpace(match[2])
		}
	}
	return result
}

// containsValidMultiMention checks if text has pattern like ", @agent task" or " and @agent task"
func containsValidMultiMention(text string) bool {
	// Look for ", @word word+" or " and @word word+"
	pattern := regexp.MustCompile(`(?:,\s*|\s+and\s+)@\w+\s+\S`)
	return pattern.MatchString(text)
}

// splitMentionsByDelimiters splits text on ", @" or " and @" while keeping the @
func splitMentionsByDelimiters(text string) []string {
	// Replace ", @" and " and @" with a unique delimiter, keeping the @
	text = regexp.MustCompile(`,\s*@`).ReplaceAllString(text, "\x00@")
	text = regexp.MustCompile(`\s+and\s+@`).ReplaceAllString(text, "\x00@")
	return strings.Split(text, "\x00")
}

// formatMentionsSummary creates a display summary of multi-agent tasks
func formatMentionsSummary(mentions map[string]string) string {
	var parts []string
	for agentID, task := range mentions {
		parts = append(parts, fmt.Sprintf("@%s: %s", agentID, task))
	}
	return strings.Join(parts, "\n")
}

// sendToAgentCmd creates a command that sends a task to a specific agent (non-streaming fallback)
func sendToAgentCmd(caller *hub.LocalCaller, agentID, taskText string) tea.Cmd {
	return func() tea.Msg {
		msg := types.Message{
			Kind:      "message",
			MessageID: utils.NewID("msg"),
			Role:      "user",
			Parts:     []types.Part{{Kind: "text", Text: taskText}},
			Metadata:  map[string]any{"targetAgent": agentID},
		}
		params, _ := json.Marshal(map[string]any{
			"message":       msg,
			"configuration": map[string]any{"historyLength": 10},
		})
		resp, err := caller.Call(context.Background(), "message/send", params)
		if err != nil {
			return agentResultMsg{agentID: agentID, err: err}
		}
		if resp.Error != nil {
			return agentResultMsg{agentID: agentID, err: fmt.Errorf(resp.Error.Message)}
		}
		var task types.Task
		if err := decodeResult(resp.Result, &task); err != nil {
			return agentResultMsg{agentID: agentID, err: err}
		}
		return agentResultMsg{agentID: agentID, text: extractTaskText(task)}
	}
}

// startStreamingCmd starts a streaming execution for an agent
func startStreamingCmd(server *hub.Server, agentID, message string, stream *AgentStream) tea.Cmd {
	return func() tea.Msg {
		info, ok := server.Registry().Get(agentID)
		if !ok {
			stream.Output <- types.StreamEvent{Kind: "error", Text: "agent not found", AgentID: agentID, Timestamp: time.Now().UTC()}
			close(stream.Output)
			return nil
		}

		workingDir, _ := os.Getwd()
		ctx := types.ExecutionContext{
			TaskID:      utils.NewID("task"),
			ContextID:   utils.NewID("ctx"),
			UserMessage: types.Message{Kind: "message", Role: "user", Parts: []types.Part{{Kind: "text", Text: message}}},
			WorkingDir:  workingDir,
		}

		// Check if agent supports streaming
		if streamer, ok := info.Agent.(types.StreamingExecutor); ok {
			go func() {
				defer close(stream.Output)
				_ = streamer.ExecuteStreaming(ctx, stream.Output, stream.Input)
			}()
		} else {
			// Fallback: run non-streaming and emit single result
			go func() {
				defer close(stream.Output)
				result, err := info.Agent.Execute(ctx)
				if err != nil {
					stream.Output <- types.StreamEvent{Kind: "error", Text: err.Error(), AgentID: agentID, Timestamp: time.Now().UTC()}
				} else {
					text := extractTaskText(result.Task)
					stream.Output <- types.StreamEvent{Kind: "output", Text: text, AgentID: agentID, Timestamp: time.Now().UTC()}
					stream.Output <- types.StreamEvent{Kind: "complete", AgentID: agentID, Timestamp: time.Now().UTC()}
				}
			}()
		}
		return nil
	}
}

// listenAgentStream listens for events from an agent's output channel
func listenAgentStream(agentID string, ch <-chan types.StreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return streamEventMsg{agentID: agentID, event: types.StreamEvent{Kind: "complete", AgentID: agentID}}
		}
		return streamEventMsg{agentID: agentID, event: event}
	}
}

// listenAllStreams returns a batch command to listen on all active streams
func (m *model) listenAllStreams() tea.Cmd {
	var cmds []tea.Cmd
	for agentID, stream := range m.streamChannels {
		if !stream.Done {
			cmds = append(cmds, listenAgentStream(agentID, stream.Output))
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}
