package ui

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	psutil "github.com/shirou/gopsutil/v3/cpu"
	psmem "github.com/shirou/gopsutil/v3/mem"

	"hasher/internal/analyzer"
	"hasher/internal/client"
	"hasher/internal/hasher"
)

// View states
const (
	MainMenuView = iota
	ChatView
	ProgressView
	LogView
)

// Styles
var (
	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#FFFF00")).
			Padding(0, 2).
			Bold(true).
			Width(80)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#4B5563")).
			Padding(0, 2).
			Width(80)

	chatViewStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#9CA3AF"))

	logViewStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#9CA3AF"))

	userMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#60A5FA")).
				Bold(true)

	llmMessageStyle = lipgloss.NewStyle()

	// Text selection and highlighting styles
	highlightStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#3B82F6")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)

	copyNoticeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#10B981")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 2).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			Italic(true)

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#2563EB")).
			Padding(0, 1)

	listStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#2563EB"))

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#2563EB")).
				Bold(true)

	progressStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#34D399")).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA"))

	logoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00")).
			Bold(true).
			MarginTop(1)

	// Scrollbar styles
	scrollbarTrackStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#374151")).
				Width(1)

	scrollbarThumbStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#6B7280")).
				Width(1)

	scrollbarThumbHoverStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("#9CA3AF")).
					Width(1)
)

// ASCII art logo for HASHER
const hasherLogo = `
██╗  ██╗ █████╗ ███████╗██╗  ██╗███████╗██████╗
██║  ██║██╔══██╗██╔════╝██║  ██║██╔════╝██╔══██╗
███████║███████║███████╗███████║█████╗  ██████╔╝
██╔══██║██╔══██║╚════██║██╔══██║██╔══╝  ██╔══██╗
██║  ██║██║  ██║███████║██║  ██║███████╗██║  ██║
╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝`

// Menu item definitions
type menuItem struct {
	title       string
	description string
	view        int
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.description }
func (i menuItem) FilterValue() string { return i.title }

var menuItems = []list.Item{
	menuItem{
		title:       "1. Discovery",
		description: "Discover ASIC devices on the network",
		view:        MainMenuView,
	},
	menuItem{
		title:       "2. Probe",
		description: "Probe connected ASIC device",
		view:        MainMenuView,
	},
	menuItem{
		title:       "3. Protocol",
		description: "Detect ASIC device protocol",
		view:        MainMenuView,
	},
	menuItem{
		title:       "4. Provision",
		description: "Deploy hasher-server to ASIC device",
		view:        MainMenuView,
	},
	menuItem{
		title:       "5. Troubleshoot",
		description: "Run troubleshooting diagnostics",
		view:        MainMenuView,
	},
	menuItem{
		title:       "6. Configure",
		description: "Configure hasher inference service",
		view:        MainMenuView,
	},
	menuItem{
		title:       "7. Rules",
		description: "Manage logical validation rules",
		view:        MainMenuView,
	},
	menuItem{
		title:       "8. Test",
		description: "Test ASIC communication pattern",
		view:        MainMenuView,
	},
	menuItem{
		title:       "9. Chat",
		description: "Test hasher validation service",
		view:        ChatView,
	},
	menuItem{
		title:       "0. Quit",
		description: "Exit the application",
		view:        MainMenuView,
	},
}

// Model represents the application state
type Model struct {
	CurrentView    int
	MainMenu       list.Model
	ChatView       textarea.Model
	LogView        textarea.Model
	InitView       viewport.Model // Initialization logs view (using viewport for scrolling)
	Input          textarea.Model
	ServerCmd      *exec.Cmd
	ServerLogs     []string
	ChatHistory    []string
	ServerReady    bool
	ServerStarting bool // true when hasher-host process is running but not ready yet
	ResourceData   string
	Width          int
	Height         int
	ProgressText   string
	ProgressStatus string
	Deployer       *analyzer.Deployer
	DeviceIP       string            // Connected ASIC device IP (empty if none)
	DeviceType     string            // Type of connected device
	CryptoEnabled  bool              // Whether crypto-transformer is enabled
	APIClient      *client.APIClient // API client for hasher-host

	// Text selection fields
	SelectedText    string // Currently selected text
	ShowCopyNotice  bool   // Whether to show "copied to clipboard" notice
	CopyNoticeTimer int    // Timer for hiding copy notice
	SelectionMode   bool   // Whether we're in text selection mode
	ActiveView      string // Which view is active for selection: "chat" or "log"

	// Viewport content for scrolling
	ChatContent string
	LogContent  string
	InitContent string // Initialization log content
}

// NewModel creates a new UI model
func NewModel() Model {
	// Default dimensions
	defaultWidth := 80
	defaultHeight := 24
	menuHeight := defaultHeight - 13
	if menuHeight < 6 {
		menuHeight = 6
	}

	// Initialize menu with proper initial size
	menuList := list.New(menuItems, list.NewDefaultDelegate(), defaultWidth-4, menuHeight)
	menuList.Title = "Hasher CLI - Main Menu"
	menuList.SetShowStatusBar(false)
	menuList.SetFilteringEnabled(false)

	// Initialize chat view as textarea for text selection support
	chatView := textarea.New()
	chatView.SetWidth(77)
	chatView.SetHeight(10)
	chatView.SetValue("")
	chatView.Focus()
	chatView.Prompt = ""
	chatView.ShowLineNumbers = false
	chatView.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#2563EB"))
	chatView.Blur()

	// Initialize chat viewport for scrolling
	chatViewport := viewport.New(defaultWidth-4, 10)
	chatViewport.Style = chatViewStyle
	chatViewport.SetContent("Welcome to Hasher CLI!\n\nType your message below for hasher-based inference.")

	// Initialize log view as textarea for text selection support
	logView := textarea.New()
	logView.SetWidth(77)
	logView.SetHeight(8)
	logView.SetValue("")
	logView.Prompt = ""
	logView.ShowLineNumbers = false
	logView.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#2563EB"))
	logView.Blur()

	// Initialize init view as viewport for initialization logs with scrolling
	initView := viewport.New(77, 12)
	initView.Style = logViewStyle

	// Initialize input area
	input := textarea.New()
	input.Placeholder = "Type your message here (or /quit to exit)..."
	input.Focus()
	input.Prompt = ""
	input.SetHeight(1)
	input.SetWidth(76)
	input.ShowLineNumbers = false
	input.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#2563EB"))

	// Initialize deployer
	config := analyzer.DefaultDeployerConfig()
	deployer, _ := analyzer.NewDeployer(config)

	// Create model with initial data
	model := Model{
		CurrentView:    MainMenuView,
		MainMenu:       menuList,
		ChatView:       chatView,
		LogView:        logView,
		InitView:       initView,
		Input:          input,
		ServerLogs:     []string{"Logs will appear here..."},
		ChatHistory:    []string{"Welcome to Hasher CLI!\n\nType your message below for hasher-based inference."},
		ServerReady:    false,
		ServerStarting: false,
		Width:          80,
		Height:         24,
		ProgressText:   "",
		ProgressStatus: "",
		Deployer:       deployer,
		DeviceIP:       "", // No device connected initially
		DeviceType:     "",
		CryptoEnabled:  false,
		APIClient:      client.NewAPIClient(8080),

		// Text selection fields
		SelectedText:    "",
		ShowCopyNotice:  false,
		CopyNoticeTimer: 0,
		SelectionMode:   false,
		ActiveView:      "chat",
		ChatContent:     "Welcome to Hasher CLI!\n\nType your message below for hasher-based inference.",
		LogContent:      "Logs will appear here...",
		InitContent:     "",
	}

	// Initialize views
	model.updateChatView()
	model.updateLogView()

	return model
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.ClearScreen,
		m.updateResourceData(),
		m.checkServerHealth(),
	)
}

