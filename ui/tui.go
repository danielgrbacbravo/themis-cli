package ui

//use charm CLI to create the TUI for this application

// A simple example demonstrating the use of multiple text input components
// from the Bubbles component library.

import (
	"fmt"
	"strings"
	cfg "themis-cli/config"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	focusedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle         = focusedStyle.Copy()
	noStyle             = lipgloss.NewStyle()
	helpStyle           = blurredStyle.Copy()
	cursorModeHelpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	focusedButton = focusedStyle.Copy().Render("[ Submit ]")
	blurredButton = fmt.Sprintf("[ %s ]", blurredStyle.Render("Submit"))

	// vars
	spinnerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	dotStyle      = helpStyle.Copy().UnsetMargins()
	durationStyle = dotStyle.Copy()
	appStyle      = lipgloss.NewStyle().Margin(1, 2, 0, 2)
)

type LoginModel struct {
	focusIndex int
	inputs     []textinput.Model
	cursorMode cursor.Mode
}

type AssignmentLoaderModel struct {
	assignments []ResultMsg
	spinner     spinner.Model
	quitting    bool
}

type ResultMsg struct {
	assignmentName string
	assignmentURL  string
	duration       time.Duration
}

func (result ResultMsg) String() string {
	return fmt.Sprintf("%s %s %s", result.assignmentName,
		durationStyle.Render(result.assignmentURL), result.duration)
}

func InitalizeResultMsg() ResultMsg {
	return ResultMsg{
		assignmentName: "",
		assignmentURL:  "",
		duration:       0,
	}
}

func (result ResultMsg) SetAssignmentName(name string) ResultMsg {
	result.assignmentName = name
	return result
}

func (result ResultMsg) SetAssignmentURL(url string) ResultMsg {
	result.assignmentURL = url
	return result
}

func (result ResultMsg) SetDuration(duration time.Duration) ResultMsg {
	result.duration = duration
	return result
}

func NewAssignmentLoaderModel() AssignmentLoaderModel {
	const numLastResults = 5
	s := spinner.New()
	s.Style = spinnerStyle
	return AssignmentLoaderModel{
		spinner:     s,
		assignments: make([]ResultMsg, numLastResults),
	}
}

func (m AssignmentLoaderModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m LoginModel) Init() tea.Cmd {
	return textinput.Blink
}

func initialModel() LoginModel {
	m := LoginModel{
		inputs: make([]textinput.Model, 2),
	}

	var t textinput.Model
	for i := range m.inputs {
		t = textinput.New()
		t.Cursor.Style = cursorStyle
		t.CharLimit = 32

		switch i {
		case 0:
			t.Placeholder = "S or P number"
			t.Focus()
			t.PromptStyle = focusedStyle
			t.TextStyle = focusedStyle
		case 1:
			t.Placeholder = "Password"
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}

		m.inputs[i] = t
	}

	return m
}

func (m AssignmentLoaderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.quitting = true
		return m, tea.Quit
	case ResultMsg:
		m.assignments = append(m.assignments[1:], msg)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m LoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit

		// Change cursor mode
		case "ctrl+r":
			m.cursorMode++
			if m.cursorMode > cursor.CursorHide {
				m.cursorMode = cursor.CursorBlink
			}
			cmds := make([]tea.Cmd, len(m.inputs))
			for i := range m.inputs {
				cmds[i] = m.inputs[i].Cursor.SetMode(m.cursorMode)
			}
			return m, tea.Batch(cmds...)

		// Set focus to next input
		case "tab", "shift+tab", "enter", "up", "down":
			s := msg.String()

			// Did the user press enter while the submit button was focused?
			// If so, exit.
			if s == "enter" && m.focusIndex == len(m.inputs) {
				cfg.SetUsernameInENV(m.inputs[0].Value())
				cfg.SetPasswordInENV(m.inputs[1].Value())
				return m, tea.Quit
			}

			// Cycle indexes
			if s == "up" || s == "shift+tab" {
				m.focusIndex--
			} else {
				m.focusIndex++
			}

			if m.focusIndex > len(m.inputs) {
				m.focusIndex = 0
			} else if m.focusIndex < 0 {
				m.focusIndex = len(m.inputs)
			}

			cmds := make([]tea.Cmd, len(m.inputs))
			for i := 0; i <= len(m.inputs)-1; i++ {
				if i == m.focusIndex {
					// Set focused state
					cmds[i] = m.inputs[i].Focus()
					m.inputs[i].PromptStyle = focusedStyle
					m.inputs[i].TextStyle = focusedStyle
					continue
				}
				// Remove focused state
				m.inputs[i].Blur()
				m.inputs[i].PromptStyle = noStyle
				m.inputs[i].TextStyle = noStyle
			}

			return m, tea.Batch(cmds...)
		}
	}
	// Handle character input and blinking
	cmd := m.updateInputs(msg)
	return m, cmd
}

func (m *LoginModel) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))

	// Only text inputs with Focus() set will respond, so it's safe to simply
	// update all of them here without any further logic.
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (m LoginModel) View() string {
	var b strings.Builder
	for i := range m.inputs {
		b.WriteString(m.inputs[i].View())
		if i < len(m.inputs)-1 {
			b.WriteRune('\n')
		}
	}

	button := &blurredButton
	if m.focusIndex == len(m.inputs) {
		button = &focusedButton
	}
	fmt.Fprintf(&b, "\n\n%s\n\n", *button)

	b.WriteString(helpStyle.Render("cursor mode is "))
	b.WriteString(cursorModeHelpStyle.Render(m.cursorMode.String()))
	b.WriteString(helpStyle.Render(" (ctrl+r to change style)"))
	return b.String()
}

func (m AssignmentLoaderModel) View() string {
	var s string

	if m.quitting {
		s += "All Done!"
	} else {
		s += m.spinner.View() + " Pulling Assignments..."
	}

	s += "\n\n"

	for _, res := range m.assignments {
		s += res.String() + "\n"
	}

	if !m.quitting {
		s += helpStyle.Render("Press any key to exit")
	}

	if m.quitting {
		s += "\n"
	}

	return appStyle.Render(s)
}

func SetEnvVarsFromTUI() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("could not start program: %s\n", err)
	}
}