// Update handles UI updates
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		}

	case tea.MouseMsg:
		cmds = append(cmds, m.handleMouse(msg))

	case tea.WindowSizeMsg:
		m, cmd = m.handleResize(msg)
		cmds = append(cmds, cmd)

	case updateResourceDataMsg:
		m.ResourceData = msg.data
		cmds = append(cmds, m.updateResourceData())

	case AppendLogMsg:
		m.ServerLogs = append(m.ServerLogs, msg.Log)
		if len(m.ServerLogs) > 50 {
			m.ServerLogs = m.ServerLogs[len(m.ServerLogs)-50:]
		}
		m.updateLogView()

	case AppendChatMsg:
		m.ChatHistory = append(m.ChatHistory, msg.Msg)
		m.updateChatView()

	case CombinedLogChatMsg:
		m.ServerLogs = append(m.ServerLogs, msg.Log)
		if len(m.ServerLogs) > 50 {
			m.ServerLogs = m.ServerLogs[len(m.ServerLogs)-50:]
		}
		m.updateLogView()
		m.ChatHistory = append(m.ChatHistory, msg.Chat)
		m.updateChatView()

	case ProgressUpdateMsg:
		m.ProgressText = msg.text
		m.ProgressStatus = msg.status

	case DeviceSelectedMsg:
		m.DeviceIP = msg.IP
		m.DeviceType = msg.DeviceType

	case DiscoveryResultMsg:
		// Update device info
		if msg.DeviceIP != "" {
			m.DeviceIP = msg.DeviceIP
			m.DeviceType = msg.DevType
		}
		// Update logs and chat
		m.ServerLogs = append(m.ServerLogs, msg.LogChat.Log)
		if len(m.ServerLogs) > 50 {
			m.ServerLogs = m.ServerLogs[len(m.ServerLogs)-50:]
		}
		m.updateLogView()
		m.ChatHistory = append(m.ChatHistory, msg.LogChat.Chat)
		m.updateChatView()

	case ServerReadyMsg:
		m.ServerReady = msg.Ready
		m.ServerStarting = msg.Starting
		if msg.Ready && msg.Port > 0 {
			m.APIClient = client.NewAPIClient(msg.Port)
		}
		// Continue periodic health checks
		return m, m.checkServerHealth()

	case ServerCmdMsg:
		m.ServerCmd = msg.Cmd
		if msg.Cmd != nil && msg.Cmd.Process != nil && !m.ServerReady {
			m.ServerStarting = true
		}

	case hideCopyNoticeMsg:
		m.ShowCopyNotice = false

	case textSelectedMsg:
		m.SelectedText = msg.Text
		if msg.Text != "" {
			// Copy to clipboard
			if err := clipboard.WriteAll(msg.Text); err == nil {
				m.ShowCopyNotice = true
				m.CopyNoticeTimer = 0
				cmds = append(cmds, m.startCopyNoticeTimer())
			}
		}

		// Note: scrollbarUpdateMsg removed - textarea handles scrolling natively
	}

	switch m.CurrentView {
	case MainMenuView:
		// If in initialization mode, handle InitView scrolling
		if m.ServerStarting && !m.ServerReady {
			// Pass message to viewport for mouse wheel support
			m.InitView, cmd = m.InitView.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				switch keyMsg.Type {
				case tea.KeyUp:
					m.InitView.LineUp(1)
				case tea.KeyDown:
					m.InitView.LineDown(1)
				case tea.KeyPgUp:
					m.InitView.LineUp(5)
				case tea.KeyPgDown:
					m.InitView.LineDown(5)
				}
			}
		} else {
			// Normal menu mode
			m.MainMenu, cmd = m.MainMenu.Update(msg)
			cmds = append(cmds, cmd)
		}

		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.Type {
			case tea.KeyEnter:
				if i, ok := m.MainMenu.SelectedItem().(menuItem); ok {
					switch i.title {
					case "1. Discovery":
						m.CurrentView = ChatView
						m.ChatHistory = append(m.ChatHistory, infoStyle.Render("Running Discovery..."))
						m.updateChatView()
						cmds = append(cmds, m.runDiscovery)
					case "2. Probe":
						m.CurrentView = ChatView
						m.ChatHistory = append(m.ChatHistory, infoStyle.Render("Running Probe..."))
						m.updateChatView()
						cmds = append(cmds, m.runProbe)
					case "3. Protocol":
						m.CurrentView = ChatView
						m.ChatHistory = append(m.ChatHistory, infoStyle.Render("Running Protocol Detection..."))
						m.updateChatView()
						cmds = append(cmds, m.runProtocol)
					case "4. Provision":
						m.CurrentView = ChatView
						m.ChatHistory = append(m.ChatHistory, infoStyle.Render("Running Provisioning..."))
						m.updateChatView()
						cmds = append(cmds, m.runProvision)
					case "5. Troubleshoot":
						m.CurrentView = ChatView
						m.ChatHistory = append(m.ChatHistory, infoStyle.Render("Running Troubleshooting..."))
						m.updateChatView()
						cmds = append(cmds, m.runTroubleshoot)
					case "6. Configure":
						m.CurrentView = ChatView
						cmds = append(cmds, m.runConfigure)
					case "7. Rules":
						m.CurrentView = ChatView
						cmds = append(cmds, m.runRulesManager)
					case "8. Test":
						m.CurrentView = ChatView
						m.ChatHistory = append(m.ChatHistory, infoStyle.Render("Running Communication Test..."))
						m.updateChatView()
						cmds = append(cmds, m.runTest)
					case "9. Chat":
						m.CurrentView = ChatView
					case "0. Quit":
						return m, tea.Quit
					}
				}
			}
		}

	case ChatView:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				if m.Input.Value() != "" {
					cmds = append(cmds, m.handleInput(m.Input.Value()))
					m.Input.Reset()
				}
			case tea.KeyEsc:
				m.CurrentView = MainMenuView
			case tea.KeyUp:
				m.ChatView.CursorUp()
			case tea.KeyDown:
				m.ChatView.CursorDown()
			}
			if msg.String() == "pgup" {
				for i := 0; i < m.ChatView.Height(); i++ {
					m.ChatView.CursorUp()
				}
			}
			if msg.String() == "pgdown" {
				for i := 0; i < m.ChatView.Height(); i++ {
					m.ChatView.CursorDown()
				}
			}
			if msg.String() == "ctrl+c" {
				selected := m.ChatView.Value()
				if selected != "" {
					if err := clipboard.WriteAll(selected); err == nil {
						m.ShowCopyNotice = true
						m.CopyNoticeTimer = 0
						cmds = append(cmds, m.startCopyNoticeTimer())
					}
				}
			}
			if msg.String() == "ctrl+v" {
				// Toggle between chat view, log view, and input
				if !m.SelectionMode {
					m.SelectionMode = true
					m.ActiveView = "chat"
					m.ChatView.Focus()
					m.LogView.Blur()
				} else if m.ActiveView == "chat" {
					m.ActiveView = "log"
					m.ChatView.Blur()
					m.LogView.Focus()
				} else {
					m.SelectionMode = false
					m.ChatView.Blur()
					m.LogView.Blur()
					m.Input.Focus()
				}
			}
			if msg.String() == "tab" {
				// Switch between chat and log views
				if m.ActiveView == "chat" {
					m.ActiveView = "log"
					m.ChatView.Blur()
					m.LogView.Focus()
				} else {
					m.ActiveView = "chat"
					m.LogView.Blur()
					m.ChatView.Focus()
				}
			}
			if msg.String() == "ctrl+c" {
				var selected string
				if m.ActiveView == "chat" {
					selected = m.ChatView.Value()
				} else {
					selected = m.LogView.Value()
				}
				if selected != "" {
					if err := clipboard.WriteAll(selected); err == nil {
						m.ShowCopyNotice = true
						m.CopyNoticeTimer = 0
						cmds = append(cmds, m.startCopyNoticeTimer())
					}
				}
			}
			if m.ActiveView == "chat" {
				if msg.String() == "up" || msg.String() == "k" {
					m.ChatView.CursorUp()
				}
				if msg.String() == "down" || msg.String() == "j" {
					m.ChatView.CursorDown()
				}
				if msg.String() == "pgup" {
					for i := 0; i < m.ChatView.Height(); i++ {
						m.ChatView.CursorUp()
					}
				}
				if msg.String() == "pgdown" {
					for i := 0; i < m.ChatView.Height(); i++ {
						m.ChatView.CursorDown()
					}
				}
			} else {
				if msg.String() == "up" || msg.String() == "k" {
					m.LogView.CursorUp()
				}
				if msg.String() == "down" || msg.String() == "j" {
					m.LogView.CursorDown()
				}
				if msg.String() == "pgup" {
					for i := 0; i < m.LogView.Height(); i++ {
						m.LogView.CursorUp()
					}
				}
				if msg.String() == "pgdown" {
					for i := 0; i < m.LogView.Height(); i++ {
						m.LogView.CursorDown()
					}
				}
			}

		case tea.MouseMsg:
			m.ChatView, cmd = m.ChatView.Update(msg)
			cmds = append(cmds, cmd)
			m.LogView, cmd = m.LogView.Update(msg)
			cmds = append(cmds, cmd)
		}

		m.Input, cmd = m.Input.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the UI
func (m Model) View() string {
	switch m.CurrentView {
	case MainMenuView:
		return m.renderMainMenu()
	case ChatView:
		return m.renderChatView()
	case ProgressView:
		return m.renderProgressView()
	}

	return m.renderMainMenu()
}

// renderMainMenu renders the main menu
func (m Model) renderMainMenu() string {
	serverStatus := "Server: Stopped"
	if m.ServerReady {
		serverStatus = "Server: Ready"
	} else if m.ServerStarting {
		serverStatus = "Server: Starting..."
	}

	// Build header with device IP on right side
	leftContent := fmt.Sprintf(" Hasher CLI Tool | %s", serverStatus)
	deviceStatus := ""
	if m.DeviceIP != "" {
		deviceStatus = fmt.Sprintf("ASIC: %s ", m.DeviceIP)
	}

	// Calculate padding for right-aligned device status
	padding := m.Width - len(leftContent) - len(deviceStatus) - 4 // 4 for style padding
	if padding < 1 {
		padding = 1
	}
	headerContent := leftContent + strings.Repeat(" ", padding) + deviceStatus
	header := headerStyle.Width(m.Width).Render(headerContent)

	// Build footer with device type on right side
	footerRight := ""
	if m.DeviceType != "" {
		footerRight = fmt.Sprintf(" | %s", m.DeviceType)
	}
	footer := footerStyle.Width(m.Width).Render(m.ResourceData + footerRight)

	// Render the logo centered
	logo := logoStyle.Render(hasherLogo)

	// Adjust menu height to fit: header(1) + footer(1) + logo(6) + margin(1) + menu_border(2) = 11
	// But Height() sets content area, so total menu = menuHeight + 2 for border
	// Total = 1 + 1 + 6 + 1 + (menuHeight + 2) + 1 = menuHeight + 12
	// For Total = Height: menuHeight = Height - 12
	// Add 1 more buffer to be safe
	menuHeight := m.Height - 13
	if menuHeight < 6 {
		menuHeight = 6
	}

	// Show initialization logs if server is starting but not ready
	var mainContent string
	if m.ServerStarting && !m.ServerReady {
		// Show initialization logs in blue box using viewport for scrolling
		// Update viewport dimensions - fill the available space
		m.InitView.Height = menuHeight - 3 // Leave room for title and padding
		if m.InitView.Height < 3 {
			m.InitView.Height = 3
		}
		m.InitView.Width = m.Width - 12 // Leave room for border and padding
		if m.InitView.Width < 20 {
			m.InitView.Width = 20
		}

		// Render scrollbar
		scrollbar := m.renderInitScrollbar()

		// Build the content: title + viewport + scrollbar side by side
		initTitle := infoStyle.Render("⚡ Initializing Hasher Server...") + "\n" + strings.Repeat("─", m.InitView.Width) + "\n"
		contentWithScrollbar := lipgloss.JoinHorizontal(lipgloss.Top, m.InitView.View(), " "+scrollbar)
		initBoxContent := initTitle + contentWithScrollbar

		mainContent = listStyle.Copy().Width(m.Width - 4).Height(menuHeight).Render(initBoxContent)
	} else {
		// Show normal menu
		mainContent = listStyle.Copy().Width(m.Width - 4).Height(menuHeight).Render(m.MainMenu.View())
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		logo,
		mainContent,
		footer,
	)
}

// renderInitScrollbar renders a vertical scrollbar for the initialization view
func (m Model) renderInitScrollbar() string {
	// Viewport height
	viewportHeight := m.InitView.Height
	if viewportHeight <= 0 {
		viewportHeight = 10
	}

	// Get total content lines
	totalLines := len(m.ServerLogs)
	if totalLines == 0 {
		// Empty scrollbar track
		return strings.Repeat("│\n", viewportHeight-1) + "│"
	}

	// If content fits, show empty scrollbar track
	if totalLines <= viewportHeight {
		return strings.Repeat("│\n", viewportHeight-1) + "│"
	}

	// Calculate scrollbar thumb size
	thumbHeight := viewportHeight * viewportHeight / totalLines
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	if thumbHeight > viewportHeight {
		thumbHeight = viewportHeight
	}

	// Calculate thumb position based on scroll offset
	maxScroll := totalLines - viewportHeight
	thumbPosition := 0
	if maxScroll > 0 && totalLines > 0 {
		thumbPosition = m.InitView.YOffset * (viewportHeight - thumbHeight) / maxScroll
		if thumbPosition < 0 {
			thumbPosition = 0
		}
		if thumbPosition > viewportHeight-thumbHeight {
			thumbPosition = viewportHeight - thumbHeight
		}
	}

	// Build scrollbar
	var scrollbar strings.Builder
	for i := 0; i < viewportHeight; i++ {
		if i >= thumbPosition && i < thumbPosition+thumbHeight {
			scrollbar.WriteString("█")
		} else {
			scrollbar.WriteString("│")
		}
		if i < viewportHeight-1 {
			scrollbar.WriteString("\n")
		}
	}

	return scrollbar.String()
}

// renderChatView renders the chat interface
func (m Model) renderChatView() string {
	serverStatus := "Server: Stopped"
	if m.ServerReady {
		serverStatus = "Server: Ready"
	} else if m.ServerStarting {
		serverStatus = "Server: Starting..."
	}

	// Build header with device IP and instructions on right side
	leftContent := fmt.Sprintf(" Hasher Chat | %s | ESC=menu", serverStatus)
	rightContent := ""
	if m.DeviceIP != "" {
		rightContent = fmt.Sprintf("ASIC: %s | ctrl+v: select mode", m.DeviceIP)
	} else {
		rightContent = "ctrl+v: select mode"
	}

	// Calculate padding for right-aligned content
	padding := m.Width - len(leftContent) - len(rightContent) - 4 // 4 for style padding
	if padding < 1 {
		padding = 1
	}
	headerContent := leftContent + strings.Repeat(" ", padding) + rightContent
	header := headerStyle.Width(m.Width).Render(headerContent)

	// Build footer with device type and copy notice
	footerRight := ""
	if m.DeviceType != "" {
		footerRight = fmt.Sprintf(" | %s", m.DeviceType)
	}
	footerText := m.ResourceData + footerRight
	if m.ShowCopyNotice {
		copyNotice := copyNoticeStyle.Render("✓ Copied to clipboard")
		footerText += " " + copyNotice
	} else if m.SelectionMode {
		footerText += " | [SELECT MODE] ↑↓ navigate | drag to select | ctrl+c copy"
	} else {
		footerText += " | ↑↓ scroll | ctrl+v select | ctrl+c copy"
	}
	footer := footerStyle.Width(m.Width).Render(footerText)

	// Calculate dimensions accounting for borders
	// header(1) + footer(1) + input_content(1) + input_border(2) + chat_border(2) + log_border(2) = 9
	contentHeight := m.Height - 9
	if contentHeight < 6 {
		contentHeight = 6
	}

	chatHeight := contentHeight / 2
	logHeight := contentHeight - chatHeight

	// Update textarea dimensions
	m.ChatView.SetWidth(m.Width - 4)
	m.ChatView.SetHeight(chatHeight)
	m.LogView.SetWidth(m.Width - 4)
	m.LogView.SetHeight(logHeight)

	// Render textarea content
	chatViewText := m.ChatView.View()
	logViewText := m.LogView.View()

	chatContent := chatViewStyle.Copy().
		Width(m.Width - 2).
		Height(chatHeight).
		Render(chatViewText)

	logContent := logViewStyle.Copy().
		Width(m.Width - 2).
		Height(logHeight).
		Render(logViewText)

	// Stack views vertically
	columns := lipgloss.JoinVertical(
		lipgloss.Left,
		chatContent,
		logContent,
	)

	input := inputStyle.Copy().Width(m.Width - 4).Height(1).Render(m.Input.View())

	// Build final UI
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		columns,
		input,
		footer,
	)
}

// renderProgressView renders the progress indicator
func (m Model) renderProgressView() string {
	header := headerStyle.Copy().Width(m.Width).Render(" Hasher CLI - Processing")
	footer := footerStyle.Copy().Width(m.Width).Render(m.ResourceData)

	progress := fmt.Sprintf("Processing: %s", m.ProgressText)
	if m.ProgressStatus != "" {
		progress += fmt.Sprintf("\nStatus: %s", m.ProgressStatus)
	}

	content := lipgloss.NewStyle().
		Padding(2, 4).
		Width(m.Width - 4).
		Height(m.Height - 6).
		Render(progress)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		footer,
	)
}

// handleResize adjusts layout for window resizing
func (m Model) handleResize(msg tea.WindowSizeMsg) (Model, tea.Cmd) {
	m.Width = msg.Width
	m.Height = msg.Height

	// Menu height must match renderMainMenu calculation
	menuHeight := msg.Height - 13
	if menuHeight < 6 {
		menuHeight = 6
	}
	m.MainMenu.SetSize(msg.Width-4, menuHeight)

	// Calculate dimensions for chat view
	// header(1) + footer(1) + input_content(1) + input_border(2) + chat_border(2) + log_border(2) = 9
	contentHeight := msg.Height - 9
	if contentHeight < 6 {
		contentHeight = 6
	}

	chatHeight := contentHeight / 2
	logHeight := contentHeight - chatHeight

	// Update textarea dimensions
	m.ChatView.SetWidth(msg.Width - 4)
	m.ChatView.SetHeight(chatHeight)
	m.LogView.SetWidth(msg.Width - 4)
	m.LogView.SetHeight(logHeight)

	m.Input.SetWidth(msg.Width - 6)
	m.Input.SetHeight(1)

	// Update init view dimensions
	m.InitView.Width = msg.Width - 12
	m.InitView.Height = menuHeight - 3
	if m.InitView.Height < 3 {
		m.InitView.Height = 3
	}

	headerStyle = headerStyle.Width(msg.Width)
	footerStyle = footerStyle.Width(msg.Width)

	m.updateChatView()
	m.updateLogView()

	return m, nil
}

// updateChatView updates the chat view with history
func (m *Model) updateChatView() {
	var content string
	width := m.ChatView.Width()
	for _, msg := range m.ChatHistory {
		// Word wrap message to viewport width
		wrappedMsg := ansi.Wordwrap(msg, width, " \t")
		content += wrappedMsg + "\n\n"
	}
	m.ChatView.SetValue(content)
	m.ChatContent = content
}

// updateLogView updates the log view with server logs
func (m *Model) updateLogView() {
	var content string
	width := m.LogView.Width()
	for _, log := range m.ServerLogs {
		// Word wrap log entry to viewport width
		wrappedLog := ansi.Wordwrap(log, width, " \t")
		content += wrappedLog + "\n"
	}
	m.LogView.SetValue(content)
	m.LogContent = content

	// Also update init view if we're in initialization mode
	if m.ServerStarting && !m.ServerReady {
		// Calculate how many lines fit in the viewport
		viewportHeight := m.InitView.Height
		if viewportHeight <= 0 {
			viewportHeight = 10
		}

		initWidth := m.InitView.Width
		if initWidth < 10 {
			initWidth = m.Width - 12
		}
		if initWidth < 10 {
			initWidth = 60
		}

		// Build content - only show last N logs that fit in viewport
		// Keep some buffer (3x viewport height) so user can scroll up
		maxLogLines := viewportHeight * 3
		startIdx := 0
		if len(m.ServerLogs) > maxLogLines {
			startIdx = len(m.ServerLogs) - maxLogLines
		}

		var initContent strings.Builder
		for i := startIdx; i < len(m.ServerLogs); i++ {
			log := m.ServerLogs[i]
			// Word wrap each log line
			wrappedLog := ansi.Wordwrap(log, initWidth, " \t")
			initContent.WriteString(wrappedLog)
			if i < len(m.ServerLogs)-1 {
				initContent.WriteString("\n")
			}
		}

		// Set content - viewport shows from top of content
		// Since we only include the last N logs, the "top" is actually recent history
		contentStr := initContent.String()
		m.InitView.SetContent(contentStr)
		m.InitContent = contentStr

		// Scroll to show the most recent logs (bottom of viewport content)
		// LineDown moves view down, so we want to be at the bottom
		m.InitView.GotoBottom()
	}
}

// updateResourceData updates resource usage information
func (m Model) updateResourceData() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		cpuPercent, _ := psutil.Percent(0, false)
		memInfo, _ := psmem.VirtualMemory()

		data := fmt.Sprintf("CPU: %.1f%% | RAM: %.1f%% | Go: %s",
			cpuPercent[0], memInfo.UsedPercent, runtime.Version())
		return updateResourceDataMsg{data}
	})
}

// checkServerHealth periodically checks if hasher-host is running
func (m Model) checkServerHealth() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		if m.APIClient != nil {
			health, err := m.APIClient.GetHealth()
			if err == nil && health.Status != "" {
				// Server is ready
				if !m.ServerReady {
					return ServerReadyMsg{Ready: true, Starting: false, Port: 0}
				}
			} else {
				// Server process exists but not ready yet
				if m.ServerStarting && !m.ServerReady {
					return ServerReadyMsg{Ready: false, Starting: true, Port: 0}
				}
			}
		}
		return nil
	})
}

// handleInput processes user input
func (m Model) handleInput(input string) tea.Cmd {
	if input == "/quit" {
		return tea.Quit
	}
	if input == "/menu" {
		return func() tea.Msg {
			m.CurrentView = MainMenuView
			return nil
		}
	}
	if input == "/help" {
		return func() tea.Msg {
			helpText := infoStyle.Render("Available Commands:\n")
			helpText += "  /quit           - Exit the application\n"
			helpText += "  /menu           - Return to main menu\n"
			helpText += "  /help           - Show this help\n"
			helpText += "  /rule add       - Add a logical rule\n"
			helpText += "  /rule delete    - Delete a logical rule\n"
			helpText += "  /rule list      - List all rules\n"
			helpText += "  /status         - Show server status\n"
			helpText += "  /train          - Train crypto-transformer\n"
			helpText += "\nScrolling:\n"
			helpText += "  ↑/↓             - Scroll line by line\n"
			helpText += "  PgUp/PgDn       - Scroll page by page\n"
			helpText += "  Mouse wheel     - Scroll both views\n"
			helpText += "\nCopy:\n"
			helpText += "  Ctrl+C          - Copy all text to clipboard\n"
			helpText += "  Right-click     - Copy text to clipboard\n"
			helpText += "\nType any text to perform inference with temporal ensemble."
			return AppendChatMsg{Msg: helpText}
		}
	}
	if input == "/status" {
		return m.handleStatusCommand()
	}
	if input == "/train" {
		return m.handleTrainCommand()
	}
	if strings.HasPrefix(input, "/rule") {
		return m.handleRuleCommand(input)
	}

	userMsg := userMessageStyle.Render("You: " + input)
	logStart := fmt.Sprintf("[%s] Sending to service: %s", time.Now().Format("15:04:05"), input)
	thinkingMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Italic(true).Render("Processing...")

	return tea.Batch(
		func() tea.Msg {
			return CombinedLogChatMsg{Log: logStart, Chat: userMsg}
		},
		func() tea.Msg {
			return AppendChatMsg{Msg: thinkingMsg}
		},
		func() tea.Msg {
			// Use API client to call crypto transformer inference
			resp, err := m.APIClient.CallCryptoTransformer(input, nil)

			if err != nil {
				logErr := fmt.Sprintf("[%s] Transformer Error: %v", time.Now().Format("15:04:05"), err)
				errMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render("Transformer Error: " + err.Error())
				return CombinedLogChatMsg{Log: logErr, Chat: errMsg}
			}

			logResp := fmt.Sprintf("[%s] Crypto-transformer inference completed", time.Now().Format("15:04:05"))
			llmMsg := llmMessageStyle.Render("Assistant: " + resp.Response)
			return CombinedLogChatMsg{Log: logResp, Chat: llmMsg}
		},
	)
}

// handleStatusCommand shows the current server and ASIC status
func (m Model) handleStatusCommand() tea.Cmd {
	return func() tea.Msg {
		var output strings.Builder
		output.WriteString(progressStyle.Render("System Status\n"))
		output.WriteString("════════════════\n\n")

		// Server status
		serverStatus := "Stopped"
		if m.ServerReady {
			serverStatus = "Ready"
		}
		output.WriteString(fmt.Sprintf("Server: %s\n", serverStatus))

		// Device status
		if m.DeviceIP != "" {
			output.WriteString(fmt.Sprintf("ASIC Device: %s (%s)\n", m.DeviceIP, m.DeviceType))
		} else {
			output.WriteString("ASIC Device: Not connected\n")
		}

		// API Server status
		health, err := m.APIClient.GetHealth()
		if err != nil {
			output.WriteString("API Server: Not running\n")
		} else if health.Status == "ok" {
			output.WriteString("API Server: Running\n")
			if health.UsingASIC {
				output.WriteString(fmt.Sprintf("ASIC Devices: %d chips\n", health.ChipCount))
			}
		} else {
			output.WriteString("API Server: Error\n")
		}

		return AppendChatMsg{Msg: output.String()}
	}
}

// handleTrainCommand initiates crypto-transformer training
func (m Model) handleTrainCommand() tea.Cmd {
	return func() tea.Msg {
		// Check if hasher-host is ready
		if !m.ServerReady {
			return CombinedLogChatMsg{
				Log:  "[" + time.Now().Format("15:04:05") + "] Training failed - hasher-host not ready",
				Chat: errorStyle.Render("Cannot start training: hasher-host is not ready. Please wait for the server to start."),
			}
		}

		// Start training progress message
		startMsg := infoStyle.Render("Starting crypto-transformer training...")
		thinkingMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Italic(true).Render("Initializing training loop...")

		return tea.Batch(
			func() tea.Msg {
				return CombinedLogChatMsg{
					Log:  "[" + time.Now().Format("15:04:05") + "] Training initiated",
					Chat: startMsg,
				}
			},
			func() tea.Msg {
				return AppendChatMsg{Msg: thinkingMsg}
			},
			func() tea.Msg {
				// Call hasher-host training API via API client
				resp, err := m.APIClient.CallTraining(5, 0.001, 32, generateTrainingSamples())

				if err != nil {
					logErr := fmt.Sprintf("[%s] Training API Error: %v", time.Now().Format("15:04:05"), err)
					errMsg := errorStyle.Render("Training failed: " + err.Error())
					return CombinedLogChatMsg{Log: logErr, Chat: errMsg}
				}

				logResp := fmt.Sprintf("[%s] Training completed - Epoch: %d, Loss: %.4f, Accuracy: %.4f",
					time.Now().Format("15:04:05"), resp.Epoch, resp.Loss, resp.Accuracy)

				successMsg := progressStyle.Render("Training completed successfully!\n")
				successMsg += fmt.Sprintf("Final Epoch: %d\n", resp.Epoch)
				successMsg += fmt.Sprintf("Final Loss: %.4f\n", resp.Loss)
				successMsg += fmt.Sprintf("Final Accuracy: %.2f%%\n", resp.Accuracy*100)
				successMsg += fmt.Sprintf("Training Time: %.2f seconds\n", resp.LatencyMs/1000)
				successMsg += fmt.Sprintf("ASIC Acceleration: %v\n", resp.UsingASIC)

				return CombinedLogChatMsg{Log: logResp, Chat: successMsg}
			},
		)
	}
}

// generateTrainingSamples creates sample training data for demonstration
func generateTrainingSamples() []string {
	samples := []string{
		"hello world",
		"neural network",
		"hash transformer",
		"asic acceleration",
		"crypto mining",
		"machine learning",
		"artificial intelligence",
		"deep learning",
		"blockchain technology",
		"quantum resistance",
		"seed encoding",
		"temporal ensemble",
		"logical validation",
		"hardware acceleration",
		"cryptographic ai",
		"hash matrix",
		"inference engine",
		"neural hashing",
		"asic processing",
		"transformer model",
		"embedding space",
		"attention mechanism",
		"feedforward network",
		"gradient descent",
		"backpropagation",
		"weight optimization",
		"loss function",
		"accuracy metric",
		"training dataset",
		"model checkpoint",
		"convergence criteria",
	}
	return samples
}

// handleRuleCommand processes /rule commands
func (m Model) handleRuleCommand(input string) tea.Cmd {
	return func() tea.Msg {
		parts := strings.Fields(input)
		if len(parts) < 2 {
			return AppendChatMsg{Msg: errorStyle.Render("Usage: /rule [add|delete|list] ...")}
		}

		subCmd := parts[1]
		switch subCmd {
		case "add":
			return m.handleRuleAdd(parts[2:])
		case "delete":
			return m.handleRuleDelete(parts[2:])
		case "list":
			return m.handleRuleList(parts[2:])
		default:
			return AppendChatMsg{Msg: errorStyle.Render("Unknown rule command. Use: add, delete, or list")}
		}
	}
}

// handleRuleAdd adds a new logical rule
func (m Model) handleRuleAdd(args []string) tea.Msg {
	if len(args) < 3 {
		return AppendChatMsg{Msg: errorStyle.Render("Usage: /rule add <domain> <type> <conclusion>\n  Types: constraint, subsumption, disjoint")}
	}

	domain := args[0]
	ruleType := args[1]
	conclusion := strings.Join(args[2:], " ")

	// Validate rule type
	if ruleType != "constraint" && ruleType != "subsumption" && ruleType != "disjoint" {
		return AppendChatMsg{Msg: errorStyle.Render("Invalid rule type. Must be: constraint, subsumption, or disjoint")}
	}

	// Create validator and add rule
	validator, err := hasher.NewLogicalValidator()
	if err != nil {
		return AppendChatMsg{Msg: errorStyle.Render(fmt.Sprintf("Error creating validator: %v", err))}
	}

	rule, err := hasher.NewLogicalRule(ruleType, []string{}, conclusion, "Added via CLI")
	if err != nil {
		return AppendChatMsg{Msg: errorStyle.Render(fmt.Sprintf("Error creating rule: %v", err))}
	}

	if err := validator.KnowledgeBase.AddRule(domain, rule); err != nil {
		return AppendChatMsg{Msg: errorStyle.Render(fmt.Sprintf("Error adding rule: %v", err))}
	}

	successMsg := progressStyle.Render(fmt.Sprintf("Rule added to domain '%s':\n", domain))
	successMsg += fmt.Sprintf("  Type: %s\n  Conclusion: %s\n", ruleType, conclusion)
	return AppendChatMsg{Msg: successMsg}
}

// handleRuleDelete deletes a logical rule
func (m Model) handleRuleDelete(args []string) tea.Msg {
	if len(args) < 2 {
		return AppendChatMsg{Msg: errorStyle.Render("Usage: /rule delete <domain> <index>")}
	}

	domain := args[0]
	var index int
	if _, err := fmt.Sscanf(args[1], "%d", &index); err != nil {
		return AppendChatMsg{Msg: errorStyle.Render("Invalid index. Must be a number.")}
	}

	// Create validator and delete rule
	validator, err := hasher.NewLogicalValidator()
	if err != nil {
		return AppendChatMsg{Msg: errorStyle.Render(fmt.Sprintf("Error creating validator: %v", err))}
	}

	if err := validator.KnowledgeBase.RemoveRule(domain, index); err != nil {
		return AppendChatMsg{Msg: errorStyle.Render(fmt.Sprintf("Error deleting rule: %v", err))}
	}

	return AppendChatMsg{Msg: progressStyle.Render(fmt.Sprintf("Rule %d deleted from domain '%s'", index, domain))}
}

// handleRuleList lists logical rules
func (m Model) handleRuleList(args []string) tea.Msg {
	validator, err := hasher.NewLogicalValidator()
	if err != nil {
		return AppendChatMsg{Msg: errorStyle.Render(fmt.Sprintf("Error creating validator: %v", err))}
	}

	var output strings.Builder
	output.WriteString(progressStyle.Render("Logical Validation Rules\n"))
	output.WriteString("═════════════════════════\n\n")

	if len(args) > 0 {
		// List rules for specific domain
		domain := args[0]
		rules, err := validator.KnowledgeBase.GetRules(domain)
		if err != nil {
			return AppendChatMsg{Msg: errorStyle.Render(fmt.Sprintf("Error getting rules: %v", err))}
		}

		if len(rules) == 0 {
			output.WriteString(fmt.Sprintf("No rules found for domain '%s'\n", domain))
		} else {
			output.WriteString(fmt.Sprintf("Domain: %s (%d rules)\n\n", domain, len(rules)))
			for i, rule := range rules {
				output.WriteString(fmt.Sprintf("[%d] %s\n", i, rule.String()))
				if rule.Description != "" {
					output.WriteString(fmt.Sprintf("    %s\n", rule.Description))
				}
			}
		}
	} else {
		// List all domains and rules
		for domain, rules := range validator.KnowledgeBase.Domains {
			output.WriteString(fmt.Sprintf("Domain: %s (%d rules)\n", domain, len(rules)))
			for i, rule := range rules {
				output.WriteString(fmt.Sprintf("  [%d] %s\n", i, rule.String()))
			}
			output.WriteString("\n")
		}
	}

	return AppendChatMsg{Msg: output.String()}
}

// DiscoveryResultMsg contains the result of device discovery
type DiscoveryResultMsg struct {
	LogChat  CombinedLogChatMsg
	DeviceIP string
	DevType  string
}

// runDiscovery runs device discovery
func (m Model) runDiscovery() tea.Msg {
	return func() tea.Msg {
		if m.Deployer == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Error: Deployer not initialized", time.Now().Format("15:04:05")),
				Chat: errorStyle.Render("Error: Deployer not initialized"),
			}
		}

		// Capture logs
		var logBuffer bytes.Buffer
		m.Deployer.SetLogWriter(&logBuffer)

		result, err := m.Deployer.RunDiscovery()
		if err != nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Discovery failed: %v", time.Now().Format("15:04:05"), err),
				Chat: errorStyle.Render(fmt.Sprintf("Discovery failed: %v", err)),
			}
		}

		// Get discovered devices
		devices := m.Deployer.GetDevices()
		var chatMsg string
		var selectedIP, selectedType string

		if len(devices) > 0 {
			chatMsg = progressStyle.Render(fmt.Sprintf("Found %d ASIC device(s):\n", len(devices)))
			for i, dev := range devices {
				chatMsg += fmt.Sprintf("\n[%d] %s (%s)", i+1, dev.IPAddress, dev.DeviceType)
				if dev.Accessible {
					chatMsg += " - Accessible"
				}
			}
			// Auto-select first device
			m.Deployer.SelectDevice(0)
			selectedIP = devices[0].IPAddress
			selectedType = devices[0].DeviceType
			chatMsg += fmt.Sprintf("\n\n✓ Auto-selected device: %s", selectedIP)
			chatMsg += "\n\n" + infoStyle.Render("Next: Run 'Probe' to gather device information")
		} else {
			chatMsg = infoStyle.Render("No ASIC devices found on network.\n\nCheck that ASIC devices are powered on and connected to the network.")
		}

		return DiscoveryResultMsg{
			LogChat: CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Discovery complete (%.2fs)\n%s", time.Now().Format("15:04:05"), result.Duration, logBuffer.String()),
				Chat: chatMsg,
			},
			DeviceIP: selectedIP,
			DevType:  selectedType,
		}
	}()
}

// runProbe runs device probe
func (m Model) runProbe() tea.Msg {
	return func() tea.Msg {
		if m.Deployer == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Error: Deployer not initialized", time.Now().Format("15:04:05")),
				Chat: errorStyle.Render("Error: Deployer not initialized"),
			}
		}

		device := m.Deployer.GetActiveDevice()
		if device == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] No device selected - run Discovery first", time.Now().Format("15:04:05")),
				Chat: infoStyle.Render("No device selected. Run Discovery first to find ASIC devices."),
			}
		}

		var logBuffer bytes.Buffer
		m.Deployer.SetLogWriter(&logBuffer)

		result, err := m.Deployer.RunProbe()
		if err != nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Probe failed: %v", time.Now().Format("15:04:05"), err),
				Chat: errorStyle.Render(fmt.Sprintf("Probe failed: %v", err)),
			}
		}

		chatOutput := progressStyle.Render("Probe Results:\n") + result.Output
		chatOutput += "\n\n" + infoStyle.Render("Next: Run 'Protocol' to detect communication protocol")

		return CombinedLogChatMsg{
			Log:  fmt.Sprintf("[%s] Probe complete (%.2fs)\n%s", time.Now().Format("15:04:05"), result.Duration, logBuffer.String()),
			Chat: chatOutput,
		}
	}()
}

// runProtocol runs protocol detection
func (m Model) runProtocol() tea.Msg {
	return func() tea.Msg {
		if m.Deployer == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Error: Deployer not initialized", time.Now().Format("15:04:05")),
				Chat: errorStyle.Render("Error: Deployer not initialized"),
			}
		}

		device := m.Deployer.GetActiveDevice()
		if device == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] No device selected - run Discovery first", time.Now().Format("15:04:05")),
				Chat: infoStyle.Render("No device selected. Run Discovery first to find ASIC devices."),
			}
		}

		var logBuffer bytes.Buffer
		m.Deployer.SetLogWriter(&logBuffer)

		result, err := m.Deployer.RunProtocol()
		if err != nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Protocol detection failed: %v", time.Now().Format("15:04:05"), err),
				Chat: errorStyle.Render(fmt.Sprintf("Protocol detection failed: %v", err)),
			}
		}

		chatOutput := progressStyle.Render("Protocol Detection Results:\n") + result.Output
		chatOutput += "\n\n" + infoStyle.Render("Next: Run 'Provision' to deploy hasher-server to the device")

		return CombinedLogChatMsg{
			Log:  fmt.Sprintf("[%s] Protocol detection complete (%.2fs)\n%s", time.Now().Format("15:04:05"), result.Duration, logBuffer.String()),
			Chat: chatOutput,
		}
	}()
}

// runProvision runs device provisioning
func (m Model) runProvision() tea.Msg {
	return func() tea.Msg {
		if m.Deployer == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Error: Deployer not initialized", time.Now().Format("15:04:05")),
				Chat: errorStyle.Render("Error: Deployer not initialized"),
			}
		}

		device := m.Deployer.GetActiveDevice()
		if device == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] No device selected - run Discovery first", time.Now().Format("15:04:05")),
				Chat: infoStyle.Render("No device selected. Run Discovery first to find ASIC devices."),
			}
		}

		var logBuffer bytes.Buffer
		m.Deployer.SetLogWriter(&logBuffer)

		result, err := m.Deployer.RunProvision()
		if err != nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Provisioning failed: %v", time.Now().Format("15:04:05"), err),
				Chat: errorStyle.Render(fmt.Sprintf("Provisioning failed: %v", err)),
			}
		}

		chatOutput := progressStyle.Render("Provisioning Results:\n") + result.Output
		chatOutput += "\n\n" + infoStyle.Render("Next: Run 'Test' to verify ASIC communication, or 'Chat' to start inference")

		return CombinedLogChatMsg{
			Log:  fmt.Sprintf("[%s] Provisioning complete (%.2fs)\n%s", time.Now().Format("15:04:05"), result.Duration, logBuffer.String()),
			Chat: chatOutput,
		}
	}()
}

// runTroubleshoot runs troubleshooting
func (m Model) runTroubleshoot() tea.Msg {
	return func() tea.Msg {
		if m.Deployer == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Error: Deployer not initialized", time.Now().Format("15:04:05")),
				Chat: errorStyle.Render("Error: Deployer not initialized"),
			}
		}

		device := m.Deployer.GetActiveDevice()
		if device == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] No device selected - run Discovery first", time.Now().Format("15:04:05")),
				Chat: infoStyle.Render("No device selected. Run Discovery first to find ASIC devices."),
			}
		}

		var logBuffer bytes.Buffer
		m.Deployer.SetLogWriter(&logBuffer)

		result, err := m.Deployer.RunTroubleshoot()
		if err != nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Troubleshooting failed: %v", time.Now().Format("15:04:05"), err),
				Chat: errorStyle.Render(fmt.Sprintf("Troubleshooting failed: %v", err)),
			}
		}

		chatOutput := progressStyle.Render("Troubleshooting Report:\n") + result.Output
		chatOutput += "\n\n" + infoStyle.Render("Review the report above. Run 'Provision' if hasher-server is not deployed.")

		return CombinedLogChatMsg{
			Log:  fmt.Sprintf("[%s] Troubleshooting complete (%.2fs)\n%s", time.Now().Format("15:04:05"), result.Duration, logBuffer.String()),
			Chat: chatOutput,
		}
	}()
}

// runConfigure runs configuration
func (m Model) runConfigure() tea.Msg {
	return func() tea.Msg {
		if m.Deployer == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Error: Deployer not initialized", time.Now().Format("15:04:05")),
				Chat: errorStyle.Render("Error: Deployer not initialized"),
			}
		}

		// Show current configuration
		device := m.Deployer.GetActiveDevice()
		var output strings.Builder
		output.WriteString(progressStyle.Render("Current Configuration:\n"))
		output.WriteString(strings.Repeat("-", 40) + "\n\n")

		if device != nil {
			output.WriteString(fmt.Sprintf("  ✓ Active Device: %s\n", device.IPAddress))
			output.WriteString(fmt.Sprintf("    Device Type:   %s\n", device.DeviceType))
			output.WriteString(fmt.Sprintf("    Protocol:      %s\n", device.Protocol.String()))
			output.WriteString(fmt.Sprintf("    Accessible:    %v\n", device.Accessible))
			if len(device.OpenPorts) > 0 {
				output.WriteString(fmt.Sprintf("    Open Ports:    %v\n", device.OpenPorts))
			}
		} else {
			output.WriteString("  ✗ No device selected\n")
			output.WriteString("\n" + infoStyle.Render("Run 'Discovery' first to find ASIC devices on the network."))
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Configuration displayed (no device)", time.Now().Format("15:04:05")),
				Chat: output.String(),
			}
		}

		output.WriteString("\n" + infoStyle.Render("Workflow Steps:") + "\n")
		output.WriteString("  1. Discovery  - Find ASIC devices on network\n")
		output.WriteString("  2. Probe      - Gather device system information\n")
		output.WriteString("  3. Protocol   - Detect communication protocol\n")
		output.WriteString("  4. Provision  - Deploy hasher-server binary\n")
		output.WriteString("  5. Test       - Verify ASIC communication\n")
		output.WriteString("  6. Chat       - Start inference with ASIC\n")

		return CombinedLogChatMsg{
			Log:  fmt.Sprintf("[%s] Configuration displayed", time.Now().Format("15:04:05")),
			Chat: output.String(),
		}
	}()
}

// runRulesManager shows the logical rules management interface
func (m Model) runRulesManager() tea.Msg {
	return func() tea.Msg {
		var output strings.Builder
		output.WriteString(progressStyle.Render("Logical Validation Rules Manager\n"))
		output.WriteString("══════════════════════════════════════\n\n")

		// Create a validator for rule management (independent of orchestrator)
		validator, err := hasher.NewLogicalValidator()
		if err != nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Error creating validator: %v", time.Now().Format("15:04:05"), err),
				Chat: errorStyle.Render(fmt.Sprintf("Error: %v", err)),
			}
		}

		// Show available domains and rules
		output.WriteString(infoStyle.Render("Available Domains:\n"))
		for domain, rules := range validator.KnowledgeBase.Domains {
			output.WriteString(fmt.Sprintf("\n  %s (%d rules)\n", domain, len(rules)))
			for i, rule := range rules {
				output.WriteString(fmt.Sprintf("    [%d] %s\n", i, rule.String()))
				if rule.Description != "" {
					output.WriteString(fmt.Sprintf("        %s\n", rule.Description))
				}
			}
		}

		output.WriteString("\n" + infoStyle.Render("Rule Management Commands:\n"))
		output.WriteString("  In Chat view, use these commands:\n")
		output.WriteString("  /rule add <domain> <type> <conclusion>\n")
		output.WriteString("    Types: constraint, subsumption, disjoint\n")
		output.WriteString("  /rule delete <domain> <index>\n")
		output.WriteString("  /rule list [domain]\n")
		output.WriteString("\n  Example:\n")
		output.WriteString("    /rule add temperature constraint \"Valid range: -40 to 85\"\n")

		return CombinedLogChatMsg{
			Log:  fmt.Sprintf("[%s] Rules manager displayed", time.Now().Format("15:04:05")),
			Chat: output.String(),
		}
	}()
}

// runTest runs service tests
func (m Model) runTest() tea.Msg {
	return func() tea.Msg {
		if m.Deployer == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Error: Deployer not initialized", time.Now().Format("15:04:05")),
				Chat: errorStyle.Render("Error: Deployer not initialized"),
			}
		}

		device := m.Deployer.GetActiveDevice()
		if device == nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] No device selected - run Discovery first", time.Now().Format("15:04:05")),
				Chat: infoStyle.Render("No device selected. Run Discovery first to find ASIC devices."),
			}
		}

		var logBuffer bytes.Buffer
		m.Deployer.SetLogWriter(&logBuffer)

		result, err := m.Deployer.RunTest()
		if err != nil {
			return CombinedLogChatMsg{
				Log:  fmt.Sprintf("[%s] Test failed: %v", time.Now().Format("15:04:05"), err),
				Chat: errorStyle.Render(fmt.Sprintf("Test failed: %v", err)),
			}
		}

		chatOutput := progressStyle.Render("Communication Test Results:\n") + result.Output
		chatOutput += "\n\n" + infoStyle.Render("Tests complete! Run 'Chat' to start inference with the ASIC device.")

		return CombinedLogChatMsg{
			Log:  fmt.Sprintf("[%s] Test complete (%.2fs)\n%s", time.Now().Format("15:04:05"), result.Duration, logBuffer.String()),
			Chat: chatOutput,
		}
	}()
}

// Messages
type updateResourceDataMsg struct {
	data string
}

type AppendLogMsg struct {
	Log string
}

type AppendChatMsg struct {
	Msg string
}

type hideCopyNoticeMsg struct{}

type textSelectedMsg struct {
	Text string
}

type CombinedLogChatMsg struct {
	Log  string
	Chat string
}

type ProgressUpdateMsg struct {
	text   string
	status string
}

// DeviceSelectedMsg is sent when an ASIC device is discovered and selected
type DeviceSelectedMsg struct {
	IP         string
	DeviceType string
}

// ServerReadyMsg is sent when hasher-host is ready
type ServerReadyMsg struct {
	Ready    bool
	Starting bool
	Port     int
}

// ServerCmdMsg is sent to update the server command reference
type ServerCmdMsg struct {
	Cmd *exec.Cmd
}

// handleMouse handles mouse events for text selection and scrolling
func (m Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	switch msg.Type {
	case tea.MouseRight:
		// Copy all text on right-click (context-aware)
		var selected string
		if m.ServerStarting && !m.ServerReady && m.CurrentView == MainMenuView {
			// During initialization, copy all InitContent
			selected = m.InitContent
		} else if m.CurrentView == ChatView {
			// In chat view, copy from ChatView
			selected = m.ChatView.Value()
		} else if m.CurrentView == MainMenuView {
			// In menu view, copy from LogView
			selected = m.LogView.Value()
		}
		if selected != "" {
			if err := clipboard.WriteAll(selected); err == nil {
				m.ShowCopyNotice = true
				m.CopyNoticeTimer = 0
				return m.startCopyNoticeTimer()
			}
		}
	}
	return nil
}

func (m Model) startCopyNoticeTimer() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return hideCopyNoticeMsg{}
	})
}

// renderScrollbar renders a vertical scrollbar
func (m Model) renderScrollbar(totalHeight, visibleHeight, scrollOffset int, isDragging bool) string {
	if totalHeight <= visibleHeight {
		// No scrollbar needed if content fits
		return strings.Repeat(" ", visibleHeight)
	}

	// Calculate thumb size and position
	thumbHeight := visibleHeight * visibleHeight / totalHeight
	if thumbHeight < 1 {
		thumbHeight = 1
	}

	maxScrollOffset := totalHeight - visibleHeight
	thumbPosition := 0
	if maxScrollOffset > 0 {
		thumbPosition = scrollOffset * (visibleHeight - thumbHeight) / maxScrollOffset
	}

	// Build scrollbar track with thumb
	var scrollbar strings.Builder
	for i := 0; i < visibleHeight; i++ {
		if i >= thumbPosition && i < thumbPosition+thumbHeight {
			if isDragging {
				scrollbar.WriteString(scrollbarThumbHoverStyle.Render("│"))
			} else {
				scrollbar.WriteString(scrollbarThumbStyle.Render("│"))
			}
		} else {
			scrollbar.WriteString(scrollbarTrackStyle.Render(" "))
		}
	}

	return scrollbar.String()
}

// isInScrollbar checks if mouse coordinates are within a scrollbar area
func (m Model) isInScrollbar(x, y int, viewportX, viewportY, viewportWidth, viewportHeight int) bool {
	// Scrollbar is positioned at the right edge of the viewport
	scrollbarX := viewportX + viewportWidth - 1
	scrollbarY := viewportY

	return x == scrollbarX && y >= scrollbarY && y < scrollbarY+viewportHeight
}
